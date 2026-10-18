// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/wildcard"
)

// WildcardContext returns the entries whose headword fits a wildcard pattern
// (internal/wildcard, D167), in browse order. Entry headwords only: aliases
// and `@` sub-entries are not matched.
//
// The cost is set by how many headwords cross from SQLite into Go, ~250 ns
// each on either driver, against 10-80 ns to match one (D167):
//
//   - a case-sensitive pattern is SQLite's own GLOB, so only the hits cross;
//   - a case-insensitive one needs Fold, which SQLite does not have, so every
//     headword read crosses and is matched in Go.
//
// Both read only the slices of idx_entry_w the pattern's first character
// allows (wildcardRanges), and stop at the limit.
func (s *Store) WildcardContext(ctx context.Context, pattern string, limit int) ([]dict.Result, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, nil
	}
	p, err := wildcard.Parse(pattern)
	if err != nil {
		return nil, err
	}
	ids, err := s.wildcardIDs(ctx, p, clamp(limit))
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	return s.entriesByID(ctx, ids)
}

// keyRange is a slice of idx_entry_w, a NOCASE index: lo inclusive, hi
// exclusive; an empty bound is open.
type keyRange struct{ lo, hi string }

// wildcardRanges returns the slices of idx_entry_w that can hold a match, in
// index order, and whether the headwords starting with a nonspacing mark can
// too (wildcardMarks). NOCASE folds ASCII A-Z and compares every other byte as
// it is, so headwords whose first byte is not ASCII sort after every ASCII
// one, in byte order.
//
//   - No lead character (the pattern starts with `*` or `?`): the whole index.
//   - A case-sensitive pattern: the lead's own range. NOCASE puts "c" and "C"
//     in one range, a superset GLOB then narrows.
//   - A case-insensitive pattern: the range of every character that folds to
//     the lead (wildcard.Folds: c, C, ç, Ć, č, …), and the marks, which Fold
//     drops so that the character after them leads (D167 Am. 2). A lead of
//     U+FFFD is also what invalid UTF-8 reads as, so it takes every non-ASCII
//     headword.
func wildcardRanges(p wildcard.Pattern) (rs []keyRange, marks bool) {
	lead, ok := p.Lead()
	switch {
	case !ok:
		return []keyRange{{}}, false
	case p.Sensitive:
		return []keyRange{leadRange(lead)}, false
	case lead == utf8.RuneError:
		return []keyRange{{lo: "\x80"}}, false
	}
	for _, r := range wildcard.Folds(lead) {
		rs = append(rs, leadRange(r))
	}
	return joinRanges(rs), true
}

// joinRanges sorts rs and joins the ones that touch: adjacent characters
// (É Ê Ë) make one range, the hi of one being the lo of the next.
func joinRanges(rs []keyRange) []keyRange {
	slices.SortFunc(rs, func(a, b keyRange) int { return strings.Compare(a.lo, b.lo) })
	out := rs[:0]
	for _, k := range rs {
		if n := len(out); n > 0 && k.lo <= out[n-1].hi {
			out[n-1].hi = max(out[n-1].hi, k.hi)
			continue
		}
		out = append(out, k)
	}
	return out
}

// leadRange is the NOCASE range of the headwords that start with r.
func leadRange(r rune) keyRange {
	if r < utf8.RuneSelf {
		b := byte(r)
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A' // NOCASE compares the lowercase byte
		}
		return keyRange{string([]byte{b}), string([]byte{b + 1})}
	}
	return spanRanges(r, r)[0]
}

// spanRanges is the range of the headwords starting with a character in
// lo..hi, both non-ASCII, as one range per UTF-8 length: within one length
// the encoding keeps code point order. A UTF-8 sequence ends in a
// continuation byte (0x80-0xBF), so hi's last byte plus one is the first
// string past every string with hi's prefix.
func spanRanges(lo, hi rune) []keyRange {
	var out []keyRange
	for _, edge := range []rune{0x7FF, 0xFFFF, unicode.MaxRune} {
		if lo > edge {
			continue
		}
		top := min(hi, edge)
		h := string(top)
		out = append(out, keyRange{string(lo), h[:len(h)-1] + string([]byte{h[len(h)-1] + 1})})
		if hi <= edge {
			break
		}
		lo = edge + 1
	}
	return out
}

// where renders the range as a condition on w, and the arguments of its
// bounds.
func (k keyRange) where() (string, []any) {
	switch {
	case k.lo == "" && k.hi == "":
		return "", nil
	case k.hi == "":
		return " AND w >= ? COLLATE NOCASE", []any{k.lo}
	}
	return " AND w >= ? COLLATE NOCASE AND w < ? COLLATE NOCASE", []any{k.lo, k.hi}
}

// wildcardScan is the statement that streams one range's headwords in index
// order. With glob its first argument is the GLOB pattern, and only the
// headwords GLOB accepts leave SQLite.
func wildcardScan(cond string, glob bool) string {
	g := ""
	if glob {
		g = " AND w GLOB ?"
	}
	return "SELECT id, w FROM entry WHERE " + browseWhere + g + cond + browseOrder
}

