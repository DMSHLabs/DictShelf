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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/indexconst"
	"github.com/wuweidict/wudict/internal/store"
)

const (
	wildcardCacheMaxAge  = time.Duration(indexconst.ResultCacheMaxAgeDays) * 24 * time.Hour
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
	activePaths map[string]int
	maxBytes    int64 // zero means unlimited
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
	c.pruneLocked(time.Now(), "", true)
	c.mu.Unlock()
}

func (c *wildcardPageCache) begin(id string, signature wildcardCacheSignature, pageSize, dictCount int,
	dictID, dictName string, dictOrder int, within string, withinMask bool, dir string) (*wildcardCacheEntry, uint64, error) {
	if !wildcardCacheIDPattern.MatchString(id) {
		return nil, 0, errors.New("invalid wildcard cache id")
	}
	if pageSize != indexconst.WildcardCacheBlockSize || pageSize%indexconst.BrowseAnchorStride != 0 ||
		dictCount < 1 || dictOrder < 0 || dictOrder >= dictCount {
		return nil, 0, errors.New("invalid wildcard cache layout")
	}
	now := time.Now()
	c.mu.Lock()
	c.dir = dir
	if c.entries == nil {
		c.entries = make(map[string]*wildcardCacheEntry)
	}
	entry := c.entries[id]
	if entry == nil {
		c.pruneLocked(now, filepath.Join(dir, "wildcard_"+id+".db"), false)
		var err error
		entry, err = openWildcardCache(id, signature, pageSize, dictCount, dir)
		if err != nil {
			c.mu.Unlock()
			return nil, 0, err
		}
		c.entries[id] = entry
		c.pruneLocked(now, entry.path, false)
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
	if err := prepareWildcardCacheDB(db, pageSize); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("prepare wildcard cache database: %w", err)
	}
	var accessed int64
	if err := db.QueryRow(`SELECT accessed_ms FROM wildcard_cache_meta LIMIT 1`).Scan(&accessed); err == nil {
		entry.updated = time.UnixMilli(accessed)
	}
	return entry, nil
}

func (e *wildcardCacheEntry) createSchema(id string) error {
	statements := []string{
		`PRAGMA user_version=3`,
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
			start_offset INTEGER NOT NULL, first_position INTEGER, last_position INTEGER,
			UNIQUE(within_text,within_wildcards,dict_id,block_no)
		)`,
		`CREATE INDEX wildcard_block_display ON wildcard_block(within_text,within_wildcards,dict_order,block_no)`,
		`CREATE INDEX wildcard_block_start_offset ON wildcard_block(within_text,within_wildcards,dict_id,start_offset)`,
		`CREATE INDEX wildcard_block_first_position ON wildcard_block(within_text,within_wildcards,dict_id,first_position)`,
		`CREATE INDEX wildcard_block_last_position ON wildcard_block(within_text,within_wildcards,dict_id,last_position)`,
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

// Existing version-2 caches already store stream offsets. Match-all Browse
// pages predate that column's use, so their offsets can be derived from page
// number without walking earlier blocks.
func prepareWildcardCacheDB(db *sql.DB, pageSize int) error {
	columns := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(wildcard_block)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	legacyOffsets := !columns["start_offset"]
	for name, definition := range map[string]string{"start_offset": "INTEGER", "first_position": "INTEGER", "last_position": "INTEGER"} {
		if !columns[name] {
			if _, err := db.Exec(`ALTER TABLE wildcard_block ADD COLUMN ` + name + ` ` + definition); err != nil {
				return err
			}
		}
	}
	if legacyOffsets {
		if _, err := db.Exec(`WITH offsets AS (
			SELECT id,COALESCE(SUM(word_count) OVER(PARTITION BY within_text,within_wildcards,dict_id ORDER BY block_no ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING),0) AS start
			FROM wildcard_block
		) UPDATE wildcard_block SET start_offset=(SELECT start FROM offsets WHERE offsets.id=wildcard_block.id)`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS wildcard_block_start_offset
		ON wildcard_block(within_text,within_wildcards,dict_id,start_offset)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS wildcard_block_first_position
		ON wildcard_block(within_text,within_wildcards,dict_id,first_position)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS wildcard_block_last_position
		ON wildcard_block(within_text,within_wildcards,dict_id,last_position)`); err != nil {
		return err
	}
	if _, err = db.Exec(`UPDATE wildcard_block SET start_offset=(block_no-1)*?
		WHERE start_offset IS NULL AND within_text='' AND within_wildcards=0`, pageSize); err != nil {
		return err
	}
	_, err = db.Exec(`PRAGMA user_version=3`)
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func strictlyIncreasingPositions(positions []int) bool {
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			return false
		}
	}
	return true
}

