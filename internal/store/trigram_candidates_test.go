// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// The candidate filter an index read builds from a substring query is a SUPERSET
// of the answer, and that is the only thing it is allowed to be: the matcher
// decides. One trigram was such a filter; several trigrams of the same term ANDed
// still is, because every trigram of a term occurs in whatever text holds the
// term. These tests hold the two ends of that: the expression the filter builds,
// and the answers of every path that uses it, against a scan with the same
// matcher - the index must never lose a headword the matcher accepts, and must
// never add one it rejects.

func TestTrigramCandidatesAsksForWindowsOfTheTerm(t *testing.T) {
	for _, tc := range []struct {
		folded, want string
	}{
		// Shorter than one window: no candidate at all, the caller scans.
		{"ab", ""},
		{"", ""},
		// Exactly one window: the single trigram this filter always used.
		{"hom", `"hom"`},
		{"home", `"hom" AND "ome"`},
		// First, middle and last: the ends are what a query of many runes has
		// that a three-letter one does not.
		{"revolution", `"rev" AND "olu" AND "ion"`},
		{"приветствие", `"при" AND "етс" AND "вие"`},
		// A window keeps a separator, as the trigram tokenizer does: "w y" is a
		// token of "new york", and a query for one is legal on detail=none.
		{"new york", `"new" AND "w y" AND "ork"`},
	} {
		if got := trigramCandidates(tc.folded); got != tc.want {
			t.Errorf("trigramCandidates(%q) = %q, want %q", tc.folded, got, tc.want)
		}
	}
}

// The fixture every answer test below is scanned from: the families that made
// the filter's behaviour visible (a long term sharing its first trigram with
// others, a term of exactly three runes, a separator, accents, Cyrillic, a
// two-rune word with no trigram at all, and a redirect stub no read may list).
func trigramFixture(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.dsl")
	if err := os.WriteFile(src, []byte("#NAME test\n\nword\n\tbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	words := []string{"home", "homeland", "homily", "homeward", "home town", "New York",
		"revolution", "revolutionary", "counterrevolution", "reverse", "revue", "evolution",
		"café", "École", "привет", "приветствие", "революция", "ab", "@_stub"}
	entries := make([]dict.Entry, 0, len(words))
	for _, w := range words {
		entries = append(entries, h(w, "<p>body</p>"))
	}
	r := &fakeReader{meta: dict.Meta{Name: "Trigram", Format: "dsl", Path: src}, entries: entries}
	textDB := filepath.Join(dir, TextDBName)
	if _, err := IngestPlan(r, textDB, Plan{Contains: true}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if !s.hasTrigram {
		t.Fatal("the fixture has no trigram index, so these tests would prove nothing")
	}
	return s
}

// scanHeadwords answers a query the slow, index-free way: every headword, the
// path's own matcher, and the '@_' stubs the SQL excludes on every read.
func scanHeadwords(t *testing.T, s *Store, keep func(string) bool) []string {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(), `SELECT w FROM entry WHERE w NOT LIKE '@_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			t.Fatal(err)
		}
		if keep(w) {
			out = append(out, w)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(words []string) []string {
	out := append([]string(nil), words...)
	sort.Strings(out)
	return out
}

func equalWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Every path that builds a substring candidate must answer exactly what a scan
// with its own matcher answers - for a three-rune query, for a long one, for one
// shorter than a trigram, and across scripts.
//
// Two matchers, because the paths differ: a search composes its reading with the
// case matcher headwordSearchMatcher (so Browse lists "café" for `cafe` and the
// search paths do not), and Browse reads only the mode's own matcher. Each path
// is compared with the matcher IT applies, which is what keeps this a test of the
// candidate filter and not of that difference.
func TestSubstringPathsAnswerLikeAScanWithTheSameMatcher(t *testing.T) {
	s := trigramFixture(t)
	ctx := context.Background()
	base := map[string]func(string) func(string) bool{
		"contains": dict.HeadwordContainsMatcher,
		"suffix":   dict.HeadwordSuffixMatcher,
	}
	paths := []struct {
		name     string
		caseful  bool
		headword func(query, mode string) ([]string, error)
	}{
		{"complete list", true, func(query, mode string) ([]string, error) {
			var ids []int64
			if err := s.StreamArticleIDs(ctx, query, mode, "", func(page []int64, _ int, done bool) error {
				ids = append(ids, page...)
				return nil
			}); err != nil {
				return nil, err
			}
			results, err := s.ArticlesByIDs(ctx, ids)
			if err != nil {
				return nil, err
			}
			out := make([]string, 0, len(results))
			for _, result := range results {
				out = append(out, result.Headword)
			}
			return out, nil
		}},
		{"search headwords", true, func(query, mode string) ([]string, error) {
			results, err := s.SearchHeadwordsMode(ctx, query, mode, 500)
			if err != nil {
				return nil, err
			}
			out := make([]string, 0, len(results))
			for _, result := range results {
				out = append(out, result.Headword)
			}
			return out, nil
		}},
		{"browse headwords", false, func(query, mode string) ([]string, error) {
			words, _, _, err := s.FindHeadwordsMode(ctx, query, mode, 0, 500)
			return words, err
		}},
	}
	for _, tc := range []struct{ mode, query string }{
		{"contains", "hom"}, {"contains", "home"}, {"contains", "home town"}, {"contains", "me t"},
		{"contains", "revolution"}, {"contains", "volution"}, {"contains", "ion"}, {"contains", "re"},
		{"contains", "r"}, {"contains", "café"}, {"contains", "cafe"}, {"contains", "ÉCOLE"},
		{"contains", "привет"}, {"contains", "революция"}, {"contains", "люци"}, {"contains", "new york"},
		{"contains", "york"}, {"contains", "ab"}, {"contains", "xyz"},
		{"suffix", "home"}, {"suffix", "land"}, {"suffix", "olution"}, {"suffix", "привет"},
		{"suffix", "york"}, {"suffix", "tion"}, {"suffix", "e"},
	} {
		terms := base[tc.mode](tc.query)
		for _, path := range paths {
			keep := func(w string) bool { return terms(w) }
			if path.caseful {
				caseMatch := headwordSearchMatcher(ctx, tc.query, tc.mode)
				keep = func(w string) bool { return terms(w) && caseMatch(w) }
			}
			want := scanHeadwords(t, s, keep)
			got, err := path.headword(tc.query, tc.mode)
			if err != nil {
				t.Fatalf("%s %q %s: %v", tc.mode, tc.query, path.name, err)
			}
			if !equalWords(sortedCopy(got), want) {
				t.Errorf("%s %q %s = %v, want %v", tc.mode, tc.query, path.name, sortedCopy(got), want)
			}
		}
	}
}

// The wildcard reads build their candidate from the longest literal run of the
// pattern, and verify with the pattern's own regular expression.
func TestMaskPathsAnswerLikeThePatternScan(t *testing.T) {
	s := trigramFixture(t)
	ctx := context.Background()
	for _, pattern := range []string{"*revolution*", "*olution", "home*ard", "rev*ion", "*привет*", "*люци*", "caf?", "ab*"} {
		re, err := regexp.Compile("(?i)^" + wildcardRegexp(pattern) + "$")
		if err != nil {
			t.Fatal(err)
		}
		want := scanHeadwords(t, s, re.MatchString)
		words, _, _, err := s.FindHeadwordsMask(ctx, pattern, 0, 500)
		if err != nil {
			t.Fatalf("%q: %v", pattern, err)
		}
		if !equalWords(sortedCopy(words), want) {
			t.Errorf("%q: mask list = %v, want %v", pattern, sortedCopy(words), want)
		}
	}
}
