// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/indexconst"
	"github.com/wuweidict/wudict/internal/logx"
)

// StreamBrowseHeadwords emits matched headwords as soon as a result block is
// ready. total is the number found so far; done marks the final exact count.
func (s *Store) StreamBrowseHeadwords(ctx context.Context, query string, mask bool, emit func([]string, int, bool, int, int) error) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return emit(nil, 0, true, 0, s.meta.EntryCount)
	}
	var rows *sql.Rows
	var err error
	if mask {
		re, compileErr := regexp.Compile("(?i)^" + wildcardRegexp(query) + "$")
		if compileErr != nil {
			return compileErr
		}
		stream := newBrowseRowStream(s.meta.EntryCount, emit)
		strategy := s.MaskCandidateStrategy(query)
		if strategy == "exact_key" {
			folded := dict.Fold(query)
			rows, queryErr := s.db.QueryContext(ctx, `SELECT DISTINCT w FROM entry
				WHERE wf COLLATE NOCASE IN (?) AND w NOT LIKE '@_%' ORDER BY w COLLATE NOCASE`, folded)
			if queryErr != nil {
				return queryErr
			}
			if err := stream.rows(ctx, rows, re.MatchString); err != nil {
				return err
			}
			return stream.finish()
		}
		if literal := maskTrigramLiteral(query); strategy == "trigram" && literal != "" {
			phrase := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
			rows, queryErr := s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_trigram t
				JOIN entry e ON e.id=t.rowid WHERE entry_trigram MATCH ? AND e.w NOT LIKE '@_%'
				ORDER BY e.w COLLATE NOCASE`, phrase)
			if queryErr != nil {
				return queryErr
			}
			if err := stream.rows(ctx, rows, re.MatchString); err != nil {
				return err
			}
		} else {
			for _, span := range maskIndexSpans(query) {
				rows, queryErr := s.db.QueryContext(ctx, `SELECT w FROM entry WHERE `+browseWhere+span.where+browseOrder, span.args...)
				if queryErr != nil {
					return queryErr
				}
				if err := stream.rows(ctx, rows, re.MatchString); err != nil {
					return err
				}
			}
		}
		return stream.finish()
	}
	if !strings.ContainsFunc(query, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
		return emit(nil, 0, true, 0, s.meta.EntryCount)
	}
	match := buildMatch(query, "w")
	rows, err = s.db.QueryContext(ctx, `SELECT e.w FROM entry_fts f
		JOIN entry e ON e.id=f.rowid WHERE entry_fts MATCH ? AND e.w NOT LIKE '@_%'
		ORDER BY e.w COLLATE NOCASE`, match)
	if err != nil {
		return err
	}
	defer rows.Close()
	terms := dict.HeadwordTermMatcher(query)
	return streamBrowseRows(ctx, rows, terms, s.meta.EntryCount, emit)
}

func streamBrowseRows(ctx context.Context, rows *sql.Rows, matches func(string) bool, scanTotal int, emit func([]string, int, bool, int, int) error) error {
	stream := newBrowseRowStream(scanTotal, emit)
	if err := stream.rows(ctx, rows, matches); err != nil {
		return err
	}
	return stream.finish()
}

type browseRowStream struct {
	words        []string
	total        int
	firstMatch   bool
	scanned      int
	scanTotal    int
	previous     string
	nextProgress time.Time
	emit         func([]string, int, bool, int, int) error
}

func newBrowseRowStream(scanTotal int, emit func([]string, int, bool, int, int) error) *browseRowStream {
	return &browseRowStream{words: make([]string, 0, dict.MaskPageSize), scanTotal: scanTotal,
		nextProgress: time.Now().Add(time.Second), emit: emit}
}

func (s *browseRowStream) rows(ctx context.Context, rows *sql.Rows, matches func(string) bool) error {
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var word string
		if err := rows.Scan(&word); err != nil {
			return err
		}
		if word == s.previous {
			continue
		}
		s.previous = word
		s.scanned++
		if matches(word) {
			s.words = append(s.words, word)
			s.total++
		}
		now := time.Now()
		if len(s.words) > 0 && !s.firstMatch {
			if err := s.emit(s.words, s.total, false, s.scanned, s.scanTotal); err != nil {
				return err
			}
			s.words = make([]string, 0, dict.MaskPageSize)
			s.firstMatch = true
			s.nextProgress = now.Add(time.Second)
		} else if len(s.words) >= dict.MaskPageSize || (len(s.words) > 0 && !now.Before(s.nextProgress)) {
			if err := s.emit(s.words, s.total, false, s.scanned, s.scanTotal); err != nil {
				return err
			}
			s.words = make([]string, 0, dict.MaskPageSize)
			s.nextProgress = now.Add(time.Second)
		} else if now.After(s.nextProgress) {
			if err := s.emit(nil, s.total, false, s.scanned, s.scanTotal); err != nil {
				return err
			}
			s.nextProgress = now.Add(time.Second)
		}
	}
	return rows.Err()
}

func (s *browseRowStream) finish() error {
	if len(s.words) > 0 {
		if err := s.emit(s.words, s.total, false, s.scanned, s.scanTotal); err != nil {
			return err
		}
	}
	return s.emit(nil, s.total, true, s.scanTotal, s.scanTotal)
}

// FindHeadwords uses the always-present FTS headword index. A token-prefix
// match finds a word at the start or after a separator, wherever it occurs in
// a multiword title; article text is excluded by the w: column filter.
func (s *Store) FindHeadwords(ctx context.Context, query string, offset, limit int) ([]string, int, error) {
	query = strings.TrimSpace(query)
	if query == "" || limit <= 0 || !strings.ContainsFunc(query, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
		return []string{}, 0, nil
	}
	if offset < 0 {
		offset = 0
	}
	match := buildMatch(query, "w")
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT e.w) FROM entry_fts f
		JOIN entry e ON e.id=f.rowid WHERE entry_fts MATCH ? AND e.w NOT LIKE '@_%'`, match).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_fts f
		JOIN entry e ON e.id=f.rowid WHERE entry_fts MATCH ? AND e.w NOT LIKE '@_%'`+
		` ORDER BY e.w COLLATE NOCASE LIMIT ? OFFSET ?`, match, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	words := make([]string, 0, limit)
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			return nil, 0, err
		}
		words = append(words, word)
	}
	return words, total, rows.Err()
}

// FindHeadwordsMode searches prepared headwords with one of Browse's matching
// rules. Lemma modes accept unit-separated alternatives from the server.
func (s *Store) FindHeadwordsMode(ctx context.Context, query, mode string, offset, limit int) ([]string, int, []dict.Letter, error) {
	query = strings.TrimSpace(query)
	if mode == "prefix" || mode == "" {
		words, total, err := s.FindHeadwords(ctx, query, offset, limit)
		return words, total, nil, err
	}
	if mode == "wildcards" {
		return s.FindHeadwordsMask(ctx, query, offset, limit)
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || query == "" {
		return []string{}, 0, nil, nil
	}
	var rows *sql.Rows
	var err error
	var matcher func(string) bool
	switch mode {
	case "suffix", "contains":
		matcher = dict.HeadwordSuffixMatcher(query)
		if mode == "contains" {
			matcher = dict.HeadwordContainsMatcher(query)
		}
		if s.hasTrigram && !s.staleFold {
			if literal := headwordTrigramLiteral(query); literal != "" {
				phrase := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
				rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_trigram t
					JOIN entry e ON e.id=t.rowid WHERE entry_trigram MATCH ? AND e.w NOT LIKE '@_%'
					ORDER BY e.w COLLATE NOCASE`, phrase)
				break
			}
		}
		rows, err = s.db.QueryContext(ctx, `SELECT w FROM entry WHERE `+browseWhere+browseOrder)
	case "word", "wordLemma":
		match := buildWholeWordMatch(query, "w")
		if match == "" {
			return []string{}, 0, nil, nil
		}
		matcher = dict.HeadwordWordMatcher(query)
		rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_fts f
			JOIN entry e ON e.id=f.rowid WHERE entry_fts MATCH ? AND e.w NOT LIKE '@_%'
			ORDER BY e.w COLLATE NOCASE`, match)
	case "exact", "exactLemma":
		matcher = dict.HeadwordExactMatcher(query)
		values := strings.Split(query, "\x1f")
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
		args := make([]any, len(values))
		for i, value := range values {
			args[i] = dict.Fold(value)
		}
		rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT w FROM entry
			WHERE wf COLLATE NOCASE IN (`+placeholders+`) AND w NOT LIKE '@_%' ORDER BY w COLLATE NOCASE`, args...)
	default:
		return nil, 0, nil, fmt.Errorf("unknown headword match mode %q", mode)
	}
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()
	words := make([]string, 0, limit)
	total := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, err
		}
		var word string
		if err := rows.Scan(&word); err != nil {
			return nil, 0, nil, err
		}
		if !matcher(word) {
			continue
		}
		if total >= offset && len(words) < limit {
			words = append(words, word)
		}
		total++
	}
	return words, total, nil, rows.Err()
}

