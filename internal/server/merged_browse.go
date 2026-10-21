// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"container/heap"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/search"
	"github.com/wuweidict/wudict/internal/store"
)

const mergedBrowseAnchorStep = 100

var mergedBrowseLocks sync.Map

func lockMergedCache(path string) func() {
	value, _ := mergedBrowseLocks.LoadOrStore(path, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

type mergedBrowseSource struct {
	name    string
	browser dict.Browser
	total   int
	offset  int
	words   []string
	index   int
}

type mergedBrowseHead struct {
	word   string
	source int
}

type mergedBrowseHeap []mergedBrowseHead

func (h mergedBrowseHeap) Len() int           { return len(h) }
func (h mergedBrowseHeap) Less(i, j int) bool { return sqliteNoCaseLess(h[i].word, h[j].word) }
func (h mergedBrowseHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *mergedBrowseHeap) Push(v any)        { *h = append(*h, v.(mergedBrowseHead)) }
func (h *mergedBrowseHeap) Pop() any {
	old := *h
	v := old[len(old)-1]
	*h = old[:len(old)-1]
	return v
}

func sqliteNoCaseLess(a, b string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return x < y
		}
	}
	return len(a) < len(b)
}

func sqliteNoCaseEqual(a, b string) bool { return !sqliteNoCaseLess(a, b) && !sqliteNoCaseLess(b, a) }

func (s *Server) mergedBrowsePage(ctx context.Context, q url.Values, progress func(int, int) error) (browseResp, error) {
	ids := strings.Split(q.Get("ids"), ",")
	if len(ids) == 0 || len(ids) > 256 || q.Get("ids") == "" {
		return browseResp{}, errors.New("no dictionaries selected for the combined A–Z list")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || len(id) > 256 || seen[id] {
			return browseResp{}, errors.New("invalid dictionary selection for the combined A–Z list")
		}
		seen[id] = true
	}
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	sources := make([]mergedBrowseSource, 0, len(ids))
	var signature strings.Builder
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return browseResp{}, err
		}
		e, err := s.reg.get(id)
		if err != nil {
			return browseResp{}, err
		}
		if !hasBrowseIndex(e.Path) {
			return browseResp{}, fmt.Errorf("%s has no Browse index; create one in Dictionary Settings before using the combined A–Z list", e.probeName())
		}
		d, err := e.open()
		if err != nil {
			return browseResp{}, fmt.Errorf("opening %s: %w", e.probeName(), err)
		}
		browser, ok := d.(dict.Browser)
		if !ok {
			return browseResp{}, fmt.Errorf("%s does not provide a Browse index", e.probeName())
		}
		alphabet, err := browser.Alphabet()
		if err != nil {
			return browseResp{}, fmt.Errorf("reading %s alphabet: %w", e.probeName(), err)
		}
		total := 0
		for _, letter := range alphabet {
			total += letter.Count
		}
		if total == 0 {
			continue
		}
		path, prepared := e.preparedDB()
		var stamp string
		if prepared {
			if info, statErr := os.Stat(path); statErr == nil {
				stamp = fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
			}
		}
		signature.WriteString(id)
		signature.WriteByte(0)
		signature.WriteString(stamp)
		signature.WriteByte(0)
		signature.WriteString(fmt.Sprint(e.gen.Load()))
		signature.WriteByte('\n')
		sources = append(sources, mergedBrowseSource{name: e.probeName(), browser: browser, total: total})
	}
	keyBytes := sha256.Sum256([]byte(signature.String()))
	cacheID := hex.EncodeToString(keyBytes[:16])
	dir := s.wildcardCacheDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return browseResp{}, err
	}
	path := filepath.Join(dir, "wildcard_merged_"+cacheID+".db")
	unlock := lockMergedCache(path)
	defer unlock()
	s.wildcardPages.beginExternal(path)
	defer s.wildcardPages.endExternal(path)
	if err := ensureMergedBrowseCache(ctx, path, signature.String(), sources, progress); err != nil {
		return browseResp{}, err
	}
	return readMergedBrowsePage(path, q, cacheID)
}

