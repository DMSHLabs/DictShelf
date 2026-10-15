// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/wildcard"
)

// WildcardContext returns the entries whose headword fits a wildcard pattern
// (internal/wildcard, D167), in browse order. Entry headwords only: aliases
// and `@` sub-entries are not matched.
//
// The cost is set by how many headwords cross from SQLite into Go, ~250 ns
// each on either driver, against 10-80 ns to match one (docs/OPEN.md O13):
//
//   - a case-sensitive pattern is SQLite's own GLOB, so only the hits cross;
//   - a case-insensitive one needs Fold, which SQLite does not have, so every
//     headword read crosses and is matched in Go.
//
// Both read only the slice of idx_entry_w the pattern's first character
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
	n := clamp(limit)
	var ids []int64
	if p.Sensitive {
		ids, err = s.wildcardGlob(ctx, p, n)
	} else {
		ids, err = s.wildcardScan(ctx, p, n)
	}
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	return s.entriesByID(ctx, ids)
}

// keyRange is a slice of idx_entry_w under NOCASE: lo inclusive, hi exclusive;
// an empty bound is open.
type keyRange struct{ lo, hi string }

// wildcardRanges returns the slices of idx_entry_w that can hold a match, in
// index order. NOCASE folds ASCII A-Z and compares every other byte as it is,
// so headwords whose first byte is not ASCII sort after every ASCII one.
//
//   - No lead character (the pattern starts with `*` or `?`): the whole index.
//   - A case-sensitive pattern: the lead's own range. NOCASE puts "c" and "C"
//     in one range, a superset GLOB then narrows.
//   - A case-insensitive pattern with an ASCII lead: the lead's range, and
//     every headword with a non-ASCII first byte, because Fold maps ç, ć, č -
//     and a leading combining mark, which it drops - onto c. Fold never maps
//     one ASCII character to another, so no other ASCII range can match.
//   - A case-insensitive pattern with a non-ASCII lead: every headword with a
//     non-ASCII first byte. The characters that fold to the lead are not one
//     range, but none is ASCII: Fold maps an ASCII character to an ASCII one
//     and drops only non-ASCII marks.
func wildcardRanges(p wildcard.Pattern) []keyRange {
	lead, ok := p.Lead()
	switch {
	case !ok:
		return []keyRange{{}}
	case p.Sensitive:
		return []keyRange{leadRange(lead)}
	case lead < utf8.RuneSelf:
		return []keyRange{leadRange(lead), {lo: "\x80"}}
	}
	return []keyRange{{lo: "\x80"}}
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
	// A UTF-8 sequence ends in a continuation byte (0x80-0xBF), so the last
	// byte plus one is the first string past every string with this prefix.
	lo := string(r)
	return keyRange{lo, lo[:len(lo)-1] + string([]byte{lo[len(lo)-1] + 1})}
}

// where renders the range as a condition on w, for browseWhere's statements.
func (k keyRange) where() (string, []any) {
	switch {
	case k.lo == "" && k.hi == "":
		return "", nil
	case k.hi == "":
		return " AND w >= ? COLLATE NOCASE", []any{k.lo}
	}
	return " AND w >= ? COLLATE NOCASE AND w < ? COLLATE NOCASE", []any{k.lo, k.hi}
}

// wildcardGlob matches a case-sensitive pattern inside SQLite.
func (s *Store) wildcardGlob(ctx context.Context, p wildcard.Pattern, n int) ([]int64, error) {
	var ids []int64
	for _, k := range wildcardRanges(p) {
		cond, args := k.where()
		args = append([]any{p.Glob()}, append(args, n-len(ids))...)
		rows, err := s.db.QueryContext(ctx,
			"SELECT id FROM entry WHERE "+browseWhere+" AND w GLOB ?"+cond+browseOrder+" LIMIT ?", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(ids) >= n {
			break
		}
	}
	return ids, nil
}

// wildcardScan matches a case-insensitive pattern in Go, streaming the
// headwords in index order and stopping at the limit. sql.RawBytes reuses one
// buffer for every row; nothing keeps it past the row it was scanned from.
func (s *Store) wildcardScan(ctx context.Context, p wildcard.Pattern, n int) ([]int64, error) {
	m := p.Matcher()
	var ids []int64
	for _, k := range wildcardRanges(p) {
		cond, args := k.where()
		rows, err := s.db.QueryContext(ctx, "SELECT id, w FROM entry WHERE "+browseWhere+cond+browseOrder, args...)
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
			if m.Match(w) {
				ids = append(ids, id)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(ids) >= n {
			break
		}
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
