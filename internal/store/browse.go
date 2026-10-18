// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
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
		if literal := maskTrigramLiteral(query); s.hasTrigram && !s.staleFold && literal != "" && !maskHasASCIILead(query) {
			phrase := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
			rows, queryErr := s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_trigram t
				JOIN entry e ON e.id=t.rowid WHERE entry_trigram MATCH ? AND `+browseWhere+
				` ORDER BY e.w COLLATE NOCASE`, phrase)
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
		if len(s.words) >= dict.MaskPageSize || (len(s.words) > 0 && !now.Before(s.nextProgress)) {
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

// FindHeadwordsMask scans the existing headword index and pages mask matches.
func (s *Store) FindHeadwordsMask(ctx context.Context, query string, offset, limit int) ([]string, int, []dict.Letter, error) {
	words, total, letters, _, err := s.FindHeadwordsMaskWithin(ctx, query, "", false, offset, limit)
	return words, total, letters, err
}

// FindHeadwordsMaskWithin intersects the main mask with the bottom Browse
// control. Positions address the unfiltered mask list for match navigation.
func (s *Store) FindHeadwordsMaskWithin(ctx context.Context, query, within string, withinMask bool, offset, limit int) ([]string, int, []dict.Letter, []int, error) {
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

	// Use the optional trigram index as a candidate source when the mask has
	// a sufficiently long literal run. The complete mask still verifies every
	// candidate. Otherwise, retain the regular headword-index scan.
	if literal := maskTrigramLiteral(query); s.hasTrigram && !s.staleFold && literal != "" && !maskHasASCIILead(query) {
		phrase := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT e.w FROM entry_trigram t
			JOIN entry e ON e.id=t.rowid WHERE entry_trigram MATCH ? AND `+browseWhere+
			` ORDER BY e.w COLLATE NOCASE`, phrase)
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

