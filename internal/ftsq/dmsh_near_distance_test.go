// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package ftsq

import (
	"strings"
	"testing"
)

// The near window is a reader SETTING now, so two things have to hold at once:
// the number inside the expression is the resolved setting, and a value nobody
// chose - zero, or one a hand-edited state file pushed past the ceiling - cannot
// become a window of its own. What this replaces is one constant copied into a
// mark, a hint and an archive reader, kept in step by a test.
func TestNearDistanceIsTheResolvedSetting(t *testing.T) {
	cases := []struct {
		setting, want int
	}{
		{0, NearDefault},       // nobody chose one
		{-3, NearDefault},      // below the floor is the same as unset
		{NearMin, NearMin},     // the floor: one word may stand between
		{7, 7},                 // a real choice
		{NearMax, NearMax},     // the ceiling itself
		{NearMax + 1, NearMax}, // above it clamps, never grows
		{9999, NearMax},
	}
	for _, tc := range cases {
		if got := ResolveNear(tc.setting); got != tc.want {
			t.Errorf("ResolveNear(%d) = %d, want %d", tc.setting, got, tc.want)
		}
	}
}

func TestRungsWithCarriesTheChosenWindow(t *testing.T) {
	query := Parse("Физика в конспектах")
	if got := rungNamed(t, query.Rungs(), "near"); !strings.HasSuffix(got, ", 10)") {
		t.Errorf("the rung without a setting is %q; it must run at NearDefault", got)
	}
	if got := rungNamed(t, query.RungsWith(25), "near"); !strings.HasSuffix(got, ", 25)") {
		t.Errorf("RungsWith(25) built %q", got)
	}
	if got := rungNamed(t, query.RungsWith(0), "near"); !strings.HasSuffix(got, ", 10)") {
		t.Errorf("RungsWith(0) built %q; unset is the default, not a zero-wide window", got)
	}
	if got := rungNamed(t, query.RungsWith(NearMax+10), "near"); !strings.HasSuffix(got, ", 50)") {
		t.Errorf("RungsWith(NearMax+10) built %q; the ceiling holds here too", got)
	}
	// An explicit operator keeps the distance the reader typed: the setting is
	// the reading's window, not a rewrite of what was asked for.
	if got := rungNamed(t, Parse("NEAR(собака кошка, 3)").RungsWith(25), "query"); !strings.Contains(got, ", 3)") {
		t.Errorf("an explicit NEAR(…, 3) was rewritten to %q", got)
	}
}

func rungNamed(t *testing.T, rungs []Rung, name string) string {
	t.Helper()
	for _, r := range rungs {
		if r.Name == name {
			return r.Match
		}
	}
	t.Fatalf("no %q rung among %d", name, len(rungs))
	return ""
}
