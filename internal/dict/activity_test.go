// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import "testing"

func TestActivity(t *testing.T) {
	const p = "/d/x.wudict.md.gz"
	if _, ok := ActivityOf(p); ok {
		t.Fatal("an activity before any was set")
	}
	SetActivity(p, Activity{Phase: PhaseUnpacking, Done: 1, Total: 2})
	SetActivity(p, Activity{Phase: PhaseIndexing, Done: 3, Total: 4})
	if a, ok := ActivityOf(p); !ok || a != (Activity{Phase: PhaseIndexing, Done: 3, Total: 4}) {
		t.Errorf("ActivityOf = %+v, %v: the last set must win", a, ok)
	}
	ClearActivity(p)
	if _, ok := ActivityOf(p); ok {
		t.Error("cleared, still there")
	}
}
