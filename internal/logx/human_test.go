// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package logx

import "testing"

// The same rule as the pages' mb() (index.html, lemmas.html, setup.html).
func TestSize(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0 B"}, {999, "999 B"}, {1_000, "1 kB"}, {1_499, "1 kB"}, {57_300, "57 kB"},
		{1_000_000, "1.0 MB"}, {1_450_000, "1.4 MB"}, {9_990_000, "10.0 MB"},
		{10_000_000, "10 MB"}, {57_300_000, "57 MB"}, {1_000_000_000, "1.0 GB"}, {2_550_000_000, "2.5 GB"},
	} {
		if got := Size(tc.n); got != tc.want {
			t.Errorf("Size(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
	if Plural(1, "entry", "entries") != "1 entry" || Plural(0, "entry", "entries") != "0 entries" {
		t.Error("Plural")
	}
}