// Locate implements dict.Browser.
func (s *Store) Locate(word string) (int, error) {
	if s.hasBrowseAnchors {
		var ordinal, entryID int
		var anchorWord string
		err := s.db.QueryRow(`SELECT ordinal, entry_id, w FROM browse_anchor
			WHERE w < ? COLLATE NOCASE ORDER BY w COLLATE NOCASE DESC, entry_id DESC LIMIT 1`, word).
			Scan(&ordinal, &entryID, &anchorWord)
		if err == nil {
			var count int
			err = s.db.QueryRow(`SELECT COUNT(*) FROM entry WHERE `+browseWhere+
				` AND (w COLLATE NOCASE, id) > (? COLLATE NOCASE, ?)`+
				` AND w < ? COLLATE NOCASE`, anchorWord, entryID, word).Scan(&count)
			return ordinal + count, err
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM entry WHERE "+browseWhere+" AND w < ? COLLATE NOCASE", word).Scan(&n)
	return n, err
}

// Alphabet implements dict.Browser, once per open. A text.db is immutable
// while it is open - a re-ingest writes a new file and the registry reopens -
// so the strip is computed on the first browse and kept.
func (s *Store) Alphabet() ([]dict.Letter, error) {
	s.alphaOnce.Do(func() { s.alpha, s.alphaErr = s.buildAlphabet() })
	return s.alpha, s.alphaErr
}

// maxInitials bounds buildAlphabet's walk. It is not a limit on the strip, nor
// one the walk is expected to approach: initialRunEnd jumps a whole script in
// one step, so a real dictionary spends one iteration per distinct initial that
// actually earns a chip. This only refuses to be unbounded.
const maxInitials = 1 << 16

// How large a chip may get before it is cut into pieces.
//
// An initial is a chip only while it is a usable one. 漢 over a Chinese
// encyclopaedia is 70,000 headwords behind a single tab - the strip is drawn,
// the dictionary is still unbrowsable, and the reader's only recourse is 230
// page turns. The same happens to S in a large enough alphabetic dictionary;
// Han is just where it happens first and hardest.
//
// So a run longer than the span is cut into pieces of that span, each labelled
// with the first headword it holds. 3000 is ten pages - short enough to page
// through, long enough that a modest dictionary is never chopped up - and the
// budget keeps the strip from growing with the dictionary: a 3 M-headword
// title gets pieces of 90,000 rather than a thousand chips.
const (
	chipCeiling = 3000
	chipBudget  = 32
)

// browseRun is one contiguous stretch of the index sharing an initial, as the
// walk finds it. Contiguous is the point: a piece of a run can be addressed by
// counting rows from its start, which is not true of a chip, because chips
// merge runs that are far apart (É … é).
type browseRun struct {
	label  string
	lo, hi string // first headword, and the exclusive bound ("" = to the end)
	offset int
	count  int
}

// buildAlphabet walks the index one initial at a time.
//
// The naive spelling of this is `GROUP BY substr(w,1,1)`, and it is a trap:
// SQLite cannot use an index for a grouping expression, so it sorts every row
// in the dictionary into a temp b-tree to produce forty numbers. This walks
// instead - seek to the first headword at or after a bound, take its initial,
// count the range that initial owns, and use the range's upper bound as the
// next seek - so the counting is exactly one pass over the index (each row
// counted once, in C), plus one O(log n) seek per initial.
//
// Cutting the long runs (below) costs a second pass at most, and only over the
// runs that are actually too long: each boundary is found by walking `span`
// index entries on from the previous one, never by an OFFSET from the start.
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

	var runs []browseRun
	total, from, done := 0, "", false
	for i := 0; i < maxInitials; i++ {
		var w string
		switch err := seek.QueryRow(from).Scan(&w); {
		case err == sql.ErrNoRows:
			done = true
		case err != nil:
			return nil, err
		}
		if done {
			break
		}
		r, _ := utf8.DecodeRuneInString(w)
		hi, bounded := initialRunEnd(r)
		var n int
		if bounded {
			err = span.QueryRow(w, hi).Scan(&n)
		} else {
			err = tail.QueryRow(w).Scan(&n)
		}
		if err != nil {
			return nil, err
		}
		if n <= 0 { // w itself is in the range, so this cannot happen
			break
		}
		if !bounded {
			hi = ""
		}
		runs = append(runs, browseRun{label: initialOf(r), lo: w, hi: hi, offset: total, count: n})
		total += n
		if !bounded {
			break
		}
		from = hi
	}
	if len(runs) == 0 {
		return nil, nil
	}

	cut := total / chipBudget
	if cut < chipCeiling {
		cut = chipCeiling
	}

	var out []dict.Letter
	at := map[string]int{} // chip label -> its index in out
	// Merge by label, not by adjacency. Under NOCASE only ASCII case folds, so
	// "Ä" and "ä" are two separate runs with half the Latin supplement between
	// them - and both belong under the same "A". First occurrence wins the
	// offset: the chip goes where the bulk is.
	emit := func(label string, offset, n int) {
		if j, seen := at[label]; seen {
			out[j].Count += n
			return
		}
		at[label] = len(out)
		out = append(out, dict.Letter{Letter: label, Offset: offset, Count: n})
	}

	var after, afterOpen *sql.Stmt
	for _, run := range runs {
		if run.count <= cut {
			emit(run.label, run.offset, run.count)
			continue
		}
		if after == nil {
			if after, err = s.db.Prepare("SELECT w FROM entry WHERE " + browseWhere + " AND w >= ? COLLATE NOCASE AND w < ? COLLATE NOCASE" + browseOrder + " LIMIT 1 OFFSET ?"); err != nil {
				return nil, err
			}
			defer after.Close()
			if afterOpen, err = s.db.Prepare("SELECT w FROM entry WHERE " + browseWhere + " AND w >= ? COLLATE NOCASE" + browseOrder + " LIMIT 1 OFFSET ?"); err != nil {
				return nil, err
			}
			defer afterOpen.Close()
		}
		// The first piece keeps the run's own label, so the strip still reads
		// A B C … and 漢 still says where the Han section starts; the pieces
		// after it are labelled by the headword they open on.
		lo, offset, left, label, prev := run.lo, run.offset, run.count, run.label, ""
		for left > cut {
			emit(label, offset, cut)
			prev, offset, left = label, offset+cut, left-cut
			var w string
			if run.hi != "" {
				err = after.QueryRow(lo, run.hi, cut).Scan(&w)
			} else {
				err = afterOpen.QueryRow(lo, cut).Scan(&w)
			}
			if err == sql.ErrNoRows {
				break // counted rows the walk can no longer reach
			}
			if err != nil {
				return nil, err
			}
			lo = w
			if label = chipPrefixFor(w, prev); label == "" {
				label = run.label
			}
		}
		emit(label, offset, left)
	}
	return out, nil
}

// chipPrefixFor labels a piece of a cut run with the headword it opens on,
// taking as few characters as tell it apart from the piece before it: one for
// Han, where the character is the whole word's identity, two or three where a
// long alphabetic run is being cut and "S" would otherwise be the label of
// every piece of S.
func chipPrefixFor(w, prev string) string {
	label := ""
	for n := 1; n <= 3; n++ {
		if label = chipPrefix(w, n); label != prev {
			break
		}
	}
	return label
}

// chipPrefix is the first n characters of a headword as a chip shows them:
// folded (so an accent does not change what the tab looks like) with the
// initial capitalised, exactly as initialOf presents a whole-letter chip.
func chipPrefix(w string, n int) string {
	rs := []rune(dict.Fold(w))
	if len(rs) == 0 {
		rs = []rune(w)
	}
	if len(rs) == 0 {
		return ""
	}
	if n > len(rs) {
		n = len(rs)
	}
	rs = rs[:n:n]
	rs[0] = unicode.ToUpper(rs[0])
	return string(rs)
}

// afterInitial returns the smallest string that sorts after every headword
// beginning with r, under the collation the index was built with.
//
// The fold matters and is easy to get wrong: SQLite's NOCASE folds ASCII and
// nothing else, so the successor of "Z" is not "[" (which sorts BELOW every
// lowercase letter and would cut the z-words out of their own range) but the
// successor of "z". Outside ASCII, NOCASE is byte order, and r+1 is the
// successor of r's whole range for the same reason UTF-8 sorts by code point.
func afterInitial(r rune) (string, bool) {
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A'
	}
	r++
	if r >= 0xD800 && r <= 0xDFFF { // string(rune) of a surrogate is U+FFFD
		r = 0xE000
	}
	if r > unicode.MaxRune {
		return "", false
	}
	return string(r), true
}