// StreamBrowseHeadwordsMode uses the prepared FTS or exact-key index where
// possible, then emits bounded result batches in headword order.
func (s *Store) StreamBrowseHeadwordsMode(ctx context.Context, query, mode string, emit func([]string, int, bool, int, int) error) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return emit(nil, 0, true, 0, s.meta.EntryCount)
	}
	if mode == "prefix" || mode == "" {
		return s.StreamBrowseHeadwords(ctx, query, false, emit)
	}
	if mode == "wildcards" {
		return s.StreamBrowseHeadwords(ctx, query, true, emit)
	}
	var rows *sql.Rows
	var err error
	var matcher func(string) bool
	switch mode {
	case "suffix", "contains":
		matcher = dict.HeadwordSuffixMatcher(query)
		if mode == "contains" {
			matcher = dict.HeadwordContainsMatcher(query)
		}
		if s.hasTrigram && !s.staleFold {
			if literal := headwordTrigramLiteral(query); literal != "" {
				phrase := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
				rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_trigram t
					JOIN entry e ON e.id=t.rowid WHERE entry_trigram MATCH ? AND e.w NOT LIKE '@_%'
					ORDER BY e.w COLLATE NOCASE`, phrase)
				break
			}
		}
		rows, err = s.db.QueryContext(ctx, `SELECT w FROM entry WHERE `+browseWhere+browseOrder)
	case "word", "wordLemma":
		match := buildWholeWordMatch(query, "w")
		if match == "" {
			return emit(nil, 0, true, 0, s.meta.EntryCount)
		}
		matcher = dict.HeadwordWordMatcher(query)
		rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_fts f
			JOIN entry e ON e.id=f.rowid WHERE entry_fts MATCH ? AND e.w NOT LIKE '@_%'
			ORDER BY e.w COLLATE NOCASE`, match)
	case "exact", "exactLemma":
		matcher = dict.HeadwordExactMatcher(query)
		values := strings.Split(query, "\x1f")
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
		args := make([]any, len(values))
		for i, value := range values {
			args[i] = dict.Fold(value)
		}
		rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT w FROM entry
			WHERE wf COLLATE NOCASE IN (`+placeholders+`) AND w NOT LIKE '@_%' ORDER BY w COLLATE NOCASE`, args...)
	default:
		return fmt.Errorf("unknown headword match mode %q", mode)
	}
	if err != nil {
		return err
	}
	return streamBrowseRows(ctx, rows, matcher, s.meta.EntryCount, emit)
}

func buildWholeWordMatch(input, column string) string {
	if strings.Contains(input, "\x1f") {
		var alternatives []string
		for _, value := range strings.Split(input, "\x1f") {
			if match := buildWholeWordMatch(value, column); match != "" {
				alternatives = append(alternatives, "("+match+")")
			}
		}
		return strings.Join(alternatives, " OR ")
	}
	var parts []string
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		value := strings.ReplaceAll(token.String(), `"`, `""`)
		part := `"` + value + `"`
		if column != "" {
			part = column + ":" + part
		}
		parts = append(parts, part)
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
	return strings.Join(parts, " ")
}

// FindHeadwordsMask scans the existing headword index and pages mask matches.
func (s *Store) FindHeadwordsMask(ctx context.Context, query string, offset, limit int) ([]string, int, []dict.Letter, error) {
	words, total, letters, _, err := s.FindHeadwordsMaskWithin(ctx, query, "", false, offset, limit)
	return words, total, letters, err
}

// FindHeadwordsMaskWithin intersects the main mask with the bottom Browse
// control. Positions address the unfiltered mask list for match navigation.
func (s *Store) FindHeadwordsMaskWithin(ctx context.Context, query, within string, withinMask bool, offset, limit int) ([]string, int, []dict.Letter, []int, error) {
	mode := "prefix"
	if withinMask {
		mode = "wildcards"
	}
	return s.FindHeadwordsMaskWithinMode(ctx, query, within, mode, offset, limit)
}

// FindHeadwordsMaskWithinMode intersects the main mask with a Browse match mode.
// Positions address the unfiltered mask list for match navigation.
func (s *Store) FindHeadwordsMaskWithinMode(ctx context.Context, query, within, withinMode string, offset, limit int) ([]string, int, []dict.Letter, []int, error) {
	withinMask := withinMode == "wildcards"
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		return []string{}, 0, nil, nil, nil
	}
	re, err := regexp.Compile("(?i)^" + wildcardRegexp(query) + "$")
	if err != nil {
		return nil, 0, nil, nil, err
	}
	var withinRE *regexp.Regexp
	if within != "" && withinMask {
		withinRE, err = regexp.Compile("(?i)^" + wildcardRegexp(within) + "$")
		if err != nil {
			return nil, 0, nil, nil, err
		}
	}
	matchTerms := dict.HeadwordTermMatcher(within)
	switch withinMode {
	case "suffix":
		matchTerms = dict.HeadwordSuffixMatcher(within)
	case "contains":
		matchTerms = dict.HeadwordContainsMatcher(within)
	case "word", "wordLemma":
		matchTerms = dict.HeadwordWordMatcher(within)
	case "exact", "exactLemma":
		matchTerms = dict.HeadwordExactMatcher(within)
	}
	words := make([]string, 0, limit)
	positions := make([]int, 0, limit)
	total, basePosition := 0, 0
	var sections dict.MaskSections
	visit := func(rows *sql.Rows) error {
		defer rows.Close()
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var word string
			if err := rows.Scan(&word); err != nil {
				return err
			}
			if !re.MatchString(word) {
				continue
			}
			position := basePosition
			basePosition++
			if within != "" && (withinMask && !withinRE.MatchString(word) || !withinMask && !matchTerms(word)) {
				continue
			}
			sections.Add(word)
			if total >= offset && len(words) < limit {
				words = append(words, word)
				positions = append(positions, position)
			}
			total++
		}
		return rows.Err()
	}

	// Narrow a literal-only mask through the exact key. For wildcard patterns,
	// the longest stable literal run is a trigram candidate when it has at least
	// three runes; the complete mask still verifies every candidate.
	strategy := s.MaskCandidateStrategy(query)
	if strategy == "exact_key" {
		folded := dict.Fold(query)
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT w FROM entry
			WHERE wf COLLATE NOCASE IN (?) AND w NOT LIKE '@_%' ORDER BY w COLLATE NOCASE`, folded)
		if err != nil {
			return nil, 0, nil, nil, err
		}
		if err := visit(rows); err != nil {
			return nil, 0, nil, nil, err
		}
	} else if literal := maskTrigramLiteral(query); strategy == "trigram" && literal != "" {
		phrase := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_trigram t
			JOIN entry e ON e.id=t.rowid WHERE entry_trigram MATCH ? AND e.w NOT LIKE '@_%'
			ORDER BY e.w COLLATE NOCASE`, phrase)
		if err != nil {
			return nil, 0, nil, nil, err
		}
		if err := visit(rows); err != nil {
			return nil, 0, nil, nil, err
		}
	} else {
		for _, span := range maskIndexSpans(query) {
			rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT w FROM entry WHERE `+browseWhere+span.where+browseOrder, span.args...)
			if err != nil {
				return nil, 0, nil, nil, err
			}
			if err := visit(rows); err != nil {
				return nil, 0, nil, nil, err
			}
		}
	}
	return words, total, sections.Finish(), positions, nil
}

func headwordTrigramLiteral(query string) string {
	terms := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
	best := ""
	for _, term := range terms {
		folded := dict.Fold(term)
		if utf8.RuneCountInString(folded) > utf8.RuneCountInString(best) {
			best = folded
		}
	}
	if utf8.RuneCountInString(best) < 3 {
		return ""
	}
	return best
}

// browseWhere hides the internal entries from every browse read. `@_%` is an
// MDX redirect stub (`@@@LINK=`), not a word anybody looks up, and the filter
// has to be spelled identically in all four statements below or the counts in
// Alphabet would stop addressing the rows Page returns.
const browseWhere = "w NOT LIKE '@_%'"

// Browse order, stated once: the index's own order. idx_entry_w is
// `entry(w COLLATE NOCASE)`, so ORDER BY that expression is served by the
// index - no sort, no table lookup (the index covers the only column read),
// and an OFFSET deep into a million headwords is a walk of index entries
// rather than of articles.
const browseOrder = " ORDER BY w COLLATE NOCASE"

// Page implements dict.Browser.
func (s *Store) Page(offset, n int) ([]string, error) {
	words, _, err := s.PageWithStats(offset, n)
	return words, err
}

// BrowsePageStats describes how a prepared index served one Browse page.
type BrowsePageStats struct {
	Offset        int
	AnchorOrdinal int
	AnchorIndex   bool
	AnchorUsed    bool
	AnchorLookup  time.Duration
	Read          time.Duration
	RowsRead      int
	Words         int
}

// PageWithStats reads a Browse page and reports whether it started from a
// sparse anchor. The normal Browser interface uses Page and discards the stats.
func (s *Store) PageWithStats(offset, n int) ([]string, BrowsePageStats, error) {
	stats := BrowsePageStats{Offset: max(0, offset), AnchorIndex: s.hasBrowseAnchors}
	if offset < 0 {
		offset = 0
	}
	if n <= 0 {
		return nil, stats, nil
	}
	var rows *sql.Rows
	var err error
	if s.hasBrowseAnchors {
		var ordinal, entryID int
		var word string
		anchorStarted := time.Now()
		anchorErr := s.db.QueryRow("SELECT ordinal, entry_id, w FROM browse_anchor WHERE ordinal <= ? ORDER BY ordinal DESC LIMIT 1", offset).Scan(&ordinal, &entryID, &word)
		stats.AnchorLookup = time.Since(anchorStarted)
		if anchorErr == nil {
			stats.AnchorUsed = true
			stats.AnchorOrdinal = ordinal
			skip := offset - ordinal
			readStarted := time.Now()
			rows, err = s.db.Query(`SELECT w FROM entry WHERE `+browseWhere+
				` AND w >= ? COLLATE NOCASE`+
				` AND (w COLLATE NOCASE > ? COLLATE NOCASE OR (w = ? COLLATE NOCASE AND id >= ?))`+
				` ORDER BY w COLLATE NOCASE, id LIMIT ?`, word, word, word, entryID, skip+n)
			if err != nil {
				return nil, stats, err
			}
			out := make([]string, 0, n)
			scanned := 0
			for rows.Next() {
				var w string
				if err := rows.Scan(&w); err != nil {
					rows.Close()
					stats.Read = time.Since(readStarted)
					return nil, stats, err
				}
				stats.RowsRead++
				if scanned < skip {
					scanned++
					continue
				}
				scanned++
				if len(out) == 0 || out[len(out)-1] != w {
					out = append(out, w)
				}
			}
			err = rows.Err()
			rows.Close()
			stats.Read = time.Since(readStarted)
			stats.Words = len(out)
			return out, stats, err
		} else if anchorErr != sql.ErrNoRows {
			return nil, stats, anchorErr
		}
	}
	readStarted := time.Now()
	rows, err = s.db.Query("SELECT w FROM entry WHERE "+browseWhere+browseOrder+" LIMIT ? OFFSET ?", n, offset)
	if err != nil {
		stats.Read = time.Since(readStarted)
		return nil, stats, err
	}
	out := make([]string, 0, n)
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			rows.Close()
			stats.Read = time.Since(readStarted)
			return nil, stats, err
		}
		stats.RowsRead++
		// Homographs are two articles under one headword, and a word list
		// that prints "bank" twice reads as a bug. Only the RUN collapses -
		// the offsets stay row-based, which is what keeps them addressing the
		// same sequence Alphabet counted.
		if len(out) > 0 && out[len(out)-1] == w {
			continue
		}
		out = append(out, w)
	}
	err = rows.Err()
	rows.Close()
	stats.Read = time.Since(readStarted)
	stats.Words = len(out)
	return out, stats, err
}

// StreamBrowseMaskAlphabet builds one jump label for every paged headword
// block. Browse's regular alphabet is intentionally sparse; the Wildcards
// all-words ribbon must instead address each MaskPageSize block.
func (s *Store) StreamBrowseMaskAlphabet(ctx context.Context, progress func(scanned, total int) error) ([]dict.Letter, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT w FROM entry WHERE `+browseWhere+browseOrder)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	sections := dict.MaskSections{}
	scanned, scanTotal := 0, max(0, s.meta.EntryCount)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, scanned, err
		}
		var word string
		if err := rows.Scan(&word); err != nil {
			return nil, scanned, err
		}
		sections.Add(word)
		scanned++
		if progress != nil && scanned%30000 == 0 {
			if err := progress(scanned, scanTotal); err != nil {
				return nil, scanned, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, scanned, err
	}
	if progress != nil {
		if err := progress(scanned, scanned); err != nil {
			return nil, scanned, err
		}
	}
	return sections.Finish(), scanned, nil
}

