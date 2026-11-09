// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// articlePageCache holds rendered article portions for one search. SQLite owns
// the temporary file. Find-in-articles match counts are independent of this cache.
const articlePageCacheMaxBytes = 128 << 20

type articlePageCache struct {
	mu       sync.Mutex
	id       string
	db       *sql.DB
	access   int64
	maxBytes int64
	// flights holds one render per key that is in progress. The walk's own page
	// load and the reader's prefetch ask for the same block within a few hundred
	// milliseconds; without this, whichever arrives second starts a second
	// render of 4.5-7.8 MB instead of waiting for the first.
	flights map[string]chan struct{}
}

func (c *articlePageCache) resetFor(id string, diagnostics bool) {
	if !wildcardCacheIDPattern.MatchString(id) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id == id {
		return
	}
	previous := c.id
	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
	c.id, c.access = id, 0
	if diagnostics {
		logx.SystemDiagnostic("article_page_cache stage=reset id=%q previous_id=%q", id, previous)
	}
}

func (c *articlePageCache) eligible(r SearchRequest) bool {
	return wildcardCacheIDPattern.MatchString(r.CacheID) &&
		(r.CacheAction == "page" || r.CacheAction == "" && r.Limit > 0 &&
			r.Dictionary != "" && r.Dictionary != "all" && !strings.Contains(r.Dictionary, ","))
}

func (c *articlePageCache) openLocked() error {
	if c.db != nil {
		return nil
	}
	db, err := store.OpenTransientCacheDB(false)
	if err != nil {
		return err
	}
	if _, err = db.Exec(`PRAGMA auto_vacuum=FULL`); err == nil {
		_, err = db.Exec(`CREATE TABLE article_page(cache_key TEXT PRIMARY KEY, dict_id TEXT NOT NULL, page_start INTEGER NOT NULL, page_size INTEGER NOT NULL, payload BLOB NOT NULL, bytes INTEGER NOT NULL, accessed INTEGER NOT NULL)`)
		if err == nil {
			// The page's metadata and its bodies live apart: a reader needs one
			// body to show the awaited article and the rest only as it reads,
			// while the walk's own page load assembles the whole page from the
			// same rows.
			_, err = db.Exec(`CREATE TABLE article_page_body(cache_key TEXT NOT NULL, idx INTEGER NOT NULL, payload BLOB NOT NULL, bytes INTEGER NOT NULL, PRIMARY KEY(cache_key, idx))`)
		}
	}
	if err != nil {
		_ = db.Close()
		return err
	}
	c.db = db
	return nil
}

// articlePageCacheKey is what a stored page DEPENDS ON: the fields that change
// the bytes of the answer. The request carries more - the result limit, the
// diagnostic switches, the return-cache naming - and none of those change a
// page, so keying on the whole request stored one block under several keys and
// every caller re-rendered it: on a Wikipedia library that is 4.5-7.8 MB and
// half a second of work per duplicate, in a cache that holds 128 MiB.
//
// The window is the one field that differs by action: a page names it as
// start/size, the first answer as its limit.
func articlePageCacheKey(r SearchRequest) string {
	start, size := r.Start, r.Size
	if r.CacheAction != "page" {
		start, size = 0, r.Limit
	}
	key, _ := json.Marshal(struct {
		Query            string `json:"q"`
		Mode             string `json:"mode,omitempty"`
		Format           string `json:"format,omitempty"`
		Dictionary       string `json:"dict,omitempty"`
		Highlight        bool   `json:"hl,omitempty"`
		IgnoreDiacritics bool   `json:"ignoreDiacritics,omitempty"`
		Rung             string `json:"rung,omitempty"`
		CacheAction      string `json:"cacheAction,omitempty"`
		CacheID          string `json:"cacheID,omitempty"`
		CacheCount       int    `json:"cacheCount,omitempty"`
		CacheOrder       int    `json:"cacheOrder,omitempty"`
		Start            int    `json:"start,omitempty"`
		Size             int    `json:"size,omitempty"`
		Origin           string `json:"origin,omitempty"`
	}{Query: r.Query, Mode: r.Mode, Format: r.Format, Dictionary: r.Dictionary,
		Highlight: r.Highlight, IgnoreDiacritics: r.IgnoreDiacritics, Rung: r.Rung,
		CacheAction: r.CacheAction, CacheID: r.CacheID, CacheCount: r.CacheCount,
		CacheOrder: r.CacheOrder, Start: start, Size: size, Origin: r.Origin})
	return string(key)
}

