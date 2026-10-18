// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
)

const (
	wildcardCacheMaxAge  = 30 * time.Minute
	wildcardCacheDirName = "wildcards"
)

var wildcardCacheIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,80}$`)

var errWildcardCacheStreamReplaced = errors.New("wildcard cache stream replaced")

type wildcardCacheSignature struct {
	query     string
	group     string
	wildcards bool
}

type wildcardCacheEntry struct {
	mu        sync.Mutex
	db        *sql.DB
	path      string
	signature wildcardCacheSignature
	pageSize  int
	dictCount int
	active    int
	updated   time.Time
	nextToken uint64
	streams   map[wildcardStreamKey]uint64
}

type wildcardStreamKey struct {
	dictID     string
	within     string
	withinMask bool
}

type wildcardPageCache struct {
	mu          sync.Mutex
	dir         string
	entries     map[string]*wildcardCacheEntry
	cleanedTemp bool
}

func (s *Server) wildcardCacheDir() string {
	if s.User != "" {
		return filepath.Join(string(s.User), "cache", wildcardCacheDirName)
	}
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "wudict", wildcardCacheDirName)
	}
	return filepath.Join(os.TempDir(), "wudict", wildcardCacheDirName)
}

func (c *wildcardPageCache) init(dir string) {
	c.mu.Lock()
	c.dir = dir
	c.cleanupLocked(time.Now())
	c.mu.Unlock()
}

func (c *wildcardPageCache) begin(id string, signature wildcardCacheSignature, pageSize, dictCount int,
	dictID, dictName string, dictOrder int, within string, withinMask bool, dir string) (*wildcardCacheEntry, uint64, error) {
	if !wildcardCacheIDPattern.MatchString(id) {
		return nil, 0, errors.New("invalid wildcard cache id")
	}
	if pageSize < 1 || dictCount < 1 || dictOrder < 0 || dictOrder >= dictCount {
		return nil, 0, errors.New("invalid wildcard cache layout")
	}
	now := time.Now()
	c.mu.Lock()
	c.dir = dir
	c.cleanupLocked(now)
	if c.entries == nil {
		c.entries = make(map[string]*wildcardCacheEntry)
	}
	entry := c.entries[id]
	if entry == nil {
		var err error
		entry, err = openWildcardCache(id, signature, pageSize, dictCount, dir)
		if err != nil {
			c.mu.Unlock()
			return nil, 0, err
		}
		c.entries[id] = entry
	}
	entry.mu.Lock()
	if entry.signature != signature || entry.pageSize != pageSize || entry.dictCount != dictCount {
		entry.mu.Unlock()
		c.mu.Unlock()
		return nil, 0, errors.New("wildcard cache does not match this search")
	}
	entry.active++
	entry.updated = now
	_, err := entry.db.Exec(`INSERT INTO wildcard_dictionary(within_text,within_wildcards,dict_id,name,dict_order,total,complete,alphabet_json)
		VALUES(?,?,?,?,?,0,0,'[]') ON CONFLICT(within_text,within_wildcards,dict_id) DO UPDATE SET
		name=excluded.name,dict_order=excluded.dict_order,total=0,complete=0,alphabet_json='[]'`,
		within, boolInt(withinMask), dictID, dictName, dictOrder)
	if err == nil {
		_, err = entry.db.Exec(`DELETE FROM wildcard_block WHERE within_text=? AND within_wildcards=? AND dict_id=?`, within, boolInt(withinMask), dictID)
	}
	if err == nil {
		_, err = entry.db.Exec(`UPDATE wildcard_cache_meta SET accessed_ms=?`, now.UnixMilli())
	}
	var token uint64
	if err == nil {
		if entry.streams == nil {
			entry.streams = make(map[wildcardStreamKey]uint64)
		}
		entry.nextToken++
		token = entry.nextToken
		entry.streams[wildcardStreamKey{dictID: dictID, within: within, withinMask: withinMask}] = token
	}
	entry.mu.Unlock()
	c.mu.Unlock()
	if err != nil {
		c.finish(entry, nil, 0)
		return nil, 0, err
	}
	return entry, token, nil
}

func openWildcardCache(id string, signature wildcardCacheSignature, pageSize, dictCount int, dir string) (*wildcardCacheEntry, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create wildcard cache directory: %w", err)
	}
	path := filepath.Join(dir, "wildcard_"+id+".db")
	_, statErr := os.Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return nil, fmt.Errorf("open wildcard cache database: %w", err)
	}
	entry := &wildcardCacheEntry{db: db, path: path, signature: signature, pageSize: pageSize,
		dictCount: dictCount, updated: time.Now()}
	if os.IsNotExist(statErr) {
		if err := entry.createSchema(id); err != nil {
			entry.closeAndRemoveLocked()
			return nil, err
		}
		return entry, nil
	}
	var saved wildcardCacheSignature
	var savedID string
	var savedPageSize, savedDictCount int
	err = db.QueryRow(`SELECT cache_id,query_text,group_id,wildcards,page_size,dict_count
		FROM wildcard_cache_meta LIMIT 1`).Scan(&savedID, &saved.query, &saved.group, &saved.wildcards,
		&savedPageSize, &savedDictCount)
	if err != nil || savedID != id || saved != signature || savedPageSize != pageSize || savedDictCount != dictCount {
		if err != nil {
			_ = db.Close()
			removeWildcardCacheFiles(path)
			db, err = store.OpenWildcardCacheDB(path)
			if err != nil {
				return nil, fmt.Errorf("recreate wildcard cache database: %w", err)
			}
			entry.db = db
			if err := entry.createSchema(id); err != nil {
				entry.closeAndRemoveLocked()
				return nil, err
			}
			return entry, nil
		}
		_ = db.Close()
		return nil, errors.New("wildcard cache metadata mismatch")
	}
	var accessed int64
	if err := db.QueryRow(`SELECT accessed_ms FROM wildcard_cache_meta LIMIT 1`).Scan(&accessed); err == nil {
		entry.updated = time.UnixMilli(accessed)
	}
	return entry, nil
}

func (e *wildcardCacheEntry) createSchema(id string) error {
	statements := []string{
		`PRAGMA user_version=1`,
		`CREATE TABLE wildcard_cache_meta(
			cache_id TEXT PRIMARY KEY, query_text TEXT NOT NULL,
			group_id TEXT NOT NULL, wildcards INTEGER NOT NULL,
			page_size INTEGER NOT NULL, dict_count INTEGER NOT NULL,
			created_ms INTEGER NOT NULL, accessed_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE wildcard_dictionary(
			within_text TEXT NOT NULL, within_wildcards INTEGER NOT NULL,
			dict_id TEXT NOT NULL, name TEXT NOT NULL, dict_order INTEGER NOT NULL,
			total INTEGER NOT NULL, complete INTEGER NOT NULL, alphabet_json TEXT NOT NULL,
			PRIMARY KEY(within_text,within_wildcards,dict_id)
		)`,
		`CREATE TABLE wildcard_block(
			id INTEGER PRIMARY KEY, within_text TEXT NOT NULL, within_wildcards INTEGER NOT NULL,
			dict_id TEXT NOT NULL, dict_order INTEGER NOT NULL,
			block_no INTEGER NOT NULL, words_json TEXT NOT NULL, positions_json TEXT NOT NULL,
			word_count INTEGER NOT NULL,
			UNIQUE(within_text,within_wildcards,dict_id,block_no)
		)`,
		`CREATE INDEX wildcard_block_display ON wildcard_block(within_text,within_wildcards,dict_order,block_no)`,
	}
	for _, statement := range statements {
		if _, err := e.db.Exec(statement); err != nil {
			return fmt.Errorf("create wildcard cache schema: %w", err)
		}
	}
	_, err := e.db.Exec(`INSERT INTO wildcard_cache_meta(cache_id,query_text,group_id,wildcards,
		page_size,dict_count,created_ms,accessed_ms) VALUES(?,?,?,?,?,?,?,?)`,
		id, e.signature.query, e.signature.group, boolInt(e.signature.wildcards),
		e.pageSize, e.dictCount, e.updated.UnixMilli(), e.updated.UnixMilli())
	if err != nil {
		return fmt.Errorf("create wildcard cache schema: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (c *wildcardPageCache) page(id string, signature wildcardCacheSignature, dictID, within string,
	withinMask bool, page, dictOrder, dictCount int) (browseFindResp, bool, error) {
	if !wildcardCacheIDPattern.MatchString(id) || page < 1 {
		return browseFindResp{}, false, nil
	}
	c.mu.Lock()
	c.cleanupLocked(time.Now())
	entry := c.entries[id]
	if entry == nil {
		var err error
		entry, err = openExistingWildcardCache(id, signature, c.dir)
		if err != nil {
			c.mu.Unlock()
			if strings.Contains(err.Error(), "metadata mismatch") || os.IsNotExist(err) {
				return browseFindResp{}, false, nil
			}
			return browseFindResp{}, false, err
		}
		c.entries[id] = entry
	}
	entry.mu.Lock()
	c.mu.Unlock()
	defer entry.mu.Unlock()
	if entry.signature != signature || entry.dictCount != dictCount {
		return browseFindResp{}, false, nil
	}
	var total, complete, savedOrder int
	var alphabetJSON string
	err := entry.db.QueryRow(`SELECT total,complete,alphabet_json,dict_order FROM wildcard_dictionary
		WHERE within_text=? AND within_wildcards=? AND dict_id=?`, within, boolInt(withinMask), dictID).
		Scan(&total, &complete, &alphabetJSON, &savedOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return browseFindResp{}, false, nil
	}
	if err != nil {
		return browseFindResp{}, false, err
	}
	if savedOrder != dictOrder {
		return browseFindResp{}, false, nil
	}
	var record wildcardCachePage
	var wordsJSON, positionsJSON string
	err = entry.db.QueryRow(`SELECT words_json,positions_json FROM wildcard_block
		WHERE within_text=? AND within_wildcards=? AND dict_id=? AND block_no=?`, within, boolInt(withinMask), dictID, page).
		Scan(&wordsJSON, &positionsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return browseFindResp{}, false, nil
	}
	if err != nil {
		return browseFindResp{}, false, err
	}
	if err := json.Unmarshal([]byte(wordsJSON), &record.Words); err != nil {
		return browseFindResp{}, false, err
	}
	if err := json.Unmarshal([]byte(positionsJSON), &record.Positions); err != nil {
		return browseFindResp{}, false, err
	}
	if err := json.Unmarshal([]byte(alphabetJSON), &record.Alphabet); err != nil {
		return browseFindResp{}, false, err
	}
	entry.updated = time.Now()
	_, _ = entry.db.Exec(`UPDATE wildcard_cache_meta SET accessed_ms=?`, entry.updated.UnixMilli())
	_ = os.Chtimes(entry.path, entry.updated, entry.updated)
	pages := (total + entry.pageSize - 1) / entry.pageSize
	return browseFindResp{Words: record.Words, Positions: record.Positions, Alphabet: record.Alphabet,
		Total: total, Page: page, Pages: pages, Size: entry.pageSize, Partial: complete == 0}, true, nil
}

func openExistingWildcardCache(id string, signature wildcardCacheSignature, dir string) (*wildcardCacheEntry, error) {
	path := filepath.Join(dir, "wildcard_"+id+".db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return nil, err
	}
	var saved wildcardCacheSignature
	var savedID string
	var pageSize, dictCount int
	var accessed int64
	err = db.QueryRow(`SELECT cache_id,query_text,group_id,wildcards,page_size,dict_count,accessed_ms
		FROM wildcard_cache_meta LIMIT 1`).Scan(&savedID, &saved.query, &saved.group,
		&saved.wildcards, &pageSize, &dictCount, &accessed)
	if err != nil || savedID != id || saved != signature {
		_ = db.Close()
		if err != nil {
			removeWildcardCacheFiles(path)
			return nil, fmt.Errorf("wildcard cache metadata mismatch: %w", err)
		}
		return nil, errors.New("wildcard cache metadata mismatch")
	}
	return &wildcardCacheEntry{db: db, path: path, signature: saved, pageSize: pageSize,
		dictCount: dictCount, updated: time.UnixMilli(accessed)}, nil
}

type wildcardCachePage struct {
	Words     []string      `json:"words"`
	Positions []int         `json:"positions,omitempty"`
	Alphabet  []dict.Letter `json:"alphabet,omitempty"`
}

// storePage caches a page that came directly from a prepared headword index.
// Match-all wildcard searches already know their exact total and alphabet, so
// they can page on demand instead of scanning every key to build the cache.
func (c *wildcardPageCache) storePage(id string, signature wildcardCacheSignature, pageSize, dictCount int,
	dictID, dictName string, dictOrder, page int, result browseFindResp, dir string) error {
	if !wildcardCacheIDPattern.MatchString(id) || pageSize < 1 || dictCount < 1 ||
		dictOrder < 0 || dictOrder >= dictCount || page < 1 {
		return errors.New("invalid wildcard cache page")
	}
	now := time.Now()
	c.mu.Lock()
	c.dir = dir
	c.cleanupLocked(now)
	entry := c.entries[id]
	if entry == nil {
		var err error
		entry, err = openWildcardCache(id, signature, pageSize, dictCount, dir)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		c.entries[id] = entry
	}
	entry.mu.Lock()
	c.mu.Unlock()
	defer entry.mu.Unlock()
	if entry.signature != signature || entry.pageSize != pageSize || entry.dictCount != dictCount {
		return errors.New("wildcard cache does not match this search")
	}
	words, err := json.Marshal(result.Words)
	if err != nil {
		return err
	}
	positions, err := json.Marshal(result.Positions)
	if err != nil {
		return err
	}
	alphabet, err := json.Marshal(result.Alphabet)
	if err != nil {
		return err
	}
	tx, err := entry.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var wasComplete int
	err = tx.QueryRow(`SELECT complete FROM wildcard_dictionary WHERE within_text='' AND within_wildcards=0 AND dict_id=?`, dictID).Scan(&wasComplete)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && wasComplete == 0 {
		if _, err := tx.Exec(`DELETE FROM wildcard_block WHERE within_text='' AND within_wildcards=0 AND dict_id=?`, dictID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO wildcard_dictionary(within_text,within_wildcards,dict_id,name,dict_order,total,complete,alphabet_json)
		VALUES('',0,?,?,?,?,1,?) ON CONFLICT(within_text,within_wildcards,dict_id) DO UPDATE SET
		name=excluded.name,dict_order=excluded.dict_order,total=excluded.total,complete=1,alphabet_json=excluded.alphabet_json`,
		dictID, dictName, dictOrder, result.Total, string(alphabet)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wildcard_block(within_text,within_wildcards,dict_id,dict_order,block_no,words_json,positions_json,word_count)
		VALUES('',0,?,?,?,?,?,?) ON CONFLICT(within_text,within_wildcards,dict_id,block_no) DO UPDATE SET
		dict_order=excluded.dict_order,words_json=excluded.words_json,positions_json=excluded.positions_json,word_count=excluded.word_count`,
		dictID, dictOrder, page, string(words), string(positions), len(result.Words)); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE wildcard_cache_meta SET accessed_ms=?`, now.UnixMilli()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	entry.updated = now
	_ = os.Chtimes(entry.path, now, now)
	return nil
}