// initialRunEnd is the upper bound of the whole run that shares a chip, which
// is afterInitial for a letter and something much larger for a script initialOf
// collapses.
//
// Without it the walk steps one CODE POINT at a time, and a Chinese dictionary
// has fifteen thousand distinct initials - thirty thousand queries to produce a
// strip that will merge every one of them into 漢. Korean is worse in kind
// (11,172 precomposed syllables) and better in outcome, since those really do
// carry nineteen different chips. Both are jumped in one step per chip instead:
// a Han run ends where its Unicode block does, a Hangul run where its leading
// jamo changes.
//
// The run must be exactly what one chip covers, or the count would belong to
// the wrong tab. That holds because the jump is taken only where initialOf is
// constant across it: every syllable of one lead group has that lead, every
// character of the Han block is 漢. CJK is unaffected by dict.Fold, so the
// rune the label is computed from is the rune bounded here.
func initialRunEnd(r rune) (string, bool) {
	switch {
	case r >= 0xAC00 && r <= 0xD7A3:
		return string(0xAC00 + ((r-0xAC00)/588+1)*588), true
	case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
		for _, tbl := range []*unicode.RangeTable{unicode.Han, unicode.Hiragana, unicode.Katakana} {
			if e := blockEnd(tbl, r); e > 0 {
				return string(e), true
			}
		}
	}
	return afterInitial(r)
}

// blockEnd is the code point after the contiguous range of tbl holding r, or 0
// when tbl does not hold it in one. A strided range is declined rather than
// approximated: it would claim characters the table does not contain, and
// afterInitial's one-step answer is always correct, only slower.
func blockEnd(tbl *unicode.RangeTable, r rune) rune {
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

// hangulLead is the nineteen leading consonants of the Hangul syllable block,
// in block order: syllable (c-0xAC00)/588 is its initial. Korean headwords are
// precomposed syllables, so without this every Korean dictionary would collapse
// to one chip - and with it the strip is the ㄱㄴㄷ a reader expects.
var hangulLead = []rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")

// hangulLeadOf is the ㄱㄴㄷ chip for a Korean initial, whether the headword
// is written in precomposed syllables (the normal case) or in the conjoining
// choseong NFD leaves behind. Both answer with the COMPATIBILITY jamo, which
// is the form that renders on its own in a tab.
func hangulLeadOf(r rune) (string, bool) {
	switch {
	case r >= 0xAC00 && r <= 0xD7A3:
		return string(hangulLead[(r-0xAC00)/588]), true
	case r >= 0x1100 && r <= 0x1112:
		return string(hangulLead[r-0x1100]), true
	}
	return "", false
}

// initialOf is the chip a headword's first rune belongs under.
//
// Accents fold away (dict.Fold), so "élan" is under E where a reader looks for
// it. Scripts written without an alphabet get one chip for the whole script
// rather than one per character: a Han dictionary has tens of thousands of
// distinct initials, and a strip with tens of thousands of chips is not a
// strip. What makes that survivable rather than useless is the cut above - the
// one 漢 chip becomes 漢 一 丁 三 … , pieces of equal size labelled by the
// headword each opens on - which is a thumb index rather than a collation the
// index cannot supply.
func initialOf(r rune) string {
	// Korean is read BEFORE the fold, because the fold would destroy the
	// answer: dict.Fold normalises to NFD, and NFD decomposes a precomposed
	// Hangul syllable into conjoining jamo (각 -> U+1100 U+1161). The folded
	// rune is then a choseong, which is a letter, so the generic tail would
	// label the chip U+1100 - a joining form that renders as a dotted circle
	// or a sliver in most fonts, where the reader expects ㄱ.
	if lead, ok := hangulLeadOf(r); ok {
		return lead
	}
	f, _ := utf8.DecodeRuneInString(dict.Fold(string(r)))
	if f == utf8.RuneError {
		f = r
	}
	if lead, ok := hangulLeadOf(f); ok { // a headword already written in jamo
		return lead
	}
	switch {
	case unicode.IsDigit(f):
		return "0-9"
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

// The browse contract is an interface assertion, not a convention: the server
// reaches this backend through dict.Browser and would otherwise silently fall
// back to "not indexed" if a method signature drifted.
var _ dict.Browser = (*Store)(nil)
