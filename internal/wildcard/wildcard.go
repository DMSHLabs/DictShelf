// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package wildcard reads and matches wildcard queries (D167): `?` is exactly
// one character, `*` any run of characters including none, and a pattern
// covers the whole headword. Nothing else is special - no character sets, no
// escape - so `*` and `?` in a pattern are always wildcards.
//
// Case follows ripgrep's smart case: a pattern with an uppercase or titlecase
// letter is matched as typed, case and accents included; any other pattern is
// matched case- and accent-insensitively through Fold. A case-sensitive
// pattern is SQLite GLOB's own syntax (Glob), so the prepared backend matches
// it without a row leaving SQLite; Matcher is the Go side of the same rules.
package wildcard

import (
	"errors"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxLen is the longest pattern, in characters, after `**` collapses to `*`.
const MaxLen = 64

// ErrTooLong is returned by Parse for a pattern over MaxLen characters.
var ErrTooLong = errors.New("wildcard pattern longer than 64 characters")

// Pattern is a parsed wildcard query. The zero value matches only an empty
// headword.
type Pattern struct {
	// Sensitive is true when the pattern has a capital letter: it is then
	// matched as typed, without Fold.
	Sensitive bool
	text      string // the pattern as typed, `**` collapsed
	runes     []rune // what Matcher compares: folded unless Sensitive
}

// HasWildcards reports whether s contains `*` or `?`.
func HasWildcards(s string) bool { return strings.ContainsAny(s, "*?") }

// Parse reads a wildcard query. Invalid UTF-8 reads as U+FFFD, as everywhere
// a Go string is ranged over.
func Parse(s string) (Pattern, error) {
	raw := collapse([]rune(s))
	if len(raw) > MaxLen {
		return Pattern{}, ErrTooLong
	}
	p := Pattern{text: string(raw)}
	for _, r := range raw {
		if unicode.IsUpper(r) || unicode.IsTitle(r) {
			p.Sensitive = true
			break
		}
	}
	if p.Sensitive {
		p.runes = raw
		return p, nil
	}
	folded := make([]rune, 0, len(raw))
	for _, r := range raw {
		if f := Fold(r); f >= 0 {
			folded = append(folded, f)
		}
	}
	// Fold drops a combining mark, which can leave two stars adjacent: `*́*`.
	p.runes = collapse(folded)
	return p, nil
}

// collapse rewrites every run of `*` as one `*`, in place.
func collapse(rs []rune) []rune {
	out := rs[:0]
	for _, r := range rs {
		if r == '*' && len(out) > 0 && out[len(out)-1] == '*' {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Lead returns the character every matching headword starts with - folded
// unless the pattern is Sensitive - and false when the pattern starts with a
// wildcard or is empty.
func (p Pattern) Lead() (rune, bool) {
	if len(p.runes) == 0 || p.runes[0] == '*' || p.runes[0] == '?' {
		return 0, false
	}
	return p.runes[0], true
}

// Glob returns the pattern in SQLite GLOB syntax, for a Sensitive pattern.
// `*` and `?` are GLOB's own; `[` opens a character set in GLOB and is written
// as the set `[[]`; a `]` outside a set is already literal.
func (p Pattern) Glob() string { return strings.ReplaceAll(p.text, "[", "[[]") }

// Matcher matches headwords against one pattern. It reuses a buffer, so it
// belongs to one goroutine.
type Matcher struct {
	p         []rune
	sensitive bool
	buf       []rune
}

// Matcher returns a matcher for p.
func (p Pattern) Matcher() *Matcher {
	return &Matcher{p: p.runes, sensitive: p.Sensitive, buf: make([]rune, 0, 64)}
}

// Match reports whether the headword w fits the pattern, folding w the way
// Parse folded the pattern.
func (m *Matcher) Match(w []byte) bool {
	m.buf = m.buf[:0]
	for len(w) > 0 {
		r, n := utf8.DecodeRune(w)
		w = w[n:]
		if !m.sensitive {
			if r = Fold(r); r < 0 {
				continue
			}
		}
		m.buf = append(m.buf, r)
	}
	return match(m.p, m.buf)
}

// match is the iterative wildcard algorithm with one backtrack point, the
// last `*` seen: on a mismatch the star takes one more character and the
// pattern resumes after it. Earlier stars never need revisiting, because a
// later star can absorb anything they would. O(len(p)·len(s)) at worst.
func match(p, s []rune) bool {
	pi, si := 0, 0
	star, mark := -1, 0
	for si < len(s) {
		switch {
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, si
			pi++
		case pi < len(p) && (p[pi] == '?' || p[pi] == s[si]):
			pi++
			si++
		case star >= 0:
			mark++
			pi, si = star+1, mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// Fold maps one character to the form a case- and accent-insensitive pattern
// compares, or -1 when the character is dropped:
//
//   - a nonspacing mark (Mn) is dropped;
//   - otherwise the character is lowercased and decomposed (NFD); when every
//     character after the first is a nonspacing mark, the first is kept (é →
//     e, Й → и); otherwise the lowercase character is kept whole (a Hangul
//     syllable, a Tamil two-part vowel).
//
// Per character, unlike dict.Fold, so that a pattern and a headword fold
// character by character and `?` still means one character. Nothing folded
// here is stored, so it carries no version.
func Fold(r rune) rune {
	if r >= 0 && r < 0x10000 {
		foldOnce.Do(buildFold)
		return foldBMP[r]
	}
	return foldRune(r)
}

var (
	foldOnce sync.Once
	foldBMP  *[0x10000]rune // 256 kB, built on the first wildcard search
)

func buildFold() {
	t := new([0x10000]rune)
	for r := range t {
		t[r] = foldRune(rune(r))
	}
	foldBMP = t
}

// foldRune is Fold's definition; the BMP table is this function, tabulated.
func foldRune(r rune) rune {
	if unicode.Is(unicode.Mn, r) {
		return -1
	}
	l := unicode.ToLower(r)
	d := norm.NFD.String(string(l))
	first, size := utf8.DecodeRuneInString(d)
	for _, x := range d[size:] {
		if !unicode.Is(unicode.Mn, x) {
			return l
		}
	}
	return first
}
