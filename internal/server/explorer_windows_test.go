// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package server

import "testing"

// Explorer recognises /select, only unquoted, with the path quoted after it.
func TestExplorerCmdLine(t *testing.T) {
	got := explorerCmdLine(`C:\Users\Jo Smith\.wudict\db\Duden, 7. Auflage`)
	want := `explorer /select,"C:\Users\Jo Smith\.wudict\db\Duden, 7. Auflage"`
	if got != want {
		t.Errorf("command line = %s, want %s", got, want)
	}
}