func (c *articlePageCache) serve(ctx context.Context, r SearchRequest, emit func(SearchEvent) error, load func(func(SearchEvent) error) error) error {
	started := time.Now()
	pageStart, pageSize := r.Start, r.Size
	if r.CacheAction != "page" {
		pageStart, pageSize = 0, r.Limit
	}
	limit := c.maxBytes
	if limit <= 0 {
		limit = articlePageCacheMaxBytes
	}
	key := articlePageCacheKey(r)
	diagnostics := r.PageCacheDiagnostics

	// Pages are stored as metadata plus one row per result body. The reader
	// needs the awaited article now and the others as the fill reaches them;
	// the walk's own page load assembles the whole page from the same rows, so
	// neither side ever renders a block twice.
	rowsFor := func(first, only int) map[int][]byte {
		rows := make(map[int][]byte)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.id != r.CacheID || c.openLocked() != nil {
			return rows
		}
		if only == 1 {
			var payload []byte
			if c.db.QueryRowContext(ctx, `SELECT payload FROM article_page_body WHERE cache_key=? AND idx=?`, string(key), first).Scan(&payload) == nil {
				rows[first] = payload
			}
			return rows
		}
		result, err := c.db.QueryContext(ctx, `SELECT idx,payload FROM article_page_body WHERE cache_key=? ORDER BY idx`, string(key))
		if err != nil {
			return rows
		}
		defer result.Close()
		for result.Next() {
			var index int
			var payload []byte
			if result.Scan(&index, &payload) == nil {
				rows[index] = payload
			}
		}
		return rows
	}
	fillBodies := func(events []SearchEvent, rows map[int][]byte, bodyIndex int, bodyOnly bool) {
		if len(events) == 0 || bodyIndex == -1 {
			return
		}
		results := events[0].Results
		if bodyIndex == 0 {
			for index, payload := range rows {
				if index >= len(results) {
					continue
				}
				var result dict.Result
				if json.Unmarshal(payload, &result) == nil {
					results[index].Body = result.Body
				}
			}
			return
		}
		index := bodyIndex - 1
		if index >= len(results) {
			return
		}
		if payload, ok := rows[index]; ok {
			var result dict.Result
			if json.Unmarshal(payload, &result) == nil {
				results[index].Body = result.Body
			}
		}
		if bodyOnly {
			events[0].Results = results[index : index+1]
		}
	}
	stripBodies := func(events []SearchEvent) {
		if len(events) == 0 {
			return
		}
		results := make([]dict.Result, len(events[0].Results))
		copy(results, events[0].Results)
		for index := range results {
			results[index].Body = ""
		}
		events[0].Results = results
	}
	emitCached := func(metadata []byte) (bool, error) {
		var events []SearchEvent
		if json.Unmarshal(metadata, &events) != nil {
			return false, nil
		}
		rows := map[int][]byte{}
		if len(events) > 0 && len(events[0].Results) > 0 {
			if r.BodyIndex == 0 {
				rows = rowsFor(0, 0)
			} else if r.BodyIndex > 0 {
				rows = rowsFor(r.BodyIndex-1, 1)
			}
			fillBodies(events, rows, r.BodyIndex, r.BodyOnly)
		}
		if diagnostics {
			total := len(metadata)
			for _, payload := range rows {
				total += len(payload)
			}
			logx.SystemDiagnostic("article_page_cache stage=hit id=%q dict=%q start=%d size=%d bytes=%d events=%d elapsed_ms=%d transport=%q origin=%q format=%q hl=%t rung=%q count=%d order=%d q=%q body=%d",
				r.CacheID, r.Dictionary, pageStart, pageSize, total, len(events), time.Since(started).Milliseconds(),
				r.Transport, r.Origin, r.Format, r.Highlight, r.Rung, r.CacheCount, r.CacheOrder, r.Query, r.BodyIndex)
		}
		for _, event := range events {
			if err := emit(event); err != nil {
				return true, err
			}
		}
		return true, nil
	}

	var cached []byte
	cacheReady := false
	var waiting chan struct{}
	owner := false
	c.mu.Lock()
	if c.id == r.CacheID && c.openLocked() == nil {
		cacheReady = true
		if c.db.QueryRowContext(ctx, `SELECT payload FROM article_page WHERE cache_key=?`, string(key)).Scan(&cached) == nil {
			c.access++
			_, _ = c.db.ExecContext(ctx, `UPDATE article_page SET accessed=? WHERE cache_key=?`, c.access, string(key))
		}
		// Somebody is already rendering this page: wait for that render
		// instead of starting a second one over the same megabytes.
		if len(cached) == 0 {
			if pending, ok := c.flights[key]; ok {
				waiting = pending
			} else {
				waiting = make(chan struct{})
				if c.flights == nil {
					c.flights = make(map[string]chan struct{})
				}
				c.flights[key] = waiting
				owner = true
			}
		}
	}
	c.mu.Unlock()
	if waiting != nil && !owner {
		select {
		case <-waiting:
		case <-ctx.Done():
			return ctx.Err()
		}
		cached = nil
		c.mu.Lock()
		if c.id == r.CacheID && c.openLocked() == nil {
			_ = c.db.QueryRowContext(ctx, `SELECT payload FROM article_page WHERE cache_key=?`, string(key)).Scan(&cached)
		}
		c.mu.Unlock()
	}
	if len(cached) > 0 {
		if decoded, err := emitCached(cached); err != nil {
			return err
		} else if decoded {
			return nil
		}
	}
	if diagnostics {
		logx.SystemDiagnostic("article_page_cache stage=miss id=%q dict=%q start=%d size=%d cache_ready=%t reason=%s transport=%q origin=%q format=%q hl=%t rung=%q count=%d order=%d q=%q body=%d", r.CacheID, r.Dictionary, pageStart, pageSize, cacheReady, map[bool]string{true: "not_found", false: "inactive_or_unavailable"}[cacheReady], r.Transport, r.Origin, r.Format, r.Highlight, r.Rung, r.CacheCount, r.CacheOrder, r.Query, r.BodyIndex)
	}
	if owner {
		// This call owns the render: release the waiters when it is done,
		// whether it stored a page or not.
		defer func() {
			c.mu.Lock()
			if c.flights[key] == waiting {
				delete(c.flights, key)
			}
			c.mu.Unlock()
			close(waiting)
		}()
	}
	streamLive := r.BodyIndex == 0
	var events []SearchEvent
	err := load(func(event SearchEvent) error {
		events = append(events, event)
		if streamLive {
			return emit(event)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	emitWindow := func() error {
		if streamLive {
			return nil
		}
		stripBodies(events)
		for _, event := range events {
			if err := emit(event); err != nil {
				return err
			}
		}
		return nil
	}
	// A growing result list can change both Total and HasNext. Re-read it
	// until its ID scan is complete instead of freezing intermediate metadata.
	if r.CacheAction == "page" && (len(events) != 1 || events[0].T != "hit" || !events[0].Complete) {
		if diagnostics {
			logx.SystemDiagnostic("article_page_cache stage=skip id=%q dict=%q start=%d size=%d reason=incomplete_page events=%d", r.CacheID, r.Dictionary, pageStart, pageSize, len(events))
		}
		return emitWindow()
	}
	for _, event := range events {
		if event.Error != "" || event.Skipped {
			return emitWindow()
		}
	}
	metadataEvents := make([]SearchEvent, len(events))
	copy(metadataEvents, events)
	var bodyRows [][]byte
	stored := 0
	if len(metadataEvents) > 0 {
		metadataEvents[0].Results = make([]dict.Result, len(events[0].Results))
		copy(metadataEvents[0].Results, events[0].Results)
		for index := range metadataEvents[0].Results {
			payload, marshalErr := json.Marshal(events[0].Results[index])
			if marshalErr != nil {
				return emitWindow()
			}
			bodyRows = append(bodyRows, payload)
			stored += len(payload)
			metadataEvents[0].Results[index].Body = ""
		}
	}
	metadata, err := json.Marshal(metadataEvents)
	if err != nil || int64(len(metadata)+stored) > limit {
		if err == nil && diagnostics {
			logx.SystemDiagnostic("article_page_cache stage=skip id=%q dict=%q start=%d size=%d reason=over_limit bytes=%d limit=%d", r.CacheID, r.Dictionary, pageStart, pageSize, len(metadata)+stored, limit)
		}
		return emitWindow()
	}
	c.mu.Lock()
	if c.id == r.CacheID && c.openLocked() == nil {
		c.access++
		if _, err = c.db.ExecContext(ctx, `INSERT OR REPLACE INTO article_page(cache_key,dict_id,page_start,page_size,payload,bytes,accessed) VALUES(?,?,?,?,?,?,?)`, string(key), r.Dictionary, pageStart, pageSize, metadata, len(metadata)+stored, c.access); err == nil {
			_, _ = c.db.ExecContext(ctx, `DELETE FROM article_page_body WHERE cache_key=?`, string(key))
			for index, payload := range bodyRows {
				_, _ = c.db.ExecContext(ctx, `INSERT INTO article_page_body(cache_key,idx,payload,bytes) VALUES(?,?,?,?)`, string(key), index, payload, len(payload))
			}
		}
		if diagnostics {
			logx.SystemDiagnostic("article_page_cache stage=store id=%q dict=%q start=%d size=%d bytes=%d events=%d elapsed_ms=%d transport=%q origin=%q format=%q hl=%t rung=%q count=%d order=%d q=%q", r.CacheID, r.Dictionary, pageStart, pageSize, len(metadata)+stored, len(events), time.Since(started).Milliseconds(), r.Transport, r.Origin, r.Format, r.Highlight, r.Rung, r.CacheCount, r.CacheOrder, r.Query)
		}
		for {
			var total int64
			if c.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes),0) FROM (SELECT bytes FROM article_page UNION ALL SELECT bytes FROM article_page_body)`).Scan(&total) != nil || total <= limit {
				break
			}
			var evictedDict string
			var evictedStart, evictedSize, evictedBytes int64
			var evictedKey string
			if c.db.QueryRowContext(ctx, `SELECT cache_key,dict_id,page_start,page_size,bytes FROM article_page ORDER BY accessed LIMIT 1`).Scan(&evictedKey, &evictedDict, &evictedStart, &evictedSize, &evictedBytes) != nil {
				break
			}
			if _, err = c.db.ExecContext(ctx, `DELETE FROM article_page WHERE cache_key=?`, evictedKey); err != nil {
				break
			}
			_, _ = c.db.ExecContext(ctx, `DELETE FROM article_page_body WHERE cache_key=?`, evictedKey)
			if diagnostics {
				logx.SystemDiagnostic("article_page_cache stage=evict id=%q dict=%q start=%d size=%d bytes=%d stored_bytes_before=%d", r.CacheID, evictedDict, evictedStart, evictedSize, evictedBytes, total)
			}
		}
	}
	c.mu.Unlock()
	if streamLive {
		return nil
	}
	rows := make(map[int][]byte, len(bodyRows))
	for index, payload := range bodyRows {
		rows[index] = payload
	}
	fillBodies(metadataEvents, rows, r.BodyIndex, r.BodyOnly)
	for _, event := range metadataEvents {
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}