// LocateStats describes the indexed work used to resolve a headword position.
type LocateStats struct {
	AnchorLookup  time.Duration
	Count         time.Duration
	AnchorFound   bool
	AnchorOrdinal int
	CountedRows   int
	Ordinal       int
}

// Locate implements dict.Browser.
func (s *Store) Locate(word string) (int, error) {
	ordinal, _, err := s.LocateWithStats(word)
	return ordinal, err
}

// LocateWithStats resolves a headword ordinal and measures its indexed steps.
func (s *Store) LocateWithStats(word string) (int, LocateStats, error) {
	stats := LocateStats{}
	if s.hasBrowseAnchors {
		var ordinal, entryID int
		var anchorWord string
		anchorStarted := time.Now()
		err := s.db.QueryRow(`SELECT ordinal, entry_id, w FROM browse_anchor
			WHERE w < ? COLLATE NOCASE ORDER BY w COLLATE NOCASE DESC, entry_id DESC LIMIT 1`, word).
			Scan(&ordinal, &entryID, &anchorWord)
		stats.AnchorLookup = time.Since(anchorStarted)
		if err == nil {
			stats.AnchorFound = true
			stats.AnchorOrdinal = ordinal
			var count int
			countStarted := time.Now()
			err = s.db.QueryRow(`SELECT COUNT(*) FROM entry WHERE `+browseWhere+
				` AND w >= ? COLLATE NOCASE`+
				` AND (w > ? COLLATE NOCASE OR (w = ? COLLATE NOCASE AND id > ?))`+
				` AND w < ? COLLATE NOCASE`, anchorWord, anchorWord, anchorWord, entryID, word).Scan(&count)
			stats.Count = time.Since(countStarted)
			stats.CountedRows = count
			stats.Ordinal = ordinal + 1 + count
			return stats.Ordinal, stats, err
		}
		if err != sql.ErrNoRows {
			return 0, stats, err
		}
	}
	var n int
	countStarted := time.Now()
	err := s.db.QueryRow("SELECT COUNT(*) FROM entry WHERE "+browseWhere+" AND w < ? COLLATE NOCASE", word).Scan(&n)
	stats.Count = time.Since(countStarted)
	stats.CountedRows = n
	stats.Ordinal = n
	return n, stats, err
}