func ensureMergedBrowseCache(ctx context.Context, path, signature string, sources []mergedBrowseSource, progress func(int, int) error) error {
	if db, err := store.OpenWildcardCacheDB(path); err == nil {
		var saved string
		err = db.QueryRow(`SELECT signature FROM merged_meta LIMIT 1`).Scan(&saved)
		_ = db.Close()
		if err == nil && saved == signature {
			return nil
		}
	}
	removeWildcardCacheFiles(path)
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, statement := range []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE merged_meta(signature TEXT NOT NULL,total INTEGER NOT NULL,alphabet_json TEXT NOT NULL)`,
		`CREATE TABLE merged_word(word TEXT NOT NULL COLLATE NOCASE PRIMARY KEY)`,
		`CREATE TABLE merged_anchor(offset INTEGER PRIMARY KEY,word TEXT NOT NULL COLLATE NOCASE)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
	}
	if len(sources) == 0 {
		emptyAlphabet, _ := json.Marshal([]dict.Letter{})
		if _, err := db.Exec(`INSERT INTO merged_meta(signature,total,alphabet_json) VALUES(?,0,?)`, signature, string(emptyAlphabet)); err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
		return nil
	}
	all := 0
	for i := range sources {
		all += sources[i].total
	}
	fetch := func(i int) error {
		source := &sources[i]
		if source.index < len(source.words) {
			return nil
		}
		if source.offset >= source.total {
			source.words = nil
			source.index = 0
			return nil
		}
		words, err := source.browser.Page(source.offset, 300)
		if err != nil {
			return fmt.Errorf("reading %s headwords: %w", source.name, err)
		}
		source.offset += min(browsePageSize, source.total-source.offset)
		source.words, source.index = words, 0
		return nil
	}
	var heads mergedBrowseHeap
	for i := range sources {
		if err := fetch(i); err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
		if len(sources[i].words) > 0 {
			heap.Push(&heads, mergedBrowseHead{word: sources[i].words[0], source: i})
			sources[i].index++
		}
	}
	heap.Init(&heads)
	tx, err := db.Begin()
	if err != nil {
		removeWildcardCacheFiles(path)
		return err
	}
	defer tx.Rollback()
	insertWord, err := tx.Prepare(`INSERT OR IGNORE INTO merged_word(word) VALUES(?)`)
	if err != nil {
		removeWildcardCacheFiles(path)
		return err
	}
	defer insertWord.Close()
	insertAnchor, err := tx.Prepare(`INSERT INTO merged_anchor(offset,word) VALUES(?,?)`)
	if err != nil {
		removeWildcardCacheFiles(path)
		return err
	}
	defer insertAnchor.Close()
	var sections dict.MaskSections
	last := ""
	unique, scanned, nextProgress := 0, 0, max(1, all/100)
	for heads.Len() > 0 {
		if err := ctx.Err(); err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
		head := heap.Pop(&heads).(mergedBrowseHead)
		if unique == 0 || !sqliteNoCaseEqual(last, head.word) {
			if _, err := insertWord.Exec(head.word); err != nil {
				removeWildcardCacheFiles(path)
				return err
			}
			if unique%mergedBrowseAnchorStep == 0 {
				if _, err := insertAnchor.Exec(unique, head.word); err != nil {
					removeWildcardCacheFiles(path)
					return err
				}
			}
			sections.Add(head.word)
			last = head.word
			unique++
		}
		source := &sources[head.source]
		scanned++
		if source.index >= len(source.words) {
			source.words = nil
			source.index = 0
			if err := fetch(head.source); err != nil {
				removeWildcardCacheFiles(path)
				return err
			}
		}
		if source.index < len(source.words) {
			heap.Push(&heads, mergedBrowseHead{word: source.words[source.index], source: head.source})
			source.index++
		}
		if progress != nil && (scanned >= nextProgress || scanned == all) {
			if err := progress(scanned, all); err != nil {
				removeWildcardCacheFiles(path)
				return err
			}
			nextProgress = scanned + max(1, all/100)
		}
	}
	if progress != nil && scanned < all {
		if err := progress(all, all); err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
	}
	alphabet, err := json.Marshal(sections.Finish())
	if err != nil {
		removeWildcardCacheFiles(path)
		return err
	}
	if _, err := tx.Exec(`INSERT INTO merged_meta(signature,total,alphabet_json) VALUES(?,?,?)`, signature, unique, string(alphabet)); err != nil {
		removeWildcardCacheFiles(path)
		return err
	}
	if err := tx.Commit(); err != nil {
		removeWildcardCacheFiles(path)
		return err
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	return nil
}