// marksQuery reads every headword that starts with a nonspacing mark: one
// indexed range per wildcard.Marks range, joined in one statement, because
// there are a few hundred of them and almost all are empty. Its bounds are
// constants, written as text literals from their bytes.
var marksQuery = sync.OnceValue(func() string {
	var rs []keyRange
	for _, m := range wildcard.Marks() {
		rs = append(rs, spanRanges(m[0], m[1])...)
	}
	lit := func(s string) string { return fmt.Sprintf("CAST(x'%x' AS TEXT)", s) }
	var b strings.Builder
	b.WriteString("WITH r(lo, hi) AS (VALUES ")
	for i, k := range joinRanges(rs) {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("(" + lit(k.lo) + "," + lit(k.hi) + ")")
	}
	b.WriteString(") SELECT id, w FROM (" +
		"SELECT e.id AS id, e.w AS w FROM r JOIN entry e ON e.w >= r.lo COLLATE NOCASE AND e.w < r.hi COLLATE NOCASE" +
		") WHERE " + browseWhere + browseOrder)
	return b.String()
})

// keyHit is one matched headword and its entry.
type keyHit struct {
	id  int64
	key string
}

// wildcardMarks is the first n headwords starting with a nonspacing mark that
// m accepts, in NOCASE order. Such headwords are rare - a dictionary of
// affixes written with a leading combining mark - so they are read whole and
// merged into the range scan.
func (s *Store) wildcardMarks(ctx context.Context, m *wildcard.Matcher, n int) ([]keyHit, error) {
	s.marksOnce.Do(func() {
		var found bool
		if s.marksErr = s.db.QueryRow("SELECT EXISTS(" + marksQuery() + ")").Scan(&found); s.marksErr == nil && found {
			s.marksStmt, s.marksErr = s.db.Prepare(marksQuery())
		}
	})
	if s.marksStmt == nil || s.marksErr != nil {
		return nil, s.marksErr
	}
	rows, err := s.marksStmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []keyHit
	for len(out) < n && rows.Next() {
		var h keyHit
		if err := rows.Scan(&h.id, &h.key); err != nil {
			return nil, err
		}
		if m.Match([]byte(h.key)) {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

// scanStmt is the prepared statement for one wildcardScan shape: six of
// them, run once per range, a pattern reading up to a few dozen ranges.
func (s *Store) scanStmt(q string) (*sql.Stmt, error) {
	if st, ok := s.scanStmts.Load(q); ok {
		return st.(*sql.Stmt), nil
	}
	st, err := s.db.Prepare(q)
	if err != nil {
		return nil, err
	}
	if prev, loaded := s.scanStmts.LoadOrStore(q, st); loaded {
		st.Close()
		return prev.(*sql.Stmt), nil
	}
	return st, nil
}

// nocaseLess is SQLite's NOCASE order: bytes, with ASCII A-Z read as a-z.
func nocaseLess(a string, b []byte) bool {
	lower := func(c byte) byte {
		if 'A' <= c && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if x, y := lower(a[i]), lower(b[i]); x != y {
			return x < y
		}
	}
	return len(a) < len(b)
}

// wildcardIDs streams the headwords of every range in order, matches them -
// GLOB for a case-sensitive pattern, the Go matcher otherwise - merges in the
// ones that start with a mark, and returns the first n entries matched. No
// entry is read twice: the ranges are disjoint, and the marks are merged only
// when no range can hold a headword led by one. sql.RawBytes reuses one
// buffer for every row; nothing keeps it past the row it was scanned from.
func (s *Store) wildcardIDs(ctx context.Context, p wildcard.Pattern, n int) ([]int64, error) {
	ranges, withMarks := wildcardRanges(p)
	var m *wildcard.Matcher
	if !p.Sensitive {
		m = p.Matcher()
	}
	var marks []keyHit
	if withMarks {
		var err error
		if marks, err = s.wildcardMarks(ctx, m, n); err != nil {
			return nil, err
		}
	}
	var ids []int64
	for _, k := range ranges {
		if len(ids) >= n {
			break
		}
		cond, args := k.where()
		st, err := s.scanStmt(wildcardScan(cond, p.Sensitive))
		if err != nil {
			return nil, err
		}
		if p.Sensitive {
			args = append([]any{p.Glob()}, args...)
		}
		rows, err := st.QueryContext(ctx, args...)
		if err != nil {
			return nil, err
		}
		var (
			id int64
			w  sql.RawBytes
		)
		for len(ids) < n && rows.Next() {
			if err := rows.Scan(&id, &w); err != nil {
				rows.Close()
				return nil, err
			}
			if m != nil && !m.Match(w) {
				continue
			}
			for len(marks) > 0 && len(ids) < n && nocaseLess(marks[0].key, w) {
				ids = append(ids, marks[0].id)
				marks = marks[1:]
			}
			if len(ids) < n {
				ids = append(ids, id)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	for _, h := range marks {
		if len(ids) >= n {
			break
		}
		ids = append(ids, h.id)
	}
	return ids, nil
}

// entriesByID reads the headword and article of each id, in the order given.
// The bodies are read only here, for the rows that matched, never during the
// scan: rows carry their article inline.
func (s *Store) entriesByID(ctx context.Context, ids []int64) ([]dict.Result, error) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, w, m FROM entry WHERE id IN (?"+strings.Repeat(",?", len(ids)-1)+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[int64]dict.Result, len(ids))
	for rows.Next() {
		var (
			id   int64
			r    dict.Result
			body []byte // TEXT or a compressed BLOB; decodeBody tells them apart
		)
		if err := rows.Scan(&id, &r.Headword, &body); err != nil {
			return nil, err
		}
		r.Body = decodeBody(body)
		byID[id] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]dict.Result, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}
