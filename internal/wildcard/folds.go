// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wildcard

import (
	"slices"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Folds returns every character Fold maps to f, in code point order. A
// headword fits a case-insensitive pattern whose first character is f only if
// its own first character is one of these, or a nonspacing mark Fold drops
// (Marks): what lets a search read the headwords starting with them and no
// others.
func Folds(f rune) []rune {
	foldsOnce.Do(buildFolds)
	out := slices.Clone(folds[f])
	if Fold(f) == f {
		i, _ := slices.BinarySearch(out, f)
		out = slices.Insert(out, i, f)
	}
	return out
}

// Marks returns the code point ranges, inclusive, that hold every nonspacing
// mark (Mn) - the characters Fold drops, so a headword starting with one
// matches by what follows it. Ranges a short run of characters no headword
// starts with apart (no letter, digit, punctuation, symbol or space between
// them) are joined: a range that covers more is a superset, and the Matcher
// rejects what does not fit. Ascending, none below U+0080.
func Marks() [][2]rune {
	marksOnce.Do(buildMarks)
	return marks
}

var (
	foldsOnce sync.Once
	folds     map[rune][]rune // Fold(r) → every r != Fold(r) folding to it, ascending

	marksOnce sync.Once
	marks     [][2]rune
)

// buildFolds inverts Fold. Only the characters Fold changes are recorded:
// a few thousand, out of the BMP table and, past it, out of the assigned
// letters, marks, digits, punctuation and symbols (an unassigned code point
// lowercases and decomposes to itself).
func buildFolds() {
	m := map[rune][]rune{}
	add := func(r rune) {
		if f := Fold(r); f >= 0 && f != r {
			m[f] = append(m[f], r)
		}
	}
	for r := rune(0); r < 0x10000; r++ {
		if r < 0xD800 || r > 0xDFFF {
			add(r)
		}
	}
	var buf [utf8.UTFMax]byte
	for _, t := range []*unicode.RangeTable{unicode.L, unicode.M, unicode.N, unicode.P, unicode.S} {
		for _, rg := range t.R32 {
			for r := rune(rg.Lo); r <= rune(rg.Hi); r += rune(rg.Stride) {
				if r < 0x10000 {
					continue
				}
				// Fold changes r only by lowercasing or decomposing it.
				if unicode.ToLower(r) == r && norm.NFD.Properties(buf[:utf8.EncodeRune(buf[:], r)]).Decomposition() == nil {
					continue
				}
				add(r)
			}
		}
	}
	for _, rs := range m {
		slices.Sort(rs)
	}
	folds = m
}

// marksGap is the widest run of characters between two nonspacing marks that
// Marks joins over, once it has checked that no headword can start inside it.
const marksGap = 64

func buildMarks() {
	var out [][2]rune
	walk := func(lo, hi, stride rune) {
		for r := lo; r <= hi; r += stride {
			if r < utf8.RuneSelf {
				continue
			}
			if n := len(out); n > 0 && r-out[n-1][1] <= marksGap+1 && blankRun(out[n-1][1]+1, r) {
				out[n-1][1] = r
				continue
			}
			out = append(out, [2]rune{r, r})
		}
	}
	for _, rg := range unicode.Mn.R16 {
		walk(rune(rg.Lo), rune(rg.Hi), rune(rg.Stride))
	}
	for _, rg := range unicode.Mn.R32 {
		walk(rune(rg.Lo), rune(rg.Hi), rune(rg.Stride))
	}
	marks = out
}

// blankRun reports that no character in [lo, hi) is one a headword starts
// with in practice: a letter, digit, punctuation, symbol or space.
func blankRun(lo, hi rune) bool {
	for r := lo; r < hi; r++ {
		if unicode.In(r, unicode.L, unicode.N, unicode.P, unicode.S, unicode.Z) {
			return false
		}
	}
	return true
}
