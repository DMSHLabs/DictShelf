// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wildcard

import (
	"errors"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// show renders tokens with the wildcards as `*` and `?`.
func show(toks []rune) string {
	var b strings.Builder
	for _, r := range toks {
		switch r {
		case anyRun:
			b.WriteByte('*')
		case anyOne:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		in      string
		tiers   []Reading
		folded  string
		glob    string
		run     string
		end     bool
		head    string
		literal bool
	}{
		{"apple", []Reading{Folded}, "apple", "apple", "apple", true, "apple", true},
		{"a**b***", []Reading{Folded}, "a*b*", "a*b*", "a", false, "a", false},
		{"Café", []Reading{AsTyped, CaseIgnored, Folded}, "cafe", "Café", "cafe", true, "Café", true},
		{"café", []Reading{CaseIgnored, Folded}, "cafe", "café", "cafe", true, "café", true},
		{"March", []Reading{AsTyped, Folded}, "march", "March", "march", true, "March", true},
		{"CAFÉ*", []Reading{AsTyped, CaseIgnored, Folded}, "cafe*", "CAFÉ*", "cafe", false, "CAFÉ", false},
		{"ǅemal", []Reading{AsTyped, Folded}, "ǆemal", "ǅemal", "ǆemal", true, "ǅemal", true}, // titlecase counts as a capital
		{"Жук", []Reading{AsTyped, Folded}, "жук", "Жук", "жук", true, "Жук", true},           // so does a non-Latin capital
		{"жук", []Reading{Folded}, "жук", "жук", "жук", true, "жук", true},
		{"йод", []Reading{CaseIgnored, Folded}, "иод", "йод", "иод", true, "йод", true}, // й decomposes to и + a mark
		{"한국?", []Reading{Folded}, "한국?", "한국?", "한국", false, "한국", false},              // a Hangul syllable stays whole
		{"*́*x", []Reading{CaseIgnored, Folded}, "*x", "*́*x", "", false, "", false},    // a dropped mark joins two stars
		{"[a]", []Reading{Folded}, "[a]", "[[]a]", "[a]", true, "[a]", true},
		{"Ab[", []Reading{AsTyped, Folded}, "ab[", "Ab[[]", "ab[", true, "Ab[", true},
		{"1990s", []Reading{Folded}, "1990s", "1990s", "1990s", true, "1990s", true},
		{"İst", []Reading{AsTyped, Folded}, "ist", "İst", "ist", true, "İst", true}, // İ lowercases to i: a capital, no accent
	} {
		p, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.in, err)
		}
		if got := p.Tiers(); !slices.Equal(got, tc.tiers) {
			t.Errorf("Parse(%q).Tiers() = %v, want %v", tc.in, got, tc.tiers)
		}
		if got := show(p.under(Folded)); got != tc.folded {
			t.Errorf("Parse(%q) folded = %q, want %q", tc.in, got, tc.folded)
		}
		if g := p.Glob(); g != tc.glob {
			t.Errorf("Parse(%q).Glob() = %q, want %q", tc.in, g, tc.glob)
		}
		if run, end := p.Run(); string(run) != tc.run || end != tc.end {
			t.Errorf("Parse(%q).Run() = %q, %v; want %q, %v", tc.in, string(run), end, tc.run, tc.end)
		}
		if h := p.Head(); h != tc.head {
			t.Errorf("Parse(%q).Head() = %q, want %q", tc.in, h, tc.head)
		}
		if l := p.Literal(); l != tc.literal {
			t.Errorf("Parse(%q).Literal() = %v, want %v", tc.in, l, tc.literal)
		}
	}
}

// Tail: the ASCII character every match ends with, folded, in any reading.
func TestTail(t *testing.T) {
	for in, want := range map[string]int{
		"*tion": 'n', "*ción": 'n', "*CIÓN": 'n', "*cafÉ": 'e', "c?t": 't', "a\u0301": 'a',
		"*ción*": -1, "*n?": -1, "*жук": -1, "*\u0301": -1, "*": -1,
	} {
		p, _ := Parse(in)
		c, ok := p.Tail()
		if got := map[bool]int{true: int(c), false: -1}[ok]; got != want {
			t.Errorf("Parse(%q).Tail() = %d, want %d", in, got, want)
		}
	}
}

