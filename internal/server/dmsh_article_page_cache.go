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

	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
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
}

func (c *articlePageCache) resetFor(id string) {
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
	logx.System("article_page_cache stage=reset id=%q previous_id=%q", id, previous)
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
	}
	if err != nil {
		_ = db.Close()
		return err
	}
	c.db = db
	return nil
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
	key, _ := json.Marshal(struct {
		SearchRequest
		Origin string
	}{r, r.Origin})
	var cached []byte
	cacheReady := false
	c.mu.Lock()
	if c.id == r.CacheID && c.openLocked() == nil {
		cacheReady = true
		if c.db.QueryRowContext(ctx, `SELECT payload FROM article_page WHERE cache_key=?`, string(key)).Scan(&cached) == nil {
			c.access++
			_, _ = c.db.ExecContext(ctx, `UPDATE article_page SET accessed=? WHERE cache_key=?`, c.access, string(key))
		}
	}
	c.mu.Unlock()
	if len(cached) > 0 {
		var events []SearchEvent
		if json.Unmarshal(cached, &events) == nil {
			logx.System("article_page_cache stage=hit id=%q dict=%q start=%d size=%d bytes=%d events=%d elapsed_ms=%d", r.CacheID, r.Dictionary, pageStart, pageSize, len(cached), len(events), time.Since(started).Milliseconds())
			for _, event := range events {
				if err := emit(event); err != nil {
					return err
				}
			}
			return nil
		}
	}
	logx.System("article_page_cache stage=miss id=%q dict=%q start=%d size=%d cache_ready=%t reason=%s", r.CacheID, r.Dictionary, pageStart, pageSize, cacheReady, map[bool]string{true: "not_found", false: "inactive_or_unavailable"}[cacheReady])
	var events []SearchEvent
	err := load(func(event SearchEvent) error {
		if err := emit(event); err != nil {
			return err
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// A growing result list can change both Total and HasNext. Re-read it
	// until its ID scan is complete instead of freezing intermediate metadata.
	if r.CacheAction == "page" && (len(events) != 1 || events[0].T != "hit" || !events[0].Complete) {
		logx.System("article_page_cache stage=skip id=%q dict=%q start=%d size=%d reason=incomplete_page events=%d", r.CacheID, r.Dictionary, pageStart, pageSize, len(events))
		return nil
	}
	for _, event := range events {
		if event.Error != "" || event.Skipped || event.Deferred {
			return nil
		}
	}
	payload, err := json.Marshal(events)
	if err != nil || int64(len(payload)) > limit {
		if err == nil {
			logx.System("article_page_cache stage=skip id=%q dict=%q start=%d size=%d reason=over_limit bytes=%d limit=%d", r.CacheID, r.Dictionary, pageStart, pageSize, len(payload), limit)
		}
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id != r.CacheID || c.openLocked() != nil {
		return nil
	}
	c.access++
	if _, err = c.db.ExecContext(ctx, `INSERT OR REPLACE INTO article_page(cache_key,dict_id,page_start,page_size,payload,bytes,accessed) VALUES(?,?,?,?,?,?,?)`, string(key), r.Dictionary, pageStart, pageSize, payload, len(payload), c.access); err != nil {
		return nil
	}
	logx.System("article_page_cache stage=store id=%q dict=%q start=%d size=%d bytes=%d events=%d elapsed_ms=%d", r.CacheID, r.Dictionary, pageStart, pageSize, len(payload), len(events), time.Since(started).Milliseconds())
	for {
		var total int64
		if c.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes),0) FROM article_page`).Scan(&total) != nil || total <= limit {
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
		logx.System("article_page_cache stage=evict id=%q dict=%q start=%d size=%d bytes=%d stored_bytes_before=%d", r.CacheID, evictedDict, evictedStart, evictedSize, evictedBytes, total)
	}
	return nil
}
