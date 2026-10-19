// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package limits

import "testing"

// A guard can be raised or lowered, never switched off: zero and negative
// stand for the default.
func TestSet(t *testing.T) {
	t.Cleanup(func() { Set(Default) })
	Set(Limits{SlobBin: 5, BGLBlock: -1, ImportRatio: 10})
	got := Get()
	want := Default
	want.SlobBin, want.ImportRatio = 5, 10
	if got != want {
		t.Errorf("Get = %+v\nwant  %+v", got, want)
	}
	Set(Limits{})
	if Get() != Default {
		t.Errorf("Set(Limits{}) = %+v, want the defaults", Get())
	}
}