func (e *wildcardCacheEntry) append(dictID, within string, withinMask bool, token uint64, event browseFindStreamEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := wildcardStreamKey{dictID: dictID, within: within, withinMask: withinMask}
	if e.streams[key] != token {
		return errWildcardCacheStreamReplaced
	}
	e.updated = time.Now()
	if len(event.Words) > 0 {
		words, err := json.Marshal(event.Words)
		if err != nil {
			return err
		}
		positions, err := json.Marshal(event.Positions)
		if err != nil {
			return err
		}
		var blockNo int
		if err := e.db.QueryRow(`SELECT COALESCE(MAX(block_no),0)+1 FROM wildcard_block
			WHERE within_text=? AND within_wildcards=? AND dict_id=?`, within, boolInt(withinMask), dictID).Scan(&blockNo); err != nil {
			return err
		}
		var dictOrder int
		if err := e.db.QueryRow(`SELECT dict_order FROM wildcard_dictionary WHERE within_text=? AND within_wildcards=? AND dict_id=?`,
			within, boolInt(withinMask), dictID).Scan(&dictOrder); err != nil {
			return err
		}
		if _, err := e.db.Exec(`INSERT INTO wildcard_block(within_text,within_wildcards,dict_id,dict_order,block_no,words_json,positions_json,word_count)
			VALUES(?,?,?,?,?,?,?,?)`, within, boolInt(withinMask), dictID, dictOrder, blockNo, string(words), string(positions), len(event.Words)); err != nil {
			return err
		}
	}
	alphabet, err := json.Marshal(event.Alphabet)
	if err != nil {
		return err
	}
	complete := boolInt(event.Done)
	if _, err := e.db.Exec(`UPDATE wildcard_dictionary SET total=?,complete=?,alphabet_json=?
		WHERE within_text=? AND within_wildcards=? AND dict_id=?`,
		event.Total, complete, string(alphabet), within, boolInt(withinMask), dictID); err != nil {
		return err
	}
	if _, err := e.db.Exec(`UPDATE wildcard_cache_meta SET accessed_ms=?`, e.updated.UnixMilli()); err != nil {
		return err
	}
	return nil
}

