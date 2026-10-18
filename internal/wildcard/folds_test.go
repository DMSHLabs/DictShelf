// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wildcard

import (
	"slices"
	"testing"
	"unicode"
)

// Folds is Fold inverted, exactly: checked against every code point.
func TestFoldsInvertsFold(t *testing.T) {
	want := map[rune][]rune{}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		if f := Fold(r); f >= 0 {
			want[f] = append(want[f], r)
		}
	}
	for f, rs := range want {
		if got := Folds(f); !slices.Equal(got, rs) {
			t.Fatalf("Folds(%q) = %q, want %q", f, got, rs)
		}
	}
	for _, tc := range []struct {
		f    rune
		want string
	}{
		{'k', "KkķĶǨǩḰḱḲḳḴḵK"},
		{'и', "ИиЙйЍѝӢӣӤӥ"},
		{'𐐨', "𐐀𐐨"},
	} {
		got := Folds(tc.f)
		w := []rune(tc.want)
		slices.Sort(w)
		if !slices.Equal(got, w) {
			t.Errorf("Folds(%q) = %q, want %q", tc.f, string(got), string(w))
		}
	}
	if got := Folds('́'); len(got) != 0 {
		t.Errorf("Folds of a mark Fold drops = %q, want none", string(got))
	}
}

// Marks covers every nonspacing mark, and joins only over characters no
// headword starts with.
func TestMarks(t *testing.T) {
	ms := Marks()
	in := func(r rune) bool {
		_, ok := slices.BinarySearchFunc(ms, r, func(m [2]rune, r rune) int {
			switch {
			case m[1] < r:
				return -1
			case m[0] > r:
				return 1
			}
			return 0
		})
		return ok
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		mn := unicode.Is(unicode.Mn, r) && r >= 0x80
		if mn && !in(r) {
			t.Fatalf("%U is a nonspacing mark outside Marks", r)
		}
		if !mn && in(r) && unicode.In(r, unicode.L, unicode.N, unicode.P, unicode.S, unicode.Z) {
			t.Fatalf("%U (%q) starts headwords and is inside Marks", r, r)
		}
	}
	for i := 1; i < len(ms); i++ {
		if ms[i][0] <= ms[i-1][1] {
			t.Fatalf("Marks not ascending at %d: %U..%U after %U..%U", i, ms[i][0], ms[i][1], ms[i-1][0], ms[i-1][1])
		}
	}
	t.Logf("%d ranges", len(ms))
}
