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
	mu       sync.Mutex
	db       *sql.DB
	id       string
	created  time.Time
	nextSeq  int64
	complete bool
	insert   *sql.Stmt
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
	entry := &searchReturnCacheEntry{db: db, id: id, created: time.Now(), insert: statement}
	s.searchReturns.mu.Lock()
	if s.searchReturns.entries == nil {
		s.searchReturns.entries = make(map[string]*searchReturnCacheEntry)
	}
	if previous := s.searchReturns.entries[id]; previous != nil {
		previous.close()
	}
	s.searchReturns.entries[id] = entry
	s.searchReturns.pruneLocked(time.Now(), id)
	s.searchReturns.mu.Unlock()
	logx.System("search_return_cache stage=create id=%q storage=%s", id, map[bool]string{true: "memory", false: "disk"}[memory])
	return entry, nil
}

func (e *searchReturnCacheEntry) append(event SearchEvent) error {
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
	if event.T == "end" {
		e.complete = true
	}
	return nil
}

func (e *searchReturnCacheEntry) replay(ctx context.Context, emit func(SearchEvent) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.db == nil || !e.complete {
		return errSearchReturnCacheUnavailable
	}
	rows, err := e.db.QueryContext(ctx, `SELECT event_json FROM search_return_event ORDER BY seq`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return err
		}
		var event SearchEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		if err := emit(event); err != nil {
			return fmt.Errorf("%w: %w", errSearchReturnCacheEmit, err)
		}
	}
	return rows.Err()
}

func (e *searchReturnCacheEntry) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.insert != nil {
		_ = e.insert.Close()
		e.insert = nil
	}
	if e.db != nil {
		_ = e.db.Close()
		e.db = nil
	}
}

func (s *searchReturnCacheStore) pruneLocked(now time.Time, keep string) {
	for id, entry := range s.entries {
		if id != keep && now.Sub(entry.created) > searchReturnCacheMaxAge {
			entry.close()
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
		s.entries[oldestID].close()
		delete(s.entries, oldestID)
	}
}

func (s *Server) replaySearchReturnCache(ctx context.Context, id string, emit func(SearchEvent) error) error {
	if !wildcardCacheIDPattern.MatchString(id) {
		return errSearchReturnCacheUnavailable
	}
	s.searchReturns.mu.Lock()
	if s.searchReturns.entries != nil {
		s.searchReturns.pruneLocked(time.Now(), id)
	}
	entry := s.searchReturns.entries[id]
	s.searchReturns.mu.Unlock()
	if entry == nil {
		return errSearchReturnCacheUnavailable
	}
	return entry.replay(ctx, emit)
}