func readMergedBrowsePage(path string, q url.Values, cacheID string) (browseResp, error) {
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return browseResp{}, err
	}
	defer db.Close()
	var total int
	var encoded string
	if err := db.QueryRow(`SELECT total,alphabet_json FROM merged_meta LIMIT 1`).Scan(&total, &encoded); err != nil {
		return browseResp{}, err
	}
	var alphabet []dict.Letter
	if err := json.Unmarshal([]byte(encoded), &alphabet); err != nil {
		return browseResp{}, err
	}
	page, _ := strconv.Atoi(q.Get("p"))
	if page < 1 {
		page = 1
	}
	pages := (total + browsePageSize - 1) / browsePageSize
	if pages > 0 && page > pages {
		page = pages
	}
	offset := (page - 1) * browsePageSize
	mark := offset
	if raw := q.Get("at"); raw != "" {
		if err := db.QueryRow(`SELECT COUNT(*) FROM merged_word WHERE word<? COLLATE NOCASE`, raw).Scan(&mark); err != nil {
			return browseResp{}, err
		}
		offset = max(0, mark-mark%browsePageSize)
		page = offset/browsePageSize + 1
	}
	if total > 0 && offset >= total {
		offset = (total - 1) / browsePageSize * browsePageSize
		page = offset/browsePageSize + 1
	}
	if mark < offset || mark >= offset+browsePageSize {
		mark = offset
	}
	anchorOffset := 0
	anchorWord := ""
	err = db.QueryRow(`SELECT offset,word FROM merged_anchor WHERE offset<=? ORDER BY offset DESC LIMIT 1`, offset).Scan(&anchorOffset, &anchorWord)
	if err != nil && err != sql.ErrNoRows {
		return browseResp{}, err
	}
	words := make([]string, 0, browsePageSize)
	if anchorWord != "" {
		rows, err := db.Query(`SELECT word FROM merged_word WHERE word>=? COLLATE NOCASE ORDER BY word COLLATE NOCASE LIMIT ?`, anchorWord, offset-anchorOffset+browsePageSize)
		if err != nil {
			return browseResp{}, err
		}
		defer rows.Close()
		skip := offset - anchorOffset
		for rows.Next() {
			var word string
			if err := rows.Scan(&word); err != nil {
				return browseResp{}, err
			}
			if skip > 0 {
				skip--
				continue
			}
			words = append(words, word)
		}
		if err := rows.Err(); err != nil {
			return browseResp{}, err
		}
	} else {
		rows, err := db.Query(`SELECT word FROM merged_word ORDER BY word COLLATE NOCASE LIMIT ?`, browsePageSize)
		if err != nil {
			return browseResp{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var word string
			if err := rows.Scan(&word); err != nil {
				return browseResp{}, err
			}
			words = append(words, word)
		}
		if err := rows.Err(); err != nil {
			return browseResp{}, err
		}
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	return browseResp{Dict: q.Get("dict"), Total: total, Page: page, Pages: pages, Size: browsePageSize, At: mark, Words: words, Alphabet: alphabet, mergedCacheID: cacheID}, nil
}

func mergedFindSignature(baseID, query, within string, mask, withinMask bool) string {
	return strings.Join([]string{baseID, query, within, strconv.FormatBool(mask), strconv.FormatBool(withinMask)}, "\x00")
}

func mergedFindCachePath(dir, signature string) string {
	hash := sha256.Sum256([]byte(signature))
	return filepath.Join(dir, "wildcard_merged_find_"+hex.EncodeToString(hash[:16])+".db")
}

func mergedFindMatchers(query, within string, mask, withinMask bool) (func(string) bool, error) {
	query = strings.TrimSpace(query)
	within = strings.TrimSpace(within)
	if len(query) > 256 || len(within) > 256 {
		return nil, errors.New("search text is too long")
	}
	var base func(string) bool
	if mask {
		re, err := search.CompileMask(query)
		if err != nil {
			return nil, err
		}
		base = re.MatchString
	} else {
		base = dict.HeadwordTermMatcher(query)
	}
	var filter func(string) bool
	if within != "" {
		if withinMask {
			re, err := search.CompileMask(within)
			if err != nil {
				return nil, err
			}
			filter = re.MatchString
		} else {
			filter = dict.HeadwordTermMatcher(within)
		}
	}
	return func(word string) bool { return base(word) && (filter == nil || filter(word)) }, nil
}

func ensureMergedFindCache(ctx context.Context, basePath, path, signature string, query, within string, mask, withinMask bool, progress func(int, int, int) error) error {
	if db, err := store.OpenWildcardCacheDB(path); err == nil {
		var saved string
		err = db.QueryRow(`SELECT signature FROM merged_find_meta LIMIT 1`).Scan(&saved)
		_ = db.Close()
		if err == nil && saved == signature {
			_ = os.Chtimes(path, time.Now(), time.Now())
			return nil
		}
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	removeWildcardCacheFiles(path)
	matcher, err := mergedFindMatchers(query, within, mask, withinMask)
	if err != nil {
		return err
	}
	baseDB, err := store.OpenWildcardCacheDB(basePath)
	if err != nil {
		return err
	}
	defer baseDB.Close()
	var scanTotal int
	if err := baseDB.QueryRow(`SELECT total FROM merged_meta LIMIT 1`).Scan(&scanTotal); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA user_version=1;
		CREATE TABLE merged_find_meta(signature TEXT PRIMARY KEY,total INTEGER NOT NULL,alphabet_json TEXT NOT NULL);
		CREATE TABLE merged_find_word(ordinal INTEGER PRIMARY KEY,word TEXT NOT NULL COLLATE NOCASE UNIQUE,source_position INTEGER NOT NULL);
		CREATE INDEX merged_find_source_position ON merged_find_word(source_position);
		CREATE TABLE merged_find_anchor(ordinal INTEGER PRIMARY KEY,word TEXT NOT NULL,source_position INTEGER NOT NULL)`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	insertWord, err := tx.Prepare(`INSERT INTO merged_find_word(ordinal,word,source_position) VALUES(?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	insertAnchor, err := tx.Prepare(`INSERT INTO merged_find_anchor(ordinal,word,source_position) VALUES(?,?,?)`)
	if err != nil {
		_ = insertWord.Close()
		_ = tx.Rollback()
		return err
	}
	rows, err := baseDB.Query(`SELECT rowid-1,word FROM merged_word ORDER BY word COLLATE NOCASE`)
	if err != nil {
		_ = insertWord.Close()
		_ = insertAnchor.Close()
		_ = tx.Rollback()
		return err
	}
	var sections dict.MaskSections
	ordinal, scanned := 0, 0
	lastProgress := time.Now()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			_ = rows.Close()
			_ = insertWord.Close()
			_ = insertAnchor.Close()
			_ = tx.Rollback()
			return err
		}
		var sourcePosition int
		var word string
		if err := rows.Scan(&sourcePosition, &word); err != nil {
			_ = rows.Close()
			_ = insertWord.Close()
			_ = insertAnchor.Close()
			_ = tx.Rollback()
			return err
		}
		scanned++
		if matcher(word) {
			if _, err := insertWord.Exec(ordinal, word, sourcePosition); err != nil {
				_ = rows.Close()
				_ = insertWord.Close()
				_ = insertAnchor.Close()
				_ = tx.Rollback()
				return err
			}
			sections.Add(word)
			if ordinal%mergedBrowseAnchorStep == 0 {
				if _, err := insertAnchor.Exec(ordinal, word, sourcePosition); err != nil {
					_ = rows.Close()
					_ = insertWord.Close()
					_ = insertAnchor.Close()
					_ = tx.Rollback()
					return err
				}
			}
			ordinal++
		}
		if progress != nil && (scanned%10000 == 0 || time.Since(lastProgress) >= 250*time.Millisecond) {
			if err := progress(scanned, scanTotal, ordinal); err != nil {
				_ = rows.Close()
				_ = insertWord.Close()
				_ = insertAnchor.Close()
				_ = tx.Rollback()
				return err
			}
			lastProgress = time.Now()
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		_ = insertWord.Close()
		_ = insertAnchor.Close()
		_ = tx.Rollback()
		return err
	}
	if err := rows.Close(); err != nil {
		_ = insertWord.Close()
		_ = insertAnchor.Close()
		_ = tx.Rollback()
		return err
	}
	if err := insertWord.Close(); err != nil {
		_ = insertAnchor.Close()
		_ = tx.Rollback()
		return err
	}
	if err := insertAnchor.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	alphabet, err := json.Marshal(sections.Finish())
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO merged_find_meta(signature,total,alphabet_json) VALUES(?,?,?)`, signature, ordinal, string(alphabet)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if progress != nil {
		if err := progress(scanned, scanTotal, ordinal); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) mergedBrowseFindPage(ctx context.Context, q url.Values) (browseFindResp, error) {
	base, err := s.mergedBrowsePage(ctx, q, nil)
	if err != nil {
		return browseFindResp{}, err
	}
	query, within := q.Get("q"), q.Get("within")
	mask := q.Get("wildcards") == "1" || q.Get("mask") == "1"
	withinMask := q.Get("withinWildcards") == "1" || q.Get("withinMask") == "1"
	signature := mergedFindSignature(base.mergedCacheID, query, within, mask, withinMask)
	path := mergedFindCachePath(s.wildcardCacheDir(), signature)
	unlock := lockMergedCache(path)
	defer unlock()
	s.wildcardPages.beginExternal(path)
	defer s.wildcardPages.endExternal(path)
	if err := ensureMergedFindCache(ctx, filepath.Join(s.wildcardCacheDir(), "wildcard_merged_"+base.mergedCacheID+".db"), path, signature, query, within, mask, withinMask, nil); err != nil {
		return browseFindResp{}, err
	}
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return browseFindResp{}, err
	}
	defer db.Close()
	var total int
	var alphabetJSON string
	if err := db.QueryRow(`SELECT total,alphabet_json FROM merged_find_meta LIMIT 1`).Scan(&total, &alphabetJSON); err != nil {
		return browseFindResp{}, err
	}
	var alphabet []dict.Letter
	if err := json.Unmarshal([]byte(alphabetJSON), &alphabet); err != nil {
		return browseFindResp{}, err
	}
	page, _ := strconv.Atoi(q.Get("p"))
	if page < 1 {
		page = 1
	}
	if raw := q.Get("findpos"); raw != "" {
		position, err := strconv.Atoi(raw)
		if err != nil {
			return browseFindResp{}, errors.New("invalid merged match position")
		}
		direction, err := strconv.Atoi(q.Get("finddir"))
		if err != nil || direction < -1 || direction > 1 {
			return browseFindResp{}, errors.New("invalid merged match direction")
		}
		condition, order := "source_position>?", "ASC"
		if direction < 0 {
			condition, order = "source_position<?", "DESC"
		} else if direction == 0 {
			condition, order = "source_position=?", "ASC"
		}
		if err := db.QueryRow(`SELECT ordinal FROM merged_find_word WHERE `+condition+` ORDER BY source_position `+order+` LIMIT 1`, position).Scan(&position); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return browseFindResp{}, nil
			}
			return browseFindResp{}, err
		}
		page = position/browsePageSize + 1
	}
	pages := (total + browsePageSize - 1) / browsePageSize
	if pages > 0 && page > pages {
		page = pages
	}
	offset := (page - 1) * browsePageSize
	rows, err := db.Query(`SELECT word,source_position FROM merged_find_word WHERE ordinal>=? ORDER BY ordinal LIMIT ?`, offset, browsePageSize)
	if err != nil {
		return browseFindResp{}, err
	}
	defer rows.Close()
	words := make([]string, 0, browsePageSize)
	positions := make([]int, 0, browsePageSize)
	for rows.Next() {
		var word string
		var position int
		if err := rows.Scan(&word, &position); err != nil {
			return browseFindResp{}, err
		}
		words = append(words, word)
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return browseFindResp{}, err
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	return browseFindResp{Words: words, Positions: positions, Alphabet: alphabet, Total: total, Page: page, Pages: pages, Size: browsePageSize}, nil
}

func (s *Server) streamMergedBrowseFind(ctx context.Context, q url.Values, send func(browseFindStreamEvent) error) error {
	base, err := s.mergedBrowsePage(ctx, q, func(done, total int) error {
		return send(browseFindStreamEvent{Type: "progress", Scanned: done, ScanTotal: total})
	})
	if err != nil {
		return err
	}
	query, within := q.Get("q"), q.Get("within")
	mask := q.Get("wildcards") == "1" || q.Get("mask") == "1"
	withinMask := q.Get("withinWildcards") == "1" || q.Get("withinMask") == "1"
	signature := mergedFindSignature(base.mergedCacheID, query, within, mask, withinMask)
	path := mergedFindCachePath(s.wildcardCacheDir(), signature)
	unlock := lockMergedCache(path)
	defer unlock()
	s.wildcardPages.beginExternal(path)
	defer s.wildcardPages.endExternal(path)
	progress := func(scanned, total, found int) error {
		return send(browseFindStreamEvent{Type: "progress", Scanned: scanned, ScanTotal: total, Total: found})
	}
	if err := ensureMergedFindCache(ctx, filepath.Join(s.wildcardCacheDir(), "wildcard_merged_"+base.mergedCacheID+".db"), path, signature, query, within, mask, withinMask, progress); err != nil {
		return err
	}
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	var total int
	var alphabetJSON string
	if err := db.QueryRow(`SELECT total,alphabet_json FROM merged_find_meta LIMIT 1`).Scan(&total, &alphabetJSON); err != nil {
		return err
	}
	var alphabet []dict.Letter
	if err := json.Unmarshal([]byte(alphabetJSON), &alphabet); err != nil {
		return err
	}
	return send(browseFindStreamEvent{Type: "done", Total: total, Scanned: base.Total, ScanTotal: base.Total, Size: browsePageSize, Alphabet: alphabet, DiskCache: true, Done: true})
}
