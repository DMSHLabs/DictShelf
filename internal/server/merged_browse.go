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
	"unicode"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/indexconst"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/search"
	"github.com/wuweidict/wudict/internal/store"
)

const mergedBrowseAnchorStep = 100

var mergedBrowseLocks sync.Map

const mergedBrowseSignatureVersion = "merged-browse-sql-index-v5-configurable"

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
	useFTS, useTrigram := q.Get("mergedfts") == "1", q.Get("mergedtrigram") == "1"
	rawIDs := q.Get("ids")
	if rawIDs == "" {
		return browseResp{}, errors.New("no dictionaries selected for the combined A–Z list")
	}
	ids := strings.Split(rawIDs, ",")
	limit := s.uiPrefsSnapshot().CombinedDictionaryLimit
	if len(ids) > limit {
		return browseResp{}, fmt.Errorf("combined A–Z list limit is %d dictionaries; %d selected", limit, len(ids))
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
	if q.Get("sourcecache") != "" {
		return s.mergedBrowseFromWildcard(ctx, q, ids, progress)
	}
	sources := make([]mergedBrowseSource, 0, len(ids))
	var signature strings.Builder
	signature.WriteString(mergedBrowseSignatureVersion + "\n")
	fmt.Fprintf(&signature, "fts=%t\ntrigram=%t\n", useFTS, useTrigram)
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
	if err := ensureMergedBrowseCache(ctx, path, signature.String(), sources, useFTS, useTrigram, progress); err != nil {
		return browseResp{}, err
	}
	if err := rememberMergedCacheGroup(path, q.Get("wildgroup"), useFTS, useTrigram); err != nil {
		return browseResp{}, err
	}
	return readMergedBrowsePage(path, q, cacheID)
}

