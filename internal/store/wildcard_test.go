// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/wildcard"
)

// wildcardWords holds what the first-character ranges can lose: accented and
// non-Latin initials (which NOCASE sorts after every ASCII headword), a
// leading combining mark, NFD-stored text, Hangul, characters outside the
// BMP, invalid UTF-8, case twins, GLOB's own `[`, and an `@` sub-entry that
// no search may return.
var wildcardWords = []string{
	"apple", "Apple", "APPLE", "apples", "pineapple", "apple pie", "Äpple",
	"cat", "Cat", "cot", "cut", "coat", "çat", "Ćat", "́cat", "ct",
	"café", "café", "Café", "CAFÉ", "cafe", "Köln", "koln",
	"March", "march", "MARCH", "жук", "Жук", "йод", "Йод", "ёлка",
	"한국", "한", "1990s", "[a]", "a]b", "[[x", "x*y", "q?r",
	"@_sub", "@", "ab\xff", "zz", "Zebra", "ǅemal", "džemal",
	"𐐨x", "𐐀x", "ſtar", "star", "by the way", "ice cream", "~tilde", "\x7fdel",
}

func wildcardStore(t *testing.T) *Store {
	t.Helper()
	entries := make([]dict.Entry, len(wildcardWords))
	for i, w := range wildcardWords {
		entries[i] = h(w, "<p>"+w+"</p>")
	}
	r := &fakeReader{meta: dict.Meta{Name: "Wildcard Fixture", Format: "mdx", Path: "/nonexistent/wildcard.mdx"}, entries: entries}
	dbPath := t.TempDir() + "/wildcard.text.db"
	if _, err := IngestPlan(r, dbPath, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func headwords(rs []dict.Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Headword
	}
	return out
}

func TestWildcard(t *testing.T) {
	s := wildcardStore(t)
	if !s.Caps().Wildcard {
		t.Fatal("a headword-only database does not offer wildcard search")
	}
	for _, tc := range []struct {
		pattern string
		want    []string // as a set: TestWildcardAgainstBruteForce checks the order
	}{
		{"c?t", []string{"cat", "Cat", "cot", "cut", "çat", "Ćat", "́cat"}},
		{"apple", []string{"apple", "Apple", "APPLE", "Äpple"}},
		{"*apple", []string{"apple", "Apple", "APPLE", "pineapple", "Äpple"}},
		{"cafe", []string{"cafe", "café", "café", "Café", "CAFÉ"}},
		{"koln", []string{"koln", "Köln"}},
		{"иод", []string{"йод", "Йод"}},
		{"ж*", []string{"жук", "Жук"}},
		{"한?", []string{"한국"}},
		{"* *", []string{"apple pie", "by the way", "ice cream"}},
		{"[*", []string{"[[x", "[a]"}},
		{"x*y", []string{"x*y"}},
		{"@*", []string{"@"}}, // never the `@_` sub-entry
		{"ab?", []string{"ab\xff"}},
		{"~*", []string{"~tilde"}},
		// Case-sensitive: as typed, accents included.
		{"March", []string{"March"}},
		{"C*", []string{"Café", "CAFÉ", "Cat"}},
		{"Café", []string{"Café"}},
		{"Cafe", nil},
		{"Ж?к", []string{"Жук"}},
		{"ǅ*", []string{"ǅemal"}},
		{"[[*", []string{"[[x"}},
	} {
		got, err := s.WildcardContext(bg, tc.pattern, 100)
		if err != nil {
			t.Fatalf("%q: %v", tc.pattern, err)
		}
		hw := headwords(got)
		slices.Sort(hw)
		slices.Sort(tc.want)
		if !slices.Equal(hw, tc.want) {
			t.Errorf("%q = %q, want %q", tc.pattern, hw, tc.want)
		}
	}
}

func TestWildcardBodiesAndLimit(t *testing.T) {
	s := wildcardStore(t)
	got, err := s.WildcardContext(bg, "c?t", 2)
	if err != nil {
		t.Fatal(err)
	}
	if hw := headwords(got); !slices.Equal(hw, []string{"cat", "Cat"}) {
		t.Fatalf("limit 2 = %q, want the first two in browse order", hw)
	}
	if got[1].Body != "<p>Cat</p>" {
		t.Errorf("body of %q = %q", got[1].Headword, got[1].Body)
	}
	if got, err := s.WildcardContext(bg, "  ", 10); err != nil || got != nil {
		t.Errorf("blank pattern = %v, %v", got, err)
	}
	if _, err := s.WildcardContext(bg, strings.Repeat("a", wildcard.MaxLen+1), 10); !errors.Is(err, wildcard.ErrTooLong) {
		t.Errorf("long pattern: err = %v, want ErrTooLong", err)
	}
}

// TestWildcardAgainstBruteForce is the soundness test of the planner and of
// the two matchers. For every pattern, the answer must be exactly the
// headwords that the Go matcher accepts when it reads every headword, in
// browse order:
//
//   - a case-insensitive pattern reads only the first-character ranges, so a
//     range that lost a headword shows up here;
//   - a case-sensitive pattern is answered by SQLite GLOB, so a difference
//     between GLOB and the Go matcher shows up here.
func TestWildcardAgainstBruteForce(t *testing.T) {
	s := wildcardStore(t)
	all, err := s.Page(0, len(wildcardWords))
	if err != nil {
		t.Fatal(err)
	}
	patterns := []string{"*", "?", "??", "c*", "C*", "*t", "*T", "ä*", "Ä*", "é*", "É*", "́*",
		"[a]", "[*]", "a]*", "*]*", "q?r", "x?y", "ſ*", "s*", "𐐨*", "𐐀*", "ǅ*", "dž*", "한*", "й*", "Й*", "ё*", "е*"}
	rng := rand.New(rand.NewSource(7))
	alpha := []rune("acpeltkoCAÄäéÉçжЖ한[]*?ſ𐐀 ")
	for range 3000 {
		p := make([]rune, 1+rng.Intn(5))
		for j := range p {
			p[j] = alpha[rng.Intn(len(alpha))]
		}
		patterns = append(patterns, string(p))
	}
	for _, pat := range patterns {
		if pat = strings.TrimSpace(pat); pat == "" {
			continue
		}
		p, err := wildcard.Parse(pat)
		if err != nil {
			t.Fatal(err)
		}
		m := p.Matcher()
		var want []string
		for _, w := range all {
			if m.Match([]byte(w)) {
				want = append(want, w)
			}
		}
		got, err := s.WildcardContext(bg, pat, maxLimit)
		if err != nil {
			t.Fatalf("%q: %v", pat, err)
		}
		if hw := headwords(got); !slices.Equal(hw, want) {
			t.Fatalf("%q (sensitive %v) = %q, brute force %q", pat, p.Sensitive, hw, want)
		}
	}
}

// TestWildcardRanges pins what the planner reads; TestWildcardAgainstBruteForce
// proves that it loses nothing.
func TestWildcardRanges(t *testing.T) {
	all := keyRange{}
	for _, tc := range []struct {
		pattern string
		want    []keyRange
		marks   bool
	}{
		{"*at", []keyRange{all}, false},
		{"?at", []keyRange{all}, false},
		// c, C and every character that folds to c: ç Ç, Ć ć … č (adjacent,
		// so one range), and Ḉ ḉ
		{"cat", []keyRange{{"c", "d"}, {"Ç", "È"}, {"ç", "è"}, {"Ć", "Ď"}, {"Ḉ", "Ḋ"}}, true},
		{"Cat", []keyRange{{"c", "d"}}, false},             // GLOB narrows the NOCASE range
		{"Écu", []keyRange{{"\u00c9", "\xc3\x8a"}}, false}, // its own UTF-8 prefix
		{"жук", []keyRange{{"Ж", "З"}, {"ж", "з"}, {"Ӂ", "Ӄ"}, {"Ӝ", "Ӟ"}}, true},
		{"Жук", []keyRange{{"\u0416", "\xd0\x97"}}, false},
		{"\ufffd*", []keyRange{{lo: "\x80"}}, false}, // what invalid UTF-8 reads as
	} {
		p, err := wildcard.Parse(tc.pattern)
		if err != nil {
			t.Fatal(err)
		}
		got, marks := wildcardRanges(p)
		if !slices.Equal(got, tc.want) || marks != tc.marks {
			t.Errorf("%q: ranges %q marks %v, want %q %v", tc.pattern, got, marks, tc.want, tc.marks)
		}
	}
}

// spanRanges splits at the UTF-8 length boundaries, inside which the
// encoding keeps code point order.
func TestSpanRanges(t *testing.T) {
	got := spanRanges(0x7FE, 0x10001)
	want := []keyRange{
		{"\u07fe", "\xdf\xc0"},
		{"\u0800", "\xef\xbf\xc0"},
		{"\U00010000", "\xf0\x90\x80\x82"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("spanRanges = %q, want %q", got, want)
	}
}

// TestWildcardTruncatedUTF8 pins the one place the two matchers differ: a
// multi-byte sequence cut short is one character to SQLite and one per byte
// to Go. Only a case-sensitive pattern reaches SQLite, and only a malformed
// headword tells them apart.
func TestWildcardTruncatedUTF8(t *testing.T) {
	entries := []dict.Entry{h("AB\xe2\x82", "<p>x</p>")}
	r := &fakeReader{meta: dict.Meta{Name: "T", Format: "mdx", Path: "/nonexistent/t.mdx"}, entries: entries}
	dbPath := t.TempDir() + "/t.text.db"
	if _, err := IngestPlan(r, dbPath, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, tc := range []struct {
		pattern string
		sqlite  bool
	}{{"AB?", true}, {"AB??", false}} {
		got, err := s.WildcardContext(bg, tc.pattern, 10)
		if err != nil {
			t.Fatal(err)
		}
		if (len(got) == 1) != tc.sqlite {
			t.Errorf("GLOB %q matched %d", tc.pattern, len(got))
		}
		p, _ := wildcard.Parse(tc.pattern)
		if p.Matcher().Match([]byte("AB\xe2\x82")) == tc.sqlite {
			t.Errorf("Go %q agrees with SQLite; the divergence this test pins is gone", tc.pattern)
		}
	}
}

// TestWildcardReadsTheIndexOnly: the scans must stay on idx_entry_w, in its
// order. A plan that reads the entry table reads every article, and a sort
// would read a whole range before the first headword.
func TestWildcardReadsTheIndexOnly(t *testing.T) {
	s := wildcardStore(t)
	plan := func(q string, args ...any) string {
		t.Helper()
		rows, err := s.db.Query("EXPLAIN QUERY PLAN "+q, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		return strings.Join(plan, "; ")
	}
	for _, k := range []keyRange{{}, {lo: "c", hi: "d"}, {lo: "\x80"}} {
		cond, bounds := k.where()
		for _, glob := range []bool{false, true} {
			args := bounds
			if glob {
				args = append([]any{"c*"}, bounds...)
			}
			q := wildcardScan(cond, glob)
			if got := plan(q, args...); !strings.Contains(got, "COVERING INDEX idx_entry_w") || strings.Contains(got, "TEMP B-TREE") {
				t.Errorf("%s\nplan: %s", q, got)
			}
		}
	}
	// The marks: a seek per range, never a scan of the whole index.
	if got := plan(marksQuery()); strings.Contains(got, "SCAN e") {
		t.Errorf("marks query scans a whole table or index\nplan: %s", got)
	}
}

// The pattern is the whole headword (D167): the other spellings an entry
// lists, its redirects and its `@_` keys are not matched - exact and prefix
// search find those.
func TestWildcardHeadwordsOnly(t *testing.T) {
	entries := []dict.Entry{
		{Headwords: []string{"go", "went", "gone"}, Body: "<p>go</p>", Kind: dict.BodyHTML},
		{Headwords: []string{"run", "runs"}, Body: "<p>run</p>", Kind: dict.BodyHTML},
		{Headwords: []string{"Zoo", "a zebra"}, Body: "<p>Zoo</p>", Kind: dict.BodyHTML},
		{Headwords: []string{"zebra", "@_zebra"}, Body: "<p>zebra</p>", Kind: dict.BodyHTML},
		{Headwords: []string{"weg"}, LinkTo: "go"},
		{Headwords: []string{"́wendy"}, LinkTo: "run"}, // an alias led by a combining mark
	}
	r := &fakeReader{meta: dict.Meta{Name: "Aliases", Format: "mdx", Path: "/nonexistent/alias.mdx"}, entries: entries}
	dbPath := t.TempDir() + "/alias.text.db"
	if _, err := IngestPlan(r, dbPath, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// the aliases are there: prefix search reads them
	if got, err := s.PrefixContext(bg, "went", 10); err != nil || len(got) != 1 || got[0].Headword != "go" {
		t.Fatalf("prefix went = %+v, %v", got, err)
	}
	for _, tc := range []struct {
		pattern string
		want    []string // in order
	}{
		{"we*", nil}, // went, weg and ́wendy are aliases
		{"wen?", nil},
		{"*zebra", []string{"zebra"}}, // not by "a zebra", and @_zebra is no key
		{"z*", []string{"zebra", "Zoo"}},
		{"g*", []string{"go"}},
		{"run*", []string{"run"}},
	} {
		got, err := s.WildcardContext(bg, tc.pattern, 100)
		if err != nil {
			t.Fatalf("%q: %v", tc.pattern, err)
		}
		if hw := headwords(got); !slices.Equal(hw, tc.want) {
			t.Errorf("%q = %q, want %q", tc.pattern, hw, tc.want)
		}
	}
}