func (e *wildcardCacheEntry) closeAndRemoveLocked() {
	if e.db != nil {
		_ = e.db.Close()
		e.db = nil
	}
	removeWildcardCacheFiles(e.path)
}

func removeWildcardCacheFiles(path string) {
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}

func (c *wildcardPageCache) finish(entry *wildcardCacheEntry, key *wildcardStreamKey, token uint64) {
	entry.mu.Lock()
	if entry.active > 0 {
		entry.active--
	}
	if key != nil && entry.streams[*key] == token {
		delete(entry.streams, *key)
	}
	entry.updated = time.Now()
	_, _ = entry.db.Exec(`UPDATE wildcard_cache_meta SET accessed_ms=?`, entry.updated.UnixMilli())
	_ = os.Chtimes(entry.path, entry.updated, entry.updated)
	entry.mu.Unlock()
}

func (c *wildcardPageCache) cleanupLocked(now time.Time) {
	if c.entries == nil {
		c.entries = make(map[string]*wildcardCacheEntry)
	}
	for id, entry := range c.entries {
		entry.mu.Lock()
		expired := entry.active == 0 && now.Sub(entry.updated) > wildcardCacheMaxAge
		if expired {
			entry.closeAndRemoveLocked()
			delete(c.entries, id)
		}
		entry.mu.Unlock()
	}
	if c.cleanedTemp {
		return
	}
	c.cleanedTemp = true
	if c.dir == "" {
		return
	}
	items, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, item := range items {
		if item.IsDir() || !strings.HasPrefix(item.Name(), "wildcard_") || !strings.HasSuffix(item.Name(), ".db") {
			continue
		}
		info, err := item.Info()
		if err == nil && now.Sub(info.ModTime()) > wildcardCacheMaxAge {
			path := filepath.Join(c.dir, item.Name())
			removeWildcardCacheFiles(path)
		}
	}
}
