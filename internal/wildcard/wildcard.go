// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package wildcard reads and matches the patterns behind wildcard, prefix and
// exact search on a prepared dictionary (D167, D181): `?` is exactly one
// character, `*` any run of characters including none, and a pattern covers
// the whole headword. In a wildcard query nothing else is special - no
// character sets, no escape - so `*` and `?` are always wildcards (Parse);
// exact and prefix search read every character literally (Literal).
//
// Case and accents rank, they never exclude (D181). A headword matches when
// it fits the pattern in one of three readings - as typed, with case ignored,
// with case and accents ignored (Fold) - and Tiers says which readings come
// first: a capital letter typed puts the as-typed reading first, an accent
// typed puts the readings that keep accents before the one that drops them.
// The as-typed reading is SQLite GLOB's own syntax (Glob), so the prepared
// backend can match it without a row leaving SQLite; Matcher is the Go side
// of all three.
package wildcard

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxLen is the longest wildcard query, in characters, after `**` collapses
// to `*`.
const MaxLen = 64

// ErrTooLong is returned by Parse for a pattern over MaxLen characters.
var ErrTooLong = errors.New("wildcard pattern longer than 64 characters")

// The two wildcards, outside the range of characters, so that a literal `*`
// or `?` - an exact or prefix query - is an ordinary character. Fold's -1
// (a dropped mark) is a third value no token takes.
const (
	anyRun rune = -2 // `*`
	anyOne rune = -3 // `?`
)

// Reading is one way of comparing a pattern with a headword.
type Reading uint8

const (
	AsTyped     Reading = iota // character for character
	CaseIgnored                // both lowercased (unicode.ToLower)
	Folded                     // both through Fold: case and accents ignored
)

// apply maps one character of a pattern or a headword under the reading; -1
// drops it.
func (r Reading) apply(c rune) rune {
	switch r {
	case CaseIgnored:
		return unicode.ToLower(c)
	case Folded:
		return Fold(c)
	}
	return c
}

// Pattern is a parsed query. The zero value matches only an empty headword.
type Pattern struct {
	toks    []rune // as typed, `**` collapsed; anyRun and anyOne are the wildcards
	capital bool   // an uppercase or titlecase letter
	accent  bool   // a character Fold changes beyond its case: an accent, a mark
}

// HasWildcards reports whether s contains `*` or `?`.
func HasWildcards(s string) bool { return strings.ContainsAny(s, "*?") }

// Parse reads a wildcard query. Invalid UTF-8 reads as U+FFFD, as everywhere
// a Go string is ranged over.
func Parse(s string) (Pattern, error) {
	toks := []rune(s)
	for i, r := range toks {
		switch r {
		case '*':
			toks[i] = anyRun
		case '?':
			toks[i] = anyOne
		}
	}
	toks = collapse(toks)
	if len(toks) > MaxLen {
		return Pattern{}, ErrTooLong
	}
	return newPattern(toks), nil
}

// Literal is the pattern of an exact query, s as typed, or of a prefix
// query, s followed by `*`: every character of s, `*` and `?` included, is
// itself.
func Literal(s string, prefix bool) Pattern {
	toks := []rune(s)
	if prefix {
		toks = append(toks, anyRun)
	}
	return newPattern(toks)
}

func newPattern(toks []rune) Pattern {
	p := Pattern{toks: toks}
	for _, r := range toks {
		if r < 0 {
			continue
		}
		if unicode.IsUpper(r) || unicode.IsTitle(r) {
			p.capital = true
		}
		if Fold(r) != unicode.ToLower(r) {
			p.accent = true
		}
	}
	return p
}

