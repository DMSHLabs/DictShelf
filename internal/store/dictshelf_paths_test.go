// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultDBDirUsesDictShelfDataRoot(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	t.Setenv("WUDICT_HOME", "")
	t.Setenv("WUDICT_DB_DIR", "")
	t.Setenv("WUDICT_PORTABLE_HOME", "")

	previous := dbDir.Swap(nil)
	t.Cleanup(func() { dbDir.Store(previous) })

	want := filepath.Join(home, ".dmshlabs", "dictshelf", "db")
	if got := filepath.Clean(DefaultDBDir()); got != filepath.Clean(want) {
		t.Fatalf("DefaultDBDir() = %q, want %q", got, want)
	}
}