func (c *wildcardPageCache) page(id string, signature wildcardCacheSignature, dictID, within string,
	withinMask bool, page, dictOrder, dictCount int) (browseFindResp, bool, error) {
	if !wildcardCacheIDPattern.MatchString(id) || page < 1 {
		return browseFindResp{}, false, nil
	}
	c.mu.Lock()
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
	start := (page - 1) * entry.pageSize
	end := start + entry.pageSize
	// Older caches may have variable-sized stream rows. A result row is never
	// larger than one page, so seek from at most one page before the target;
	// newly written caches keep one row per page and return exactly one row.
	lower := max(0, start-entry.pageSize+1)
	rows, err := entry.db.Query(`SELECT words_json,positions_json,start_offset FROM wildcard_block
		WHERE within_text=? AND within_wildcards=? AND dict_id=?
		AND start_offset>=? AND start_offset<? AND start_offset+word_count>?
		ORDER BY start_offset`, within, boolInt(withinMask), dictID, lower, end, start)
	if err != nil {
		return browseFindResp{}, false, err
	}
	var record wildcardCachePage
	for rows.Next() {
		var wordsJSON, positionsJSON string
		var blockStart int
		if err := rows.Scan(&wordsJSON, &positionsJSON, &blockStart); err != nil {
			_ = rows.Close()
			return browseFindResp{}, false, err
		}
		var words []string
		var positions []int
		if err := json.Unmarshal([]byte(wordsJSON), &words); err != nil {
			_ = rows.Close()
			return browseFindResp{}, false, err
		}
		if err := json.Unmarshal([]byte(positionsJSON), &positions); err != nil {
			_ = rows.Close()
			return browseFindResp{}, false, err
		}
		from := max(0, start-blockStart)
		to := min(len(words), end-blockStart)
		if from < to {
			record.Words = append(record.Words, words[from:to]...)
			if len(positions) >= to {
				record.Positions = append(record.Positions, positions[from:to]...)
			}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return browseFindResp{}, false, err
	}
	if err := rows.Close(); err != nil {
		return browseFindResp{}, false, err
	}
	if len(record.Words) == 0 {
		return browseFindResp{}, false, nil
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

func (c *wildcardPageCache) findPosition(id string, signature wildcardCacheSignature, dictID, within string,
	withinMask bool, position, direction, dictOrder, dictCount int) (int, bool, error) {
	if !wildcardCacheIDPattern.MatchString(id) || position < 0 {
		return 0, false, nil
	}
	c.mu.Lock()
	entry := c.entries[id]
	if entry == nil {
		var err error
		entry, err = openExistingWildcardCache(id, signature, c.dir)
		if err != nil {
			c.mu.Unlock()
			if os.IsNotExist(err) || strings.Contains(err.Error(), "metadata mismatch") {
				return 0, false, nil
			}
			return 0, false, err
		}
		c.entries[id] = entry
	}
	entry.mu.Lock()
	c.mu.Unlock()
	defer entry.mu.Unlock()
	if entry.signature != signature || entry.dictCount != dictCount {
		return 0, false, nil
	}
	var savedOrder int
	err := entry.db.QueryRow(`SELECT dict_order FROM wildcard_dictionary
		WHERE within_text=? AND within_wildcards=? AND dict_id=?`, within, boolInt(withinMask), dictID).Scan(&savedOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if savedOrder != dictOrder {
		return 0, false, nil
	}
	condition, order := "last_position>?", "last_position"
	if direction == 0 {
		condition = "last_position>=?"
	} else if direction < 0 {
		condition, order = "first_position<?", "first_position DESC"
	}
	var encoded string
	var offset, count int
	err = entry.db.QueryRow(`SELECT positions_json,start_offset,word_count FROM wildcard_block
		WHERE within_text=? AND within_wildcards=? AND dict_id=? AND `+condition+` ORDER BY `+order+` LIMIT 1`,
		within, boolInt(withinMask), dictID, position).Scan(&encoded, &offset, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var positions []int
	if err := json.Unmarshal([]byte(encoded), &positions); err != nil {
		return 0, false, err
	}
	if len(positions) != count {
		return 0, false, errors.New("wildcard cache match positions do not match block length")
	}
	index := sort.Search(len(positions), func(i int) bool { return positions[i] >= position })
	if direction > 0 {
		index = sort.Search(len(positions), func(i int) bool { return positions[i] > position })
	} else if direction < 0 {
		index--
	}
	if index < 0 || index >= len(positions) || direction == 0 && positions[index] != position {
		return 0, false, nil
	}
	entry.updated = time.Now()
	_, _ = entry.db.Exec(`UPDATE wildcard_cache_meta SET accessed_ms=?`, entry.updated.UnixMilli())
	_ = os.Chtimes(entry.path, entry.updated, entry.updated)
	return offset + index, true, nil
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
	if pageSize != indexconst.WildcardCacheBlockSize {
		_ = db.Close()
		return nil, errors.New("wildcard cache metadata mismatch: unsupported block size")
	}
	if err := prepareWildcardCacheDB(db, pageSize); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("prepare wildcard cache database: %w", err)
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
	if !wildcardCacheIDPattern.MatchString(id) || pageSize != indexconst.WildcardCacheBlockSize || dictCount < 1 ||
		dictOrder < 0 || dictOrder >= dictCount || page < 1 {
		return errors.New("invalid wildcard cache page")
	}
	now := time.Now()
	c.mu.Lock()
	c.dir = dir
	entry := c.entries[id]
	if entry == nil {
		c.pruneLocked(now, filepath.Join(dir, "wildcard_"+id+".db"), false)
		var err error
		entry, err = openWildcardCache(id, signature, pageSize, dictCount, dir)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		c.entries[id] = entry
		c.pruneLocked(now, entry.path, false)
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
	if len(result.Words) > pageSize {
		return errors.New("wildcard cache page exceeds configured block size")
	}
	var first, last any
	if len(result.Positions) == len(result.Words) && len(result.Positions) > 0 {
		first, last = result.Positions[0], result.Positions[len(result.Positions)-1]
	}
	start := (page - 1) * pageSize
	if _, err := tx.Exec(`INSERT INTO wildcard_block(within_text,within_wildcards,dict_id,dict_order,block_no,words_json,positions_json,word_count,start_offset,first_position,last_position)
		VALUES('',0,?,?,?,?,?,?,?,?,?) ON CONFLICT(within_text,within_wildcards,dict_id,block_no) DO UPDATE SET
		dict_order=excluded.dict_order,words_json=excluded.words_json,positions_json=excluded.positions_json,word_count=excluded.word_count,
		start_offset=excluded.start_offset,first_position=excluded.first_position,last_position=excluded.last_position`,
		dictID, dictOrder, page, string(words), string(positions), len(result.Words), start, first, last); err != nil {
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
	if event.Total < len(event.Words) {
		return errors.New("wildcard cache stream total is smaller than its event")
	}
	if len(event.Words) > 0 {
		if within != "" && (len(event.Positions) != len(event.Words) || !strictlyIncreasingPositions(event.Positions)) {
			return errors.New("wildcard cache stream has invalid match positions")
		}
		var dictOrder int
		if err := e.db.QueryRow(`SELECT dict_order FROM wildcard_dictionary WHERE within_text=? AND within_wildcards=? AND dict_id=?`,
			within, boolInt(withinMask), dictID).Scan(&dictOrder); err != nil {
			return err
		}
		eventStart := event.Total - len(event.Words)
		tx, err := e.db.Begin()
		if err != nil {
			return err
		}
		for consumed := 0; consumed < len(event.Words); {
			absolute := eventStart + consumed
			blockNo := absolute/e.pageSize + 1
			blockStart := (blockNo - 1) * e.pageSize
			withinBlock := absolute - blockStart
			take := min(len(event.Words)-consumed, e.pageSize-withinBlock)
			var oldWordsJSON, oldPositionsJSON string
			var oldCount int
			err := tx.QueryRow(`SELECT words_json,positions_json,word_count FROM wildcard_block
				WHERE within_text=? AND within_wildcards=? AND dict_id=? AND block_no=?`,
				within, boolInt(withinMask), dictID, blockNo).Scan(&oldWordsJSON, &oldPositionsJSON, &oldCount)
			oldWords, oldPositions := []string{}, []int{}
			if err == nil {
				if oldCount != withinBlock {
					_ = tx.Rollback()
					return errors.New("wildcard cache stream is not contiguous")
				}
				if err := json.Unmarshal([]byte(oldWordsJSON), &oldWords); err != nil {
					_ = tx.Rollback()
					return err
				}
				if err := json.Unmarshal([]byte(oldPositionsJSON), &oldPositions); err != nil {
					_ = tx.Rollback()
					return err
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				_ = tx.Rollback()
				return err
			} else if withinBlock != 0 {
				_ = tx.Rollback()
				return errors.New("wildcard cache stream has a page gap")
			}
			wordsPart := event.Words[consumed : consumed+take]
			combinedWords := append(oldWords, wordsPart...)
			combinedPositions := oldPositions
			if len(event.Positions) == len(event.Words) {
				combinedPositions = append(append([]int(nil), oldPositions...), event.Positions[consumed:consumed+take]...)
			}
			if within != "" && !strictlyIncreasingPositions(combinedPositions) {
				_ = tx.Rollback()
				return errors.New("wildcard cache positions are not increasing across events")
			}
			wordsJSON, err := json.Marshal(combinedWords)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			positionsJSON, err := json.Marshal(combinedPositions)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			var first, last any
			if len(combinedPositions) == len(combinedWords) && len(combinedPositions) > 0 {
				first, last = combinedPositions[0], combinedPositions[len(combinedPositions)-1]
			}
			_, err = tx.Exec(`INSERT INTO wildcard_block(within_text,within_wildcards,dict_id,dict_order,block_no,words_json,positions_json,word_count,start_offset,first_position,last_position)
				VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(within_text,within_wildcards,dict_id,block_no) DO UPDATE SET
				words_json=excluded.words_json,positions_json=excluded.positions_json,word_count=excluded.word_count,
				start_offset=excluded.start_offset,first_position=excluded.first_position,last_position=excluded.last_position`,
				within, boolInt(withinMask), dictID, dictOrder, blockNo, string(wordsJSON), string(positionsJSON), len(combinedWords), blockStart, first, last)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			consumed += take
		}
		if err := tx.Commit(); err != nil {
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
	c.mu.Lock()
	c.pruneLocked(time.Now(), "", false)
	c.mu.Unlock()
}

func (c *wildcardPageCache) setLimit(maxBytes int64) {
	c.mu.Lock()
	c.maxBytes = maxBytes
	c.pruneLocked(time.Now(), "", false)
	c.mu.Unlock()
}

func (c *wildcardPageCache) beginExternal(path string) {
	c.mu.Lock()
	if c.activePaths == nil {
		c.activePaths = make(map[string]int)
	}
	c.activePaths[filepath.Clean(path)]++
	c.mu.Unlock()
}

func (c *wildcardPageCache) endExternal(path string) {
	c.mu.Lock()
	clean := filepath.Clean(path)
	if c.activePaths[clean] <= 1 {
		delete(c.activePaths, clean)
	} else {
		c.activePaths[clean]--
	}
	c.pruneLocked(time.Now(), "", false)
	c.mu.Unlock()
}

type resultCacheFile struct {
	path     string
	modified time.Time
	size     int64
}

func wildcardCacheFileSize(path string) int64 {
	var size int64
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if info, err := os.Stat(path + suffix); err == nil && !info.IsDir() {
			size += info.Size()
		}
	}
	return size
}

func (c *wildcardPageCache) pruneLocked(now time.Time, keep string, startup bool) {
	if c.entries == nil {
		c.entries = make(map[string]*wildcardCacheEntry)
	}
	if c.dir == "" {
		return
	}
	items, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	files := make([]resultCacheFile, 0, len(items))
	active := make(map[string]bool, len(c.activePaths))
	for p, n := range c.activePaths {
		if n > 0 {
			active[p] = true
		}
	}
	for _, item := range items {
		if item.IsDir() || !strings.HasPrefix(item.Name(), "wildcard_") || !strings.HasSuffix(item.Name(), ".db") {
			continue
		}
		path := filepath.Join(c.dir, item.Name())
		info, err := item.Info()
		if err != nil {
			continue
		}
		if startup && now.Sub(info.ModTime()) > wildcardCacheMaxAge && path != keep && !active[path] {
			removeWildcardCacheFiles(path)
			continue
		}
		files = append(files, resultCacheFile{path: path, modified: info.ModTime(), size: wildcardCacheFileSize(path)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified.After(files[j].modified) })
	var total int64
	count := 0
	for _, f := range files {
		total += f.size
		count++
	}
	for i := len(files) - 1; i >= 0 && (count > indexconst.ResultCacheLimit || c.maxBytes > 0 && total > c.maxBytes); i-- {
		f := files[i]
		if f.path == keep || active[f.path] {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f.path), "wildcard_"), ".db")
		if entry := c.entries[id]; entry != nil {
			entry.mu.Lock()
			if entry.active > 0 {
				entry.mu.Unlock()
				continue
			}
			entry.closeAndRemoveLocked()
			entry.mu.Unlock()
			delete(c.entries, id)
		} else {
			removeWildcardCacheFiles(f.path)
		}
		total -= f.size
		count--
	}
}