// One merged base can serve several groups and masks. Keep every group using it
// so cache eviction can remove both Browse layouts together.
func rememberMergedCacheGroup(path, group string, fts, trigram bool) error {
	if group == "" || len(group) > 256 {
		return nil
	}
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS merged_cache_group(group_id TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO merged_cache_group(group_id) VALUES(?)`, wildcardCacheGroupKey(group, fts, trigram))
	return err
}

func ensureMergedBrowseCache(ctx context.Context, path, signature string, sources []mergedBrowseSource, useFTS, useTrigram bool, progress func(int, int) error) error {
	started := time.Now()
	diagnostic := browseFindDiagnosticEnabled(ctx)
	if db, err := store.OpenWildcardCacheDB(path); err == nil {
		var saved string
		err = db.QueryRow(`SELECT signature FROM merged_meta LIMIT 1`).Scan(&saved)
		_ = db.Close()
		if err == nil && saved == signature {
			if diagnostic {
				logx.System("headword_merged_browse cache=hit fts=%t trigram=%t elapsed_ms=%d", useFTS, useTrigram, time.Since(started).Milliseconds())
			}
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
	if useFTS {
		if _, err := db.Exec(`CREATE VIRTUAL TABLE merged_word_fts USING fts5(word, content='', columnsize=0, tokenize='unicode61 remove_diacritics 2')`); err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
	}
	if useTrigram {
		if _, err := db.Exec(`CREATE VIRTUAL TABLE merged_word_trigram USING fts5(word, content='', columnsize=0, tokenize='trigram')`); err != nil {
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
	insertWord, err := tx.Prepare(`INSERT OR IGNORE INTO merged_word(rowid,word) VALUES(?,?)`)
	if err != nil {
		removeWildcardCacheFiles(path)
		return err
	}
	defer insertWord.Close()
	var insertFTS, insertTrigram *sql.Stmt
	if useFTS {
		insertFTS, err = tx.Prepare(`INSERT INTO merged_word_fts(rowid,word) VALUES(?,?)`)
		if err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
		defer insertFTS.Close()
	}
	if useTrigram {
		insertTrigram, err = tx.Prepare(`INSERT INTO merged_word_trigram(rowid,word) VALUES(?,?)`)
		if err != nil {
			removeWildcardCacheFiles(path)
			return err
		}
		defer insertTrigram.Close()
	}
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
			rowID := unique + 1
			if _, err := insertWord.Exec(rowID, head.word); err != nil {
				removeWildcardCacheFiles(path)
				return err
			}
			if insertTrigram != nil {
				if _, err := insertTrigram.Exec(rowID, dict.Fold(head.word)); err != nil {
					removeWildcardCacheFiles(path)
					return err
				}
			}
			if insertFTS != nil {
				if _, err := insertFTS.Exec(rowID, head.word); err != nil {
					removeWildcardCacheFiles(path)
					return err
				}
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
	if diagnostic {
		logx.System("headword_merged_browse cache=built fts=%t trigram=%t source_words=%d unique_words=%d elapsed_ms=%d", useFTS, useTrigram, scanned, unique, time.Since(started).Milliseconds())
	}
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
	if mask && search.IsSimpleMask(query) {
		query = search.NormalizeSimpleMask(query)
	}
	if withinMask && search.IsSimpleMask(within) {
		within = search.NormalizeSimpleMask(within)
	}
	parts := []string{baseID, query, within, strconv.FormatBool(mask), strconv.FormatBool(withinMask), "base-position-v4"}
	return strings.Join(parts, "\x00")
}

func mergedFindCachePath(dir, signature string) string {
	hash := sha256.Sum256([]byte(signature))
	return filepath.Join(dir, "wildcard_merged_find_"+hex.EncodeToString(hash[:16])+".db")
}

func mergedFindMatchers(query, within string, mask, withinMask bool) (func(string) bool, error) {
	query = strings.TrimSpace(query)
	within = strings.TrimSpace(within)
	if mask && search.IsSimpleMask(query) {
		query = search.NormalizeSimpleMask(query)
	}
	if withinMask && search.IsSimpleMask(within) {
		within = search.NormalizeSimpleMask(within)
	}
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

type mergedFindCandidate struct {
	table string
	match string
}

func mergedHeadwordFTSMatch(input string) string {
	var tokens []string
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		value := strings.ReplaceAll(token.String(), `"`, `""`)
		tokens = append(tokens, `word:"`+value+`"*`)
		token.Reset()
	}
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			token.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return strings.Join(tokens, " ")
}

func mergedMaskTrigramLiteral(pattern string) string {
	best := ""
	var run strings.Builder
	flush := func() {
		folded := dict.Fold(run.String())
		run.Reset()
		if utf8.RuneCountInString(folded) >= 3 && utf8.RuneCountInString(folded) > utf8.RuneCountInString(best) {
			best = folded
		}
	}
	for _, r := range pattern {
		if r == '*' || r == '?' {
			flush()
			continue
		}
		folded := dict.Fold(string(r))
		stable := true
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if dict.Fold(string(f)) != folded {
				stable = false
				break
			}
		}
		if !stable {
			flush()
			continue
		}
		run.WriteRune(r)
	}
	flush()
	return best
}

func mergedFindCandidates(query, within string, mask, withinMask, hasFTS, hasTrigram bool) []mergedFindCandidate {
	var out []mergedFindCandidate
	addText := func(value string) {
		if strings.ContainsFunc(value, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			if hasFTS {
				if match := mergedHeadwordFTSMatch(value); match != "" {
					out = append(out, mergedFindCandidate{table: "merged_word_fts", match: match})
				}
			}
		}
	}
	addMask := func(value string) {
		if literal := mergedMaskTrigramLiteral(value); hasTrigram && literal != "" {
			phrase := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
			out = append(out, mergedFindCandidate{table: "merged_word_trigram", match: phrase})
		}
	}
	if mask {
		addMask(query)
	} else {
		addText(query)
	}
	if within != "" {
		if withinMask {
			addMask(within)
		} else {
			addText(within)
		}
	}
	return out
}