// Exact and prefix queries are literal: `*`, `?` and `[` are themselves.
func TestLiteral(t *testing.T) {
	for _, tc := range []struct {
		in     string
		prefix bool
		glob   string
		yes    []string
		no     []string
	}{
		{"what?", false, "what[?]", []string{"what?", "What?"}, []string{"whatx", "what"}},
		{"a*b", true, "a[*]b*", []string{"a*b", "a*bc"}, []string{"axb", "ab"}},
		{"[x", true, "[[]x*", []string{"[x", "[xy"}, []string{"x"}},
		{"100%", true, "100%*", []string{"100% sure"}, []string{"100 sure"}},
		{"corazon", true, "corazon*", []string{"corazón", "Corazonada"}, []string{"coraz"}},
	} {
		p := Literal(tc.in, tc.prefix)
		if g := p.Glob(); g != tc.glob {
			t.Errorf("Literal(%q, %v).Glob() = %q, want %q", tc.in, tc.prefix, g, tc.glob)
		}
		if p.Literal() == tc.prefix {
			t.Errorf("Literal(%q, %v).Literal() = %v", tc.in, tc.prefix, p.Literal())
		}
		m := p.Matcher()
		for _, w := range tc.yes {
			if !m.Match([]byte(w)) {
				t.Errorf("Literal(%q, %v) does not match %q", tc.in, tc.prefix, w)
			}
		}
		for _, w := range tc.no {
			if m.Match([]byte(w)) {
				t.Errorf("Literal(%q, %v) matches %q", tc.in, tc.prefix, w)
			}
		}
	}
}

// The word a pattern spells: every star empty, only without `?`.
func TestSpelled(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"nacion*", "nacion", true},
		{"un*able", "unable", true},
		{"*tion", "tion", true},
		{"Nación*", "Nación", true},
		{"c?t*", "", false},
		{"*", "", false},
		{"apple", "", false}, // no star: the pattern is the word already
	} {
		p, _ := Parse(tc.in)
		sp, ok := p.Spelled()
		if ok != tc.ok || (ok && show(sp.toks) != tc.want) {
			t.Errorf("Parse(%q).Spelled() = %q, %v; want %q, %v", tc.in, show(sp.toks), ok, tc.want, tc.ok)
		}
		if ok && (sp.capital != p.capital || sp.accent != p.accent) {
			t.Errorf("Parse(%q).Spelled() changed the flags", tc.in)
		}
	}
	p := Literal("nacion", true)
	if sp, ok := p.Spelled(); !ok || show(sp.toks) != "nacion" {
		t.Errorf("a prefix query spells its word: %q, %v", show(sp.toks), ok)
	}
	for in, want := range map[string][]Reading{
		"march": {AsTyped, Folded},
		"café":  {AsTyped, CaseIgnored, Folded},
		"March": {AsTyped, Folded},
		"Café":  {AsTyped, CaseIgnored, Folded},
	} {
		if got := Literal(in, false).SpelledTiers(); !slices.Equal(got, want) {
			t.Errorf("Literal(%q).SpelledTiers() = %v, want %v", in, got, want)
		}
	}
}

// Tier is the first reading a headword fits in.
func TestTier(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		tiers   map[string]int // headword → tier; -1 no match
	}{
		{"March", map[string]int{"March": 0, "march": 1, "MARCH": 1, "Märch": 1, "Marchx": -1}},
		{"Café*", map[string]int{"Café": 0, "Cafés": 0, "café": 1, "CAFÉ": 1, "cafe": 2, "Cafe": 2, "kafe": -1}},
		{"café", map[string]int{"café": 0, "Café": 0, "CAFÉ": 0, "cafe": 1, "Cafe": 1}},
		{"cafe", map[string]int{"cafe": 0, "Café": 0, "café": 0}},
		{"бой", map[string]int{"бой": 0, "Бой": 0, "бои": 1}},
		// `?` is one character as typed: it takes a stored combining mark,
		// which the folded reading drops
		{"A?", map[string]int{"A\u0301": 0, "Ab": 0, "ab": 1, "a\u0301": -1, "\u00c1": -1, "A": -1}},
	} {
		p, err := Parse(tc.pattern)
		if err != nil {
			t.Fatal(err)
		}
		m := p.Matcher()
		for w, want := range tc.tiers {
			if got := m.Tier([]byte(w)); got != want {
				t.Errorf("%q: Tier(%q) = %d, want %d", tc.pattern, w, got, want)
			}
		}
	}
}

