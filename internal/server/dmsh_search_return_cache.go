// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
)

const (
	searchReturnCacheMaxEntries = 5
	searchReturnCacheMaxAge     = 30 * time.Minute
)

var errSearchReturnCacheUnavailable = errors.New("saved search results are unavailable")
var errSearchReturnCacheEmit = errors.New("could not emit saved search result")

type searchReturnCacheEntry struct {
	mu           sync.Mutex
	db           *sql.DB
	id           string
	storage      string
	created      time.Time
	nextSeq      int64
	payloadBytes int64
	articleBytes int64
	sqliteBytes  int64
	eventCounts  map[string]int
	complete     bool
	interrupted  bool
	insert       *sql.Stmt
}

type searchReturnCacheStore struct {
	mu      sync.Mutex
	entries map[string]*searchReturnCacheEntry
}

func (s *Server) beginSearchReturnCache(id, storage string) (*searchReturnCacheEntry, error) {
	if !wildcardCacheIDPattern.MatchString(id) {
		return nil, errors.New("invalid search return cache id")
	}
	prefs := s.uiPrefsSnapshot()
	if storage != "memory" && storage != "disk" {
		storage = ""
	}
	memory := storage == "memory" || storage == "" && prefs != nil && prefs.SearchReturnCache == "memory"
	db, err := store.OpenTransientCacheDB(memory)
	if err != nil {
		return nil, fmt.Errorf("open transient search return cache: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE search_return_event(seq INTEGER PRIMARY KEY,event_json BLOB NOT NULL)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create transient search return cache: %w", err)
	}
	statement, err := db.Prepare(`INSERT INTO search_return_event(seq,event_json) VALUES(?,?)`)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("prepare transient search return cache: %w", err)
	}
	entry := &searchReturnCacheEntry{
		db: db, id: id, storage: map[bool]string{true: "memory", false: "disk-temp"}[memory],
		created: time.Now(), insert: statement, eventCounts: make(map[string]int),
	}
	s.searchReturns.mu.Lock()
	if s.searchReturns.entries == nil {
		s.searchReturns.entries = make(map[string]*searchReturnCacheEntry)
	}
	if previous := s.searchReturns.entries[id]; previous != nil {
		previous.close("replaced")
	}
	s.searchReturns.entries[id] = entry
	s.searchReturns.pruneLocked(time.Now(), id)
	s.searchReturns.mu.Unlock()
	entry.mu.Lock()
	entry.logStatsLocked("created", "")
	entry.mu.Unlock()
	return entry, nil
}

func (e *searchReturnCacheEntry) append(event SearchEvent, interrupted bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.db == nil || e.complete {
		return errors.New("search return cache is closed or complete")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	e.nextSeq++
	if _, err := e.insert.Exec(e.nextSeq, data); err != nil {
		return err
	}
	e.payloadBytes += int64(len(data))
	bodyBytes := searchReturnEventBodyBytes(event)
	e.articleBytes += bodyBytes
	e.eventCounts[event.T]++
	logSearchReturnCacheEvent("write_event", e.id, e.nextSeq, event, int64(len(data)), bodyBytes, e.payloadBytes, e.articleBytes, time.Since(e.created))
	if event.T == "end" {
		e.complete = true
		e.interrupted = interrupted
		reason := "complete"
		if interrupted {
			reason = "interrupted"
		}
		e.logStatsLocked("stream_end", reason)
	}
	return nil
}

func (e *searchReturnCacheEntry) replay(ctx context.Context, emit func(SearchEvent) error) error {
	started := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.db == nil || !e.complete {
		logx.System("search_return_cache stage=replay_unavailable id=%q storage=%s db_open=%t complete=%t interrupted=%t events=%d age_ms=%d",
			e.id, e.storage, e.db != nil, e.complete, e.interrupted, e.nextSeq, time.Since(e.created).Milliseconds())
		return errSearchReturnCacheUnavailable
	}
	rows, err := e.db.QueryContext(ctx, `SELECT event_json FROM search_return_event ORDER BY seq`)
	if err != nil {
		logx.System("search_return_cache stage=replay_failed id=%q phase=query error=%q elapsed_ms=%d", e.id, err, time.Since(started).Milliseconds())
		return err
	}
	defer rows.Close()
	var replayed, payloadBytes, articleBytes int64
	eventCounts := make(map[string]int)
	replayResult := "hit"
	defer func() {
		logx.System("search_return_cache stage=replay_end id=%q result=%s source_interrupted=%t source_events=%d source_bytes=%d source_article_bytes=%d source_sqlite_bytes=%d replay_events=%d replay_bytes=%d replay_article_bytes=%d elapsed_ms=%d",
			e.id, replayResult, e.interrupted, e.nextSeq, e.payloadBytes, e.articleBytes, e.sqliteBytes, replayed, payloadBytes, articleBytes, time.Since(started).Milliseconds())
	}()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			replayResult = "read_error"
			return err
		}
		var event SearchEvent
		if err := json.Unmarshal(data, &event); err != nil {
			replayResult = "decode_error"
			return err
		}
		replayed++
		payloadBytes += int64(len(data))
		bodyBytes := searchReturnEventBodyBytes(event)
		articleBytes += bodyBytes
		eventCounts[event.T]++
		logSearchReturnCacheEvent("replay_event", e.id, replayed, event, int64(len(data)), bodyBytes, payloadBytes, articleBytes, time.Since(started))
		if err := emit(event); err != nil {
			replayResult = "emit_error"
			return fmt.Errorf("%w: %w", errSearchReturnCacheEmit, err)
		}
	}
	if err := rows.Err(); err != nil {
		replayResult = "read_error"
		return err
	}
	logx.System("search_return_cache stage=replay_counts id=%q begin=%d hit=%d morph=%d end=%d",
		e.id, eventCounts["begin"], eventCounts["hit"], eventCounts["morph"], eventCounts["end"])
	return nil
}

