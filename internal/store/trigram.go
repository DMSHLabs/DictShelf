// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"strings"
	"unicode/utf8"
)

// trigramMatch asks FTS5 only for one trigram. The query is a candidate filter;
// callers verify the complete substring against the stored folded headword or
// with their existing matcher. This also works with detail=none trigram tables.
func trigramMatch(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	n := 0
	for i, r := range s {
		if n == 3 {
			break
		}
		if r == '"' {
			b.WriteString(`""`)
		} else {
			_, size := utf8.DecodeRuneInString(s[i:])
			b.WriteString(s[i : i+size])
		}
		n++
	}
	if n < 3 {
		return ""
	}
	b.WriteByte('"')
	return b.String()
}

// trigramCandidateWindows is how many trigrams of one term the candidate filter
// may use. Three is the measured choice (docs/MEASUREMENTS.md): the second and
// third windows cost a doclist walk each and repay it by not handing the matcher
// rows that cannot hold the term.
const trigramCandidateWindows = 3

// trigramCandidates builds the FTS5 candidate expression for a folded term: up
// to trigramCandidateWindows single trigrams of that term, ANDed. It is a
// candidate filter like trigramMatch, and it stays a SUPERSET of the answer for
// the same reason one trigram did - every trigram of a term occurs in any text
// holding that term, so a row the matcher accepts satisfies all of them, and the
// matcher still decides the answer.
//
// Several windows, not three letters: an index of three-character pieces narrows
// by how RARE its pieces are, not by how long the query is. Three consecutive
// windows of `revolution` all sit inside its first five letters and filter
// exactly like `rev`, while `rev` and `ion` together keep the words that have
// both ends. detail=none refuses a PHRASE, so several one-token queries ANDed is
// the only way to use more than three runes of the query at all.
//
// The windows are the first, the last, and - when the term is long enough for a
// third to be somewhere else - one in the middle. For a term of three runes they
// are one and the same window, which is exactly the single trigram this filter
// has always used.
func trigramCandidates(folded string) string {
	runes := []rune(folded)
	if len(runes) < 3 {
		return ""
	}
	starts := []int{0, len(runes) - 3}
	if trigramCandidateWindows >= 3 && len(runes) >= 6 {
		starts = []int{0, (len(runes) - 3) / 2, len(runes) - 3}
	}
	parts := make([]string, 0, len(starts))
	seen := make(map[string]bool, len(starts))
	for _, start := range starts {
		piece := trigramMatch(string(runes[start : start+3]))
		if piece == "" || seen[piece] {
			continue
		}
		seen[piece] = true
		parts = append(parts, piece)
	}
	return strings.Join(parts, " AND ")
}