func TestParseLength(t *testing.T) {
	if _, err := Parse(strings.Repeat("ж", MaxLen)); err != nil {
		t.Errorf("%d characters: %v", MaxLen, err)
	}
	if _, err := Parse(strings.Repeat("ж", MaxLen+1)); !errors.Is(err, ErrTooLong) {
		t.Errorf("%d characters: err = %v, want ErrTooLong", MaxLen+1, err)
	}
	// Collapsed stars are not counted: the limit is on what is matched.
	if _, err := Parse("a" + strings.Repeat("*", 500) + "b"); err != nil {
		t.Errorf("a + 500 stars + b: %v", err)
	}
}

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		yes, no []string
	}{
		{"c?t", []string{"cat", "cot", "Cut", "çat"}, []string{"ct", "cats", "coat"}},
		{"apple", []string{"apple", "Apple", "Äpple", "APPLE"}, []string{"apples", "pineapple", "appl"}},
		{"apple*", []string{"apple", "apples", "apple pie"}, []string{"pineapple"}},
		{"*apple", []string{"apple", "pineapple"}, []string{"apples"}},
		{"*apple*", []string{"apple", "apples", "pineapple", "apple pie"}, []string{"appl"}},
		{"un*able", []string{"unable", "unbelievable"}, []string{"unabled", "able"}},
		{"b???d", []string{"beard", "blood", "bread"}, []string{"bed", "boards"}},
		{"by the *", []string{"by the way", "by the "}, []string{"by the"}},
		{"* *", []string{"a b", " ", "ice cream"}, []string{"icecream"}},
		{"*", []string{"", "anything"}, nil},
		{"?", []string{"a", "é", "한"}, []string{"", "ab"}},
		{"*a*a*a*a*a*a*b", []string{"aaaaaab"}, []string{strings.Repeat("a", 60)}},
		{"koln", []string{"Köln", "KÖLN", "Köln"}, []string{"Koeln"}},
		{"cafe", []string{"café", "Café", "CAFÉ", "café"}, []string{"cafes"}},
		{"иод", []string{"йод", "Йод"}, nil},
		{"한?", []string{"한국"}, []string{"한"}},
		// A capital or an accent ranks; it never excludes (TestTier).
		{"March", []string{"March", "march", "MARCH"}, []string{"Marc"}},
		{"Cafe", []string{"Cafe", "Café", "cafe"}, []string{"Caf"}},
		{"Café", []string{"Café", "café", "Cafe"}, []string{"Cafés"}},
		{"Ж?к", []string{"Жук", "жук"}, []string{"Жу"}},
		// `[` and `]` are ordinary characters.
		{"[a]*", []string{"[a]", "[a]b"}, []string{"a"}},
		{"*\\*", []string{`a\b`}, []string{"ab"}},
	} {
		p, err := Parse(tc.pattern)
		if err != nil {
			t.Fatal(err)
		}
		m := p.Matcher()
		for _, w := range tc.yes {
			if !m.Match([]byte(w)) {
				t.Errorf("%q does not match %q", tc.pattern, w)
			}
		}
		for _, w := range tc.no {
			if m.Match([]byte(w)) {
				t.Errorf("%q matches %q", tc.pattern, w)
			}
		}
	}
}

// reference is the textbook recursive reading of `?` and `*`, too slow to
// use and too short to get wrong.
func reference(p, s []rune) bool {
	if len(p) == 0 {
		return len(s) == 0
	}
	switch p[0] {
	case '*':
		for i := 0; i <= len(s); i++ {
			if reference(p[1:], s[i:]) {
				return true
			}
		}
		return false
	case '?':
		return len(s) > 0 && reference(p[1:], s[1:])
	}
	return len(s) > 0 && p[0] == s[0] && reference(p[1:], s[1:])
}

// TestMatchAgainstReference holds the backtracking matcher to the recursive
// definition over random patterns and strings from a small alphabet, where
// the backtracking cases are dense.
func TestMatchAgainstReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alpha := []rune("ab*?")
	text := []rune("abc")
	for range 50000 {
		p := make([]rune, rng.Intn(7))
		for j := range p {
			p[j] = alpha[rng.Intn(len(alpha))]
		}
		s := make([]rune, rng.Intn(9))
		for j := range s {
			s[j] = text[rng.Intn(len(text))]
		}
		toks := []rune(string(p))
		for j, r := range toks {
			switch r {
			case '*':
				toks[j] = anyRun
			case '?':
				toks[j] = anyOne
			}
		}
		if got, want := match(collapse(toks), s), reference(p, s); got != want {
			t.Fatalf("match(%q, %q) = %v, want %v", string(p), string(s), got, want)
		}
	}
}

// TestFold pins the definition on the characters it is easy to get wrong.
func TestFold(t *testing.T) {
	for _, tc := range []struct {
		in, want rune
	}{
		{'A', 'a'}, {'a', 'a'}, {'É', 'e'}, {'ç', 'c'}, {'Ä', 'a'}, {'Й', 'и'},
		{'İ', 'i'}, {'K', 'k'}, // Kelvin sign
		{'ſ', 'ſ'},           // long s has no lowercase mapping and no decomposition
		{'đ', 'đ'},           // a stroke is not a mark
		{'한', '한'},           // Hangul syllable: decomposes to jamo, which are not marks
		{'́', -1}, {'̈', -1}, // combining marks are dropped
		{'*', '*'}, {'?', '?'}, {'[', '['},
		{0x1D400, 0x1D400}, // MATHEMATICAL BOLD CAPITAL A: no lowercase, compatibility only
		{0x10400, 0x10428}, // DESERET CAPITAL LONG I, outside the table
	} {
		if got := Fold(tc.in); got != tc.want {
			t.Errorf("Fold(%U) = %U, want %U", tc.in, got, tc.want)
		}
	}
}