// collapse rewrites every run of `*` as one `*`, in place.
func collapse(rs []rune) []rune {
	out := rs[:0]
	for _, r := range rs {
		if r == anyRun && len(out) > 0 && out[len(out)-1] == anyRun {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Tiers returns the readings in the order their matches rank. Whatever the
// user typed beyond the folded form is a preference: a capital letter puts
// the as-typed reading first, an accent puts the case-ignored reading (which
// keeps accents) before the folded one. Case relaxes before accents. Without
// either, every match ranks alike.
func (p Pattern) Tiers() []Reading {
	switch {
	case p.capital && p.accent:
		return []Reading{AsTyped, CaseIgnored, Folded}
	case p.capital:
		return []Reading{AsTyped, Folded}
	case p.accent:
		return []Reading{CaseIgnored, Folded}
	}
	return []Reading{Folded}
}

// SpelledTiers is Tiers with the as-typed reading first in any case: among
// the headwords that spell one word, the one spelled as typed leads, a
// lowercase query included.
func (p Pattern) SpelledTiers() []Reading {
	t := p.Tiers()
	if t[0] != AsTyped {
		t = append([]Reading{AsTyped}, t...)
	}
	return t
}

// Literal reports that the pattern has no wildcard: it spells one word.
func (p Pattern) Literal() bool {
	return !slices.ContainsFunc(p.toks, func(r rune) bool { return r == anyRun || r == anyOne })
}

// Spelled returns the word the pattern spells when every `*` matches
// nothing (`nacion*` → nacion, `un*able` → unable), and false when it has a
// `?`, has no `*`, or spells nothing.
func (p Pattern) Spelled() (Pattern, bool) {
	if !slices.Contains(p.toks, anyRun) || slices.Contains(p.toks, anyOne) {
		return Pattern{}, false
	}
	toks := slices.DeleteFunc(slices.Clone(p.toks), func(r rune) bool { return r == anyRun })
	if len(toks) == 0 {
		return Pattern{}, false
	}
	return Pattern{toks: toks, capital: p.capital, accent: p.accent}, true
}

// Head returns what every as-typed match starts with: the pattern before its
// first wildcard, as typed.
func (p Pattern) Head() string {
	i := slices.IndexFunc(p.toks, func(r rune) bool { return r == anyRun || r == anyOne })
	if i < 0 {
		i = len(p.toks)
	}
	return string(p.toks[:i])
}

// Run returns what the folded form of every match starts with, in any
// reading - the pattern before its first wildcard, folded - and whether the
// pattern ends there. Every reading's match has it: Fold(c) == Fold(ToLower(c))
// for every character, so two characters equal as typed or lowercased are
// equal folded.
func (p Pattern) Run() (run []rune, end bool) {
	for _, r := range p.toks {
		if r == anyRun || r == anyOne {
			return run, false
		}
		if f := Fold(r); f >= 0 {
			run = append(run, f)
		}
	}
	return run, true
}

// Tail returns the ASCII character every match's folded form ends with, in
// any reading - the pattern's last character, folded, when it is ASCII and
// no wildcard follows it - and false otherwise. A matching headword then
// ends with that character in either case, or with a non-ASCII one (an
// accented form, a mark Fold drops, a character whose lowercase is ASCII).
func (p Pattern) Tail() (byte, bool) {
	f := p.under(Folded)
	if len(f) == 0 {
		return 0, false
	}
	if l := f[len(f)-1]; l >= 0 && l < utf8.RuneSelf {
		return byte(l), true
	}
	return 0, false
}

// Glob returns the pattern as typed in SQLite GLOB syntax. `*` and `?` are
// GLOB's own; a literal `*`, `?` or `[` is written as a one-character set;
// a `]` outside a set is already literal.
func (p Pattern) Glob() string {
	var b strings.Builder
	for _, r := range p.toks {
		switch r {
		case anyRun:
			b.WriteByte('*')
		case anyOne:
			b.WriteByte('?')
		case '*', '?', '[':
			b.WriteByte('[')
			b.WriteRune(r)
			b.WriteByte(']')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// under returns the pattern under one reading: its characters mapped,
// dropped ones removed, and stars collapsed again, since a dropped mark can
// leave two adjacent (`*́*`).
func (p Pattern) under(r Reading) []rune {
	out := make([]rune, 0, len(p.toks))
	for _, t := range p.toks {
		if t >= 0 {
			if t = r.apply(t); t < 0 {
				continue
			}
		}
		out = append(out, t)
	}
	return collapse(out)
}

// Matcher matches headwords against one pattern in a list of readings. It
// reuses a buffer, so it belongs to one goroutine.
type Matcher struct {
	tiers  []Reading
	pats   [][]rune // the pattern under each reading
	folded int      // the index of Folded in tiers, -1 without it
	anyOne bool     // the pattern has a `?`
	buf    []rune   // the headword under one reading
}

// Matcher returns a matcher for p in the readings of Tiers.
func (p Pattern) Matcher() *Matcher { return p.MatcherFor(p.Tiers()) }

// MatcherFor returns a matcher for p in the given readings, in order.
func (p Pattern) MatcherFor(tiers []Reading) *Matcher {
	m := &Matcher{tiers: tiers, folded: slices.Index(tiers, Folded),
		anyOne: slices.Contains(p.toks, anyOne), buf: make([]rune, 0, 64)}
	for _, r := range tiers {
		m.pats = append(m.pats, p.under(r))
	}
	return m
}

// Match reports whether the headword w fits the pattern in any reading.
func (m *Matcher) Match(w []byte) bool { return m.Tier(w) >= 0 }

// Tier returns the index of the first reading in which the headword w fits
// the pattern, or -1.
//
// The folded reading is tried first, because a headword that does not fit
// it fits no other: two characters equal as typed or lowercased are equal
// folded (Pattern.Run), and a `*` takes what Fold drops as readily as what it
// keeps. Only a `?` tells them apart, by taking one character Fold drops - a
// stored mark. So most headwords, which match nothing, cost one reading.
func (m *Matcher) Tier(w []byte) int {
	folded, dropped := false, false
	if m.folded >= 0 {
		dropped = m.read(w, Folded)
		if folded = match(m.pats[m.folded], m.buf); !folded && !(dropped && m.anyOne) {
			return -1
		}
	}
	for i, rd := range m.tiers {
		if i == m.folded {
			if folded {
				return i
			}
			continue
		}
		m.read(w, rd)
		if match(m.pats[i], m.buf) {
			return i
		}
	}
	return -1
}

// read decodes w into buf under one reading, and reports whether it dropped
// a character.
func (m *Matcher) read(w []byte, rd Reading) (dropped bool) {
	m.buf = m.buf[:0]
	for len(w) > 0 {
		r, n := utf8.DecodeRune(w)
		w = w[n:]
		if r = rd.apply(r); r < 0 {
			dropped = true
			continue
		}
		m.buf = append(m.buf, r)
	}
	return dropped
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
		case pi < len(p) && p[pi] == anyRun:
			star, mark = pi, si
			pi++
		case pi < len(p) && (p[pi] == anyOne || p[pi] == s[si]):
			pi++
			si++
		case star >= 0:
			mark++
			pi, si = star+1, mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == anyRun {
		pi++
	}
	return pi == len(p)
}

// Fold maps one character to the form the folded reading compares, or -1
// when the character is dropped:
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
	foldBMP  *[0x10000]rune // 256 kB, built on the first search through this package
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