func (e *searchReturnCacheEntry) logStatsLocked(stage, reason string) {
	if e.db != nil {
		var pageCount, pageSize int64
		_ = e.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount)
		_ = e.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize)
		e.sqliteBytes = pageCount * pageSize
	}
	logx.System("search_return_cache stage=%s id=%q storage=%s reason=%q complete=%t interrupted=%t events=%d begin=%d hit=%d morph=%d end=%d payload_bytes=%d article_bytes=%d sqlite_bytes=%d age_ms=%d",
		stage, e.id, e.storage, reason, e.complete, e.interrupted, e.nextSeq,
		e.eventCounts["begin"], e.eventCounts["hit"], e.eventCounts["morph"], e.eventCounts["end"],
		e.payloadBytes, e.articleBytes, e.sqliteBytes, time.Since(e.created).Milliseconds())
}

func searchReturnEventBodyBytes(event SearchEvent) int64 {
	var total int64
	for _, result := range event.Results {
		total += int64(len(result.Body))
	}
	return total
}

func logSearchReturnCacheEvent(stage, id string, seq int64, event SearchEvent, eventBytes, bodyBytes, totalBytes, totalBodyBytes int64, elapsed time.Duration) {
	logx.System("search_return_cache stage=%s id=%q seq=%d event=%s slot=%d dict=%q name=%q results=%d event_bytes=%d article_bytes=%d cumulative_bytes=%d cumulative_article_bytes=%d elapsed_ms=%d",
		stage, id, seq, event.T, event.I, event.Dict, event.Name, len(event.Results), eventBytes, bodyBytes,
		totalBytes, totalBodyBytes, elapsed.Milliseconds())
}

func (e *searchReturnCacheEntry) close(reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.db == nil {
		return
	}
	e.logStatsLocked("close", reason)
	if e.insert != nil {
		_ = e.insert.Close()
		e.insert = nil
	}
	_ = e.db.Close()
	e.db = nil
}

func (s *searchReturnCacheStore) pruneLocked(now time.Time, keep string) {
	for id, entry := range s.entries {
		if now.Sub(entry.created) > searchReturnCacheMaxAge {
			entry.close("expired")
			delete(s.entries, id)
		}
	}
	for len(s.entries) > searchReturnCacheMaxEntries {
		oldestID := ""
		var oldest time.Time
		for id, entry := range s.entries {
			if id == keep {
				continue
			}
			if oldestID == "" || entry.created.Before(oldest) {
				oldestID, oldest = id, entry.created
			}
		}
		if oldestID == "" {
			return
		}
		s.entries[oldestID].close("entry_limit")
		delete(s.entries, oldestID)
	}
}

func (s *Server) replaySearchReturnCache(ctx context.Context, id string, emit func(SearchEvent) error) error {
	if !wildcardCacheIDPattern.MatchString(id) {
		logx.System("search_return_cache stage=replay_miss id=%q reason=invalid_id", id)
		return errSearchReturnCacheUnavailable
	}
	s.searchReturns.mu.Lock()
	var expired bool
	if current := s.searchReturns.entries[id]; current != nil {
		expired = time.Since(current.created) > searchReturnCacheMaxAge
	}
	if s.searchReturns.entries != nil {
		s.searchReturns.pruneLocked(time.Now(), "")
	}
	entry := s.searchReturns.entries[id]
	s.searchReturns.mu.Unlock()
	if entry == nil {
		reason := "not_found_or_evicted"
		if expired {
			reason = "expired"
		}
		logx.System("search_return_cache stage=replay_miss id=%q reason=%s", id, reason)
		return errSearchReturnCacheUnavailable
	}
	return entry.replay(ctx, emit)
}