// TestReadingsNest pins what lets one seek serve all three readings
// (Pattern.Run): two characters equal as typed or lowercased are equal
// folded, because Fold(c) == Fold(ToLower(c)); an ASCII character folds to
// its lowercase, so NOCASE ranges hold its folds; Fold and ToLower are
// idempotent; a mark lowercases to itself.
func TestReadingsNest(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		l, f := unicode.ToLower(r), Fold(r)
		switch {
		case Fold(l) != f:
			t.Fatalf("Fold(%U) = %U, Fold(ToLower) = %U", r, f, Fold(l))
		case f >= 0 && Fold(f) != f:
			t.Fatalf("Fold(Fold(%U)) = %U, not %U", r, Fold(f), f)
		case unicode.ToLower(l) != l:
			t.Fatalf("ToLower is not idempotent at %U", r)
		case unicode.Is(unicode.Mn, r) && l != r:
			t.Fatalf("mark %U lowercases to %U", r, l)
		case r < utf8.RuneSelf && f != l:
			t.Fatalf("ASCII %q folds to %U", r, f)
		}
	}
}

// Unmarked spans hold no nonspacing mark, checked against every code point.
func TestUnmarked(t *testing.T) {
	marks := make([]int32, unicode.MaxRune+2) // marks[r] = number of Mn below r
	for r := rune(0); r <= unicode.MaxRune; r++ {
		marks[r+1] = marks[r]
		if unicode.Is(unicode.Mn, r) {
			marks[r+1]++
		}
	}
	for r := rune(utf8.RuneSelf); r <= unicode.MaxRune; r++ {
		lo, hi := Unmarked(r)
		if lo > r || hi < r || lo < utf8.RuneSelf {
			t.Fatalf("Unmarked(%U) = %U..%U", r, lo, hi)
		}
		if unicode.Is(unicode.Mn, r) {
			if lo != r || hi != r {
				t.Fatalf("Unmarked of the mark %U = %U..%U", r, lo, hi)
			}
			continue
		}
		if n := marks[hi+1] - marks[lo]; n != 0 {
			t.Fatalf("Unmarked(%U) = %U..%U holds %d marks", r, lo, hi, n)
		}
	}
	if lo, hi := Unmarked('е'); lo > 'Ѐ' || hi < 'џ' {
		t.Errorf("Unmarked(е) = %U..%U: the basic Cyrillic letters are not one span", lo, hi)
	}
}

// TestFoldTable checks the table against the definition for every BMP
// character, and that the definition's output stays in the BMP for it.
func TestFoldTable(t *testing.T) {
	for r := range rune(0x10000) {
		if got, want := Fold(r), foldRune(r); got != want {
			t.Fatalf("Fold(%U) = %U, foldRune = %U", r, got, want)
		}
	}
}

// TestFoldAgreesWithStringFold: for text made of base letters and marks,
// folding character by character equals the whole-string reading the
// definition describes (lowercase, NFD, drop the marks).
func TestFoldAgreesWithStringFold(t *testing.T) {
	for _, s := range []string{"Café", "Ångström", "Ïñtërnâtiônàlizætiøn", "ЁЛКА", "Ο Πλάτων", "Köln", "Việt Nam"} {
		var per strings.Builder
		for _, r := range s {
			if f := Fold(r); f >= 0 {
				per.WriteRune(f)
			}
		}
		var whole strings.Builder
		for _, r := range norm.NFD.String(strings.ToLower(s)) {
			if !unicode.Is(unicode.Mn, r) {
				whole.WriteRune(r)
			}
		}
		if per.String() != whole.String() {
			t.Errorf("%q: per character %q, whole string %q", s, per.String(), whole.String())
		}
	}
}

func TestMatchInvalidUTF8(t *testing.T) {
	p, _ := Parse("ab?")
	m := p.Matcher()
	if !m.Match([]byte("ab\xff")) {
		t.Error(`"ab?" does not match "ab\xff": one invalid byte is one character`)
	}
	if !utf8.ValidString(p.Glob()) {
		t.Error("parsed pattern is not valid UTF-8")
	}
}

func BenchmarkMatch(b *testing.B) {
	p, _ := Parse("*tion*")
	m := p.Matcher()
	w := []byte("internationalization")
	for b.Loop() {
		m.Match(w)
	}
}
