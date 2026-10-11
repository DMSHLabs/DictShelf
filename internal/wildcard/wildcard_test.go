// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wildcard

import (
	"errors"
	"math/rand"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		in        string
		sensitive bool
		runes     string
		glob      string
	}{
		{"apple", false, "apple", "apple"},
		{"a**b***", false, "a*b*", "a*b*"},
		{"Café", true, "Café", "Café"},
		{"café", false, "cafe", "café"},
		{"CAFÉ*", true, "CAFÉ*", "CAFÉ*"},
		{"ǅemal", true, "ǅemal", "ǅemal"}, // titlecase counts as a capital
		{"Жук", true, "Жук", "Жук"},       // so does a non-Latin capital
		{"жук", false, "жук", "жук"},
		{"йод", false, "иод", "йод"},  // й decomposes to и + a mark
		{"한국?", false, "한국?", "한국?"},  // a Hangul syllable stays whole
		{"*́*x", false, "*x", "*́*x"}, // a dropped mark joins two stars
		{"[a]", false, "[a]", "[[]a]"},
		{"Ab[", true, "Ab[", "Ab[[]"},
		{"1990s", false, "1990s", "1990s"},
	} {
		p, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.in, err)
		}
		if p.Sensitive != tc.sensitive || string(p.runes) != tc.runes {
			t.Errorf("Parse(%q) = sensitive %v, %q; want %v, %q", tc.in, p.Sensitive, string(p.runes), tc.sensitive, tc.runes)
		}
		if p.Sensitive {
			if g := p.Glob(); g != tc.glob {
				t.Errorf("Parse(%q).Glob() = %q, want %q", tc.in, g, tc.glob)
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

func TestLead(t *testing.T) {
	for _, tc := range []struct {
		in   string
		lead rune
		ok   bool
	}{
		{"cat", 'c', true},
		{"Cat", 'C', true},
		{"écu", 'e', true},
		{"Écu", 'É', true},
		{"́ab", 'a', true},
		{"*ab", 0, false},
		{"?ab", 0, false},
	} {
		p, _ := Parse(tc.in)
		if r, ok := p.Lead(); r != tc.lead || ok != tc.ok {
			t.Errorf("Parse(%q).Lead() = %q, %v; want %q, %v", tc.in, r, ok, tc.lead, tc.ok)
		}
	}
	var zero Pattern
	if _, ok := zero.Lead(); ok {
		t.Error("the empty pattern has a lead")
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
		// Case-sensitive: as typed, accents included.
		{"March", []string{"March"}, []string{"march", "MARCH"}},
		{"Cafe", []string{"Cafe"}, []string{"Café", "cafe"}},
		{"Café", []string{"Café"}, []string{"café", "Cafe"}},
		{"Ж?к", []string{"Жук"}, []string{"жук"}},
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
		if got, want := match(collapse(append([]rune(nil), p...)), s), reference(p, s); got != want {
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

// TestFoldNeverMakesAWildcard is what lets a pattern's `*` and `?` stay
// wildcards after folding: no other character folds to one.
func TestFoldNeverMakesAWildcard(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r == '*' || r == '?' || (r >= 0xD800 && r <= 0xDFFF) {
			continue
		}
		if f := Fold(r); f == '*' || f == '?' {
			t.Fatalf("Fold(%U) = %q", r, f)
		}
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
	if !utf8.ValidString(string(p.runes)) {
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