// Alphabet implements dict.Browser, once per open. A text.db is immutable
// while it is open - a re-ingest writes a new file and the registry reopens -
// so the strip is computed on the first browse and kept.
func (s *Store) Alphabet() ([]dict.Letter, error) {
	s.alphaOnce.Do(func() { s.alpha, s.alphaErr = s.buildAlphabet() })
	return s.alpha, s.alphaErr
}

// buildAlphabet counts initial runs through ordered-index seeks and labels each
// page from the last title before it and its first title. The anchors also
// provide the next page's first title for the optional range suffix.
func (s *Store) buildAlphabet() ([]dict.Letter, error) {
	seek, err := s.db.Prepare("SELECT w FROM entry WHERE " + browseWhere + " AND w >= ? COLLATE NOCASE" + browseOrder + " LIMIT 1")
	if err != nil {
		return nil, err
	}
	defer seek.Close()
	span, err := s.db.Prepare("SELECT COUNT(*) FROM entry WHERE " + browseWhere + " AND w >= ? COLLATE NOCASE AND w < ? COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer span.Close()
	tail, err := s.db.Prepare("SELECT COUNT(*) FROM entry WHERE " + browseWhere + " AND w >= ? COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer tail.Close()

	type initialRun struct {
		symbol string
		start  int
		end    int
	}
	var runs []initialRun
	total, from := 0, ""
	for i := 0; i < 1<<16; i++ {
		var word string
		err := seek.QueryRow(from).Scan(&word)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return nil, err
		}
		r, _ := utf8.DecodeRuneInString(word)
		hi, bounded := browseInitialRunEnd(r)
		var count int
		if bounded {
			err = span.QueryRow(word, hi).Scan(&count)
		} else {
			err = tail.QueryRow(word).Scan(&count)
		}
		if err != nil {
			return nil, err
		}
		if count <= 0 {
			return nil, fmt.Errorf("empty Browse initial range at %q", word)
		}
		symbol := browseInitial(r)
		runs = append(runs, initialRun{symbol: symbol, start: total, end: total + count})
		total += count
		if !bounded {
			break
		}
		from = hi
	}
	if total == 0 {
		return nil, nil
	}

	pageSize := indexconst.BrowsePageSize
	pageCount := (total + pageSize - 1) / pageSize
	pageRunLabels := make([]string, pageCount)
	pageNextRunLabels := make([]string, pageCount)
	runIndex := 0
	for page := range pageRunLabels {
		offset := page * pageSize
		for runIndex+1 < len(runs) && runs[runIndex].end <= offset {
			runIndex++
		}
		if runIndex < len(runs) && runs[runIndex].start <= offset && offset < runs[runIndex].end {
			pageRunLabels[page] = runs[runIndex].symbol
			if runIndex+1 < len(runs) && runs[runIndex+1].start < offset+pageSize {
				pageNextRunLabels[page] = runs[runIndex+1].symbol
			}
		}
	}
	pageFirst := make([]string, pageCount)
	pageLast := make([]string, pageCount)
	if s.hasBrowseAnchors && pageSize%indexconst.BrowseAnchorStride == 0 {
		rows, err := s.db.Query(`SELECT ordinal, w FROM browse_anchor
			WHERE ordinal % ? = 0 OR ordinal % ? = ? ORDER BY ordinal`,
			pageSize, pageSize, indexconst.BrowsePageLastOffset)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ordinal int
			var word string
			if err := rows.Scan(&ordinal, &word); err != nil {
				rows.Close()
				return nil, err
			}
			page, within := ordinal/pageSize, ordinal%pageSize
			if page < 0 || page >= pageCount {
				logx.SystemError("Browse anchor outside alphabet range ordinal=%d page=%d pages=%d", ordinal, page, pageCount)
				continue
			}
			switch within {
			case 0:
				pageFirst[page] = word
			case indexconst.BrowsePageLastOffset:
				pageLast[page] = word
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	out := make([]dict.Letter, pageCount)
	usedLabels := make(map[string]bool, pageCount)
	for page := range out {
		offset := page * pageSize
		label := pageRunLabels[page]
		if pageFirst[page] != "" {
			first := firstRune(pageFirst[page])
			initial := browseSectionInitial(first)
			previousLast := ""
			if page > 0 && pageLast[page-1] != "" {
				if (unicode.IsPunct(first) || unicode.IsSymbol(first)) && pageNextRunLabels[page] != "" &&
					browseSectionInitial(firstRune(pageLast[page-1])) == browseSectionInitial(first) {
					// This symbol already closes the preceding section. Start at the
					// first new initial that appears within this page.
					initial = pageNextRunLabels[page]
				} else {
					previousLast = pageLast[page-1]
				}
			}
			lastInitial := ""
			if pageLast[page] != "" && !unicode.Is(unicode.Han, first) {
				lastInitial = browseSectionInitial(firstRune(pageLast[page]))
			}
			label = dict.SectionLabel(previousLast, pageFirst[page], pageLast[page], initial, lastInitial, usedLabels)
		} else {
			usedLabels[label] = true
		}
		out[page] = dict.Letter{Letter: label, Offset: offset, Count: min(pageSize, total-offset)}
	}
	return out, nil
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

func browseSectionInitial(r rune) string {
	initial := browseInitial(r)
	if initial != "#" || !unicode.IsPrint(r) || unicode.IsSpace(r) {
		return initial
	}
	return string(r)
}

func browseInitial(r rune) string {
	f, _ := utf8.DecodeRuneInString(dict.Fold(string(r)))
	if f == utf8.RuneError {
		f = r
	}
	if lead, ok := hangulLeadOf(f); ok {
		return lead
	}
	switch {
	case unicode.IsDigit(f):
		return string(f)
	case unicode.Is(unicode.Han, f):
		return "漢"
	case unicode.Is(unicode.Hiragana, f):
		return "あ"
	case unicode.Is(unicode.Katakana, f):
		return "ア"
	case !unicode.IsLetter(f):
		return "#"
	}
	return string(unicode.ToUpper(f))
}

func browseInitialRunEnd(r rune) (string, bool) {
	switch {
	case r >= 0xAC00 && r <= 0xD7A3:
		return string(0xAC00 + ((r-0xAC00)/588+1)*588), true
	case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
		if end := browseBlockEnd(unicode.Han, r); end > 0 {
			return string(end), true
		}
		if end := browseBlockEnd(unicode.Hiragana, r); end > 0 {
			return string(end), true
		}
		if end := browseBlockEnd(unicode.Katakana, r); end > 0 {
			return string(end), true
		}
	}
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A'
	}
	r++
	if r >= 0xD800 && r <= 0xDFFF {
		r = 0xE000
	}
	if r > unicode.MaxRune {
		return "", false
	}
	return string(r), true
}

func browseBlockEnd(tbl *unicode.RangeTable, r rune) rune {
	for _, x := range tbl.R16 {
		if x.Stride == 1 && rune(x.Lo) <= r && r <= rune(x.Hi) {
			return rune(x.Hi) + 1
		}
	}
	for _, x := range tbl.R32 {
		if x.Stride == 1 && rune(x.Lo) <= r && r <= rune(x.Hi) {
			return rune(x.Hi) + 1
		}
	}
	return 0
}

var browseHangulLead = []rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")

func hangulLeadOf(r rune) (string, bool) {
	switch {
	case r >= 0xAC00 && r <= 0xD7A3:
		index := int((r - 0xAC00) / 588)
		if index < 0 || index >= len(browseHangulLead) {
			logx.SystemError("Browse Hangul initial outside range rune=U+%04X index=%d initials=%d", r, index, len(browseHangulLead))
			return "", false
		}
		return string(browseHangulLead[index]), true
	case r >= 0x1100 && r <= 0x1112:
		index := int(r - 0x1100)
		if index < 0 || index >= len(browseHangulLead) {
			logx.SystemError("Browse Hangul lead outside range rune=U+%04X index=%d initials=%d", r, index, len(browseHangulLead))
			return "", false
		}
		return string(browseHangulLead[index]), true
	}
	return "", false
}

// The browse contract is an interface assertion, not a convention: the server
// reaches this backend through dict.Browser and would otherwise silently fall
// back to "not indexed" if a method signature drifted.
var _ dict.Browser = (*Store)(nil)
