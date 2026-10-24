// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
)

// maskIndexSpans narrows a literal-leading mask to the relevant slices of
// idx_entry_w. The mask regexp uses Unicode case folding while SQLite NOCASE
// folds only ASCII, so the non-ASCII tail must also be scanned: for example,
// a leading K can match the Kelvin sign. Each span remains in browse order.
// A wildcard or non-ASCII lead conservatively scans the whole index.
type maskIndexSpan struct {
	where string
	args  []any
}

// MaskCandidateStrategy reports the indexed candidate source used by prepared
// headword-mask searches. The complete mask is still checked after candidates
// are read.
func (s *Store) MaskCandidateStrategy(pattern string) string {
	strategy, _ := s.MaskCandidatePlan(pattern)
	return strategy
}

// MaskCandidatePlan reports the candidate source and literal passed to that
// index. The complete mask is still checked after candidates are read.
func (s *Store) MaskCandidatePlan(pattern string) (string, string) {
	if !strings.ContainsAny(pattern, "*?") {
		return "exact_key", ""
	}
	if literal := maskTrigramLiteral(pattern); s.hasTrigram && !s.staleFold && literal != "" {
		return "trigram", literal
	}
	if maskHasASCIILead(pattern) {
		return "headword_range", ""
	}
	return "headword_scan", ""
}

func maskIndexSpans(pattern string) []maskIndexSpan {
	if !maskHasASCIILead(pattern) {
		return []maskIndexSpan{{}}
	}
	lead := pattern[0]
	if 'A' <= lead && lead <= 'Z' {
		lead += 'a' - 'A'
	}
	return []maskIndexSpan{
		{where: " AND w >= ? COLLATE NOCASE AND w < ? COLLATE NOCASE", args: []any{string(lead), string(lead + 1)}},
		{where: " AND w >= ? COLLATE NOCASE", args: []any{"\x80"}},
	}
}

func maskHasASCIILead(pattern string) bool {
	return len(pattern) > 0 && pattern[0] != '*' && pattern[0] != '?' && pattern[0] < 0x7f
}

// maskTrigramLiteral selects the longest literal run the folded trigram index
// can safely use as a candidate filter. A rune whose Unicode simple-fold
// equivalents do not all produce the same dict.Fold text splits the run:
// regexp (?i) treats those equivalents alike, while the persisted trigram
// index does not (for example, long s and ordinary s).
func maskTrigramLiteral(pattern string) string {
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