func ensureMergedFindCache(ctx context.Context, basePath, baseID, path, signature string, query, within string, mask, withinMask bool, progress func(int, int, int) error) error {
	traceStarted := time.Now()
	diagnostic := browseFindDiagnosticEnabled(ctx)
	if db, err := store.OpenWildcardCacheDB(path); err == nil {
		var saved string
		err = db.QueryRow(`SELECT signature FROM merged_find_meta LIMIT 1`).Scan(&saved)
		if err == nil && saved == signature {
			var basePositions int
			err = db.QueryRow(`SELECT COUNT(base_position) FROM merged_find_word`).Scan(&basePositions)
		}
		_ = db.Close()
		if err == nil && saved == signature {
			if diagnostic {
				logx.System("headword_find_merged cache=hit total_ms=%d", time.Since(traceStarted).Milliseconds())
			}
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
	baseMatcher, err := mergedFindMatchers(query, "", mask, false)
	if err != nil {
		return err
	}
	var baseFindDB *sql.DB
	baseFindTotal := 0
	if within != "" {
		baseSignature := mergedFindSignature(baseID, query, "", mask, false)
		baseFindPath := mergedFindCachePath(filepath.Dir(path), baseSignature)
		unlockBase := lockMergedCache(baseFindPath)
		if err := ensureMergedFindCache(ctx, basePath, baseID, baseFindPath, baseSignature, query, "", mask, false, nil); err != nil {
			unlockBase()
			return err
		}
		baseFindDB, err = store.OpenWildcardCacheDB(baseFindPath)
		unlockBase()
		if err != nil {
			return err
		}
		defer baseFindDB.Close()
		if err := baseFindDB.QueryRowContext(ctx, `SELECT total FROM merged_find_meta LIMIT 1`).Scan(&baseFindTotal); err != nil {
			return err
		}
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
	var hasFTS, hasTrigram bool
	if err := baseDB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='merged_word_fts')`).Scan(&hasFTS); err != nil {
		return err
	}
	if err := baseDB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='merged_word_trigram')`).Scan(&hasTrigram); err != nil {
		return err
	}
	candidates := mergedFindCandidates(query, within, mask, withinMask, hasFTS, hasTrigram)
	candidateTable, candidateMatch, candidateTotal := "", "", scanTotal
	if baseFindDB != nil {
		candidateTable = "merged_find_base"
		candidateTotal = baseFindTotal
	}
	for _, candidate := range candidates {
		var count int
		sqlQuery := `SELECT COUNT(*) FROM ` + candidate.table + ` WHERE ` + candidate.table + ` MATCH ?`
		if err := baseDB.QueryRowContext(ctx, sqlQuery, candidate.match).Scan(&count); err != nil {
			return err
		}
		if candidateTable == "" || count < candidateTotal {
			candidateTable, candidateMatch, candidateTotal = candidate.table, candidate.match, count
		}
	}
	indexName := candidateTable
	if indexName == "" {
		indexName = "full_merged_scan"
	}
	if diagnostic {
		logx.System("headword_find_merged cache=miss index=%s candidates=%d base_total=%d candidate_ms=%d", indexName, candidateTotal, scanTotal, time.Since(traceStarted).Milliseconds())
	}
	var candidateRows *sql.Rows
	if candidateTable == "merged_find_base" {
		candidateRows, err = baseFindDB.QueryContext(ctx, `SELECT ordinal,source_position,word FROM merged_find_word ORDER BY ordinal`)
	} else if candidateTable == "" {
		candidateRows, err = baseDB.QueryContext(ctx, `SELECT -1,rowid-1,word FROM merged_word ORDER BY rowid`)
	} else {
		sqlQuery := `SELECT -1,m.rowid-1,m.word FROM ` + candidateTable + ` f
			JOIN merged_word m ON m.rowid=f.rowid WHERE ` + candidateTable + ` MATCH ? ORDER BY m.rowid`
		candidateRows, err = baseDB.QueryContext(ctx, sqlQuery, candidateMatch)
	}
	if err != nil {
		return err
	}
	defer candidateRows.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		_ = candidateRows.Close()
		return err
	}
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		_ = candidateRows.Close()
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA user_version=1;
		CREATE TABLE merged_find_meta(signature TEXT PRIMARY KEY,total INTEGER NOT NULL,alphabet_json TEXT NOT NULL);
		CREATE TABLE merged_find_word(ordinal INTEGER PRIMARY KEY,word TEXT NOT NULL COLLATE NOCASE UNIQUE,source_position INTEGER NOT NULL,base_position INTEGER NOT NULL);
		CREATE INDEX merged_find_source_position ON merged_find_word(source_position);
		CREATE INDEX merged_find_base_position ON merged_find_word(base_position);
		CREATE TABLE merged_find_anchor(ordinal INTEGER PRIMARY KEY,word TEXT NOT NULL,source_position INTEGER NOT NULL)`); err != nil {
		_ = candidateRows.Close()
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		_ = candidateRows.Close()
		return err
	}
	insertWord, err := tx.Prepare(`INSERT INTO merged_find_word(ordinal,word,source_position,base_position) VALUES(?,?,?,?)`)
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
	var sections dict.MaskSections
	ordinal, scanned, basePosition := 0, 0, 0
	lastProgress := time.Now()
	scanStarted := time.Now()
	nextDiagnosticPercent := 10
	for candidateRows.Next() {
		if err := ctx.Err(); err != nil {
			_ = candidateRows.Close()
			_ = insertWord.Close()
			_ = insertAnchor.Close()
			_ = tx.Rollback()
			return err
		}
		var sourcePosition, candidateBasePosition int
		var word string
		if err := candidateRows.Scan(&candidateBasePosition, &sourcePosition, &word); err != nil {
			_ = candidateRows.Close()
			_ = insertWord.Close()
			_ = insertAnchor.Close()
			_ = tx.Rollback()
			return err
		}
		scanned++
		if baseMatcher(word) {
			if matcher(word) {
				basePosition = ordinal
				if candidateBasePosition >= 0 {
					basePosition = candidateBasePosition
				} else if baseFindDB != nil {
					if err := baseFindDB.QueryRowContext(ctx, `SELECT ordinal FROM merged_find_word WHERE source_position=?`, sourcePosition).Scan(&basePosition); err != nil {
						_ = candidateRows.Close()
						_ = insertWord.Close()
						_ = insertAnchor.Close()
						_ = tx.Rollback()
						return err
					}
				}
				if _, err := insertWord.Exec(ordinal, word, sourcePosition, basePosition); err != nil {
					_ = candidateRows.Close()
					_ = insertWord.Close()
					_ = insertAnchor.Close()
					_ = tx.Rollback()
					return err
				}
				sections.Add(word)
				if ordinal%mergedBrowseAnchorStep == 0 {
					if _, err := insertAnchor.Exec(ordinal, word, sourcePosition); err != nil {
						_ = candidateRows.Close()
						_ = insertWord.Close()
						_ = insertAnchor.Close()
						_ = tx.Rollback()
						return err
					}
				}
				ordinal++
			}
			if baseFindDB == nil {
				basePosition++
			}
		}
		if progress != nil && (scanned%10000 == 0 || time.Since(lastProgress) >= 250*time.Millisecond) {
			if err := progress(scanned, candidateTotal, ordinal); err != nil {
				_ = candidateRows.Close()
				_ = insertWord.Close()
				_ = insertAnchor.Close()
				_ = tx.Rollback()
				return err
			}
			lastProgress = time.Now()
		}
		if diagnostic && candidateTotal > 0 && scanned*100/candidateTotal >= nextDiagnosticPercent {
			elapsed := time.Since(scanStarted)
			rate := int64(0)
			if elapsed > 0 {
				rate = int64(scanned) * int64(time.Second) / int64(elapsed)
			}
			logx.System("headword_find_merged stage=scan index=%s percent=%d scanned=%d candidates=%d found=%d elapsed_ms=%d rows_per_sec=%d",
				indexName, nextDiagnosticPercent, scanned, candidateTotal, ordinal, elapsed.Milliseconds(), rate)
			nextDiagnosticPercent += 10
		}
	}
	scanFinished := time.Now()
	if err := candidateRows.Err(); err != nil {
		_ = candidateRows.Close()
		_ = insertWord.Close()
		_ = insertAnchor.Close()
		_ = tx.Rollback()
		return err
	}
	if err := candidateRows.Close(); err != nil {
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
	commitStarted := time.Now()
	if err := tx.Commit(); err != nil {
		return err
	}
	commitFinished := time.Now()
	if progress != nil {
		if err := progress(scanned, candidateTotal, ordinal); err != nil {
			return err
		}
	}
	if diagnostic {
		logx.System("headword_find_merged cache=built index=%s candidates_scanned=%d found=%d scan_ms=%d finalize_ms=%d commit_ms=%d total_ms=%d",
			indexName, scanned, ordinal, scanFinished.Sub(scanStarted).Milliseconds(), commitFinished.Sub(scanFinished).Milliseconds(),
			commitFinished.Sub(commitStarted).Milliseconds(), time.Since(traceStarted).Milliseconds())
	}
	return nil
}

func (s *Server) mergedBrowseFindPage(ctx context.Context, q url.Values) (browseFindResp, error) {
	base, err := s.mergedBrowseFindBase(ctx, q, nil)
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
	if err := ensureMergedFindCache(ctx, filepath.Join(s.wildcardCacheDir(), "wildcard_merged_"+base.mergedCacheID+".db"), base.mergedCacheID, path, signature, query, within, mask, withinMask, nil); err != nil {
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
		position, err = mergedFindOrdinalAtBasePosition(db, position, direction)
		if err != nil {
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
	rows, err := db.Query(`SELECT word,source_position,base_position FROM merged_find_word WHERE ordinal>=? ORDER BY ordinal LIMIT ?`, offset, browsePageSize)
	if err != nil {
		return browseFindResp{}, err
	}
	defer rows.Close()
	words := make([]string, 0, browsePageSize)
	positions := make([]int, 0, browsePageSize)
	basePositions := make([]int, 0, browsePageSize)
	for rows.Next() {
		var word string
		var position, basePosition int
		if err := rows.Scan(&word, &position, &basePosition); err != nil {
			return browseFindResp{}, err
		}
		words = append(words, word)
		positions = append(positions, position)
		basePositions = append(basePositions, basePosition)
	}
	if err := rows.Err(); err != nil {
		return browseFindResp{}, err
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	return browseFindResp{Words: words, Positions: positions, BasePositions: basePositions, Alphabet: alphabet, Total: total, Page: page, Pages: pages, Size: browsePageSize}, nil
}

func mergedFindOrdinalAtBasePosition(db *sql.DB, position, direction int) (int, error) {
	condition, order := "base_position>?", "ASC"
	if direction < 0 {
		condition, order = "base_position<?", "DESC"
	} else if direction == 0 {
		condition, order = "base_position=?", "ASC"
	}
	var ordinal int
	err := db.QueryRow(`SELECT ordinal FROM merged_find_word WHERE `+condition+` ORDER BY base_position `+order+` LIMIT 1`, position).Scan(&ordinal)
	return ordinal, err
}

func (s *Server) streamMergedBrowseFind(ctx context.Context, q url.Values, send func(browseFindStreamEvent) error) error {
	const progressScale = 10_000
	base, err := s.mergedBrowseFindBase(ctx, q, func(done, total int) error {
		return send(browseFindStreamEvent{Type: "progress", Scanned: scaleMergedFindProgress(done, total, 5_000), ScanTotal: progressScale})
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
		return send(browseFindStreamEvent{Type: "progress", Scanned: 5_000 + scaleMergedFindProgress(scanned, total, 4_900), ScanTotal: progressScale, Total: found})
	}
	if err := ensureMergedFindCache(ctx, filepath.Join(s.wildcardCacheDir(), "wildcard_merged_"+base.mergedCacheID+".db"), base.mergedCacheID, path, signature, query, within, mask, withinMask, progress); err != nil {
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
	var matchedDictionaries int
	_ = db.QueryRowContext(ctx, `SELECT matched_dictionaries FROM merged_source_meta LIMIT 1`).Scan(&matchedDictionaries)
	return send(browseFindStreamEvent{Type: "done", Total: total, Scanned: progressScale, ScanTotal: progressScale,
		Size: browsePageSize, Alphabet: alphabet, DiskCache: true, MergedCacheID: base.mergedCacheID,
		MatchedDictionaries: matchedDictionaries, Done: true})
}

// scaleMergedFindProgress keeps the combined-list build and mask-filter phases
// on one progress scale. The final completion event is the only one that reports 100%.
func scaleMergedFindProgress(done, total, limit int) int {
	if total <= 0 || done <= 0 {
		return 0
	}
	if done >= total {
		return limit
	}
	return int(int64(done) * int64(limit) / int64(total))
}

// mergedBrowseFindBase lets the upper search build the combined database, but
// requires every lower search to name and read the database already produced
// by that upper search. It never opens a dictionary on the lower-search path.
func (s *Server) mergedBrowseFindBase(ctx context.Context, q url.Values, progress func(int, int) error) (browseResp, error) {
	if strings.TrimSpace(q.Get("within")) == "" {
		return s.mergedBrowsePage(ctx, q, progress)
	}
	ids := strings.Split(q.Get("ids"), ",")
	if q.Get("ids") == "" {
		return browseResp{}, errors.New("no dictionaries selected for the combined A–Z list")
	}
	limit := s.uiPrefsSnapshot().CombinedDictionaryLimit
	if len(ids) > limit {
		return browseResp{}, fmt.Errorf("combined A–Z list limit is %d dictionaries; %d selected", limit, len(ids))
	}
	cacheID := q.Get("mergedcache")
	if !wildcardCacheIDPattern.MatchString(cacheID) {
		return browseResp{}, errors.New("combined Wildcards lower search requires a prepared SQL database")
	}
	path := filepath.Join(s.wildcardCacheDir(), "wildcard_merged_"+cacheID+".db")
	if _, err := os.Stat(path); err != nil {
		return browseResp{}, errors.New("combined Wildcards SQL database is not available")
	}
	unlock := lockMergedCache(path)
	defer unlock()
	s.wildcardPages.beginExternal(path)
	defer s.wildcardPages.endExternal(path)
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return browseResp{}, errors.New("combined Wildcards SQL database is not available")
	}
	var signature string
	err = db.QueryRowContext(ctx, `SELECT signature FROM merged_meta LIMIT 1`).Scan(&signature)
	_ = db.Close()
	if err != nil || !mergedSignatureMatchesIDs(signature, q.Get("ids")) {
		return browseResp{}, errors.New("combined Wildcards SQL database does not match the selected dictionaries")
	}
	return readMergedBrowsePage(path, q, cacheID)
}

func mergedSignatureMatchesIDs(signature, rawIDs string) bool {
	ids := strings.Split(rawIDs, ",")
	if rawIDs == "" || len(ids) == 0 || len(ids) > indexconst.CombinedDictionaryLimitMax {
		return false
	}
	sort.Strings(ids)
	lines := strings.Split(signature, "\n")
	if len(lines) != len(ids)+4 || lines[0] != mergedBrowseSignatureVersion || lines[len(lines)-1] != "" {
		return false
	}
	if (lines[1] != "fts=true" && lines[1] != "fts=false") ||
		(lines[2] != "trigram=true" && lines[2] != "trigram=false") {
		return false
	}
	for i, id := range ids {
		if id == "" || (i > 0 && ids[i-1] == id) {
			return false
		}
		parts := strings.Split(lines[i+3], "\x00")
		if len(parts) != 3 || parts[0] != id {
			return false
		}
	}
	return true
}
