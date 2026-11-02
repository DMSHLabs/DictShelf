// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A file another goroutine is holding for a moment must not fail a deletion:
// Windows refuses to unlink an open file, and this is how a dictionary came to
// be reported as "being used by another process" with its folder half removed.
// The removal waits the holder out and the folder goes.
func TestRemovingAFolderWaitsForATransientHolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WUDICT_DB_DIR", root)
	dir := filepath.Join(root, "dict")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	held := StatsPath(dir)
	if filepath.Dir(held) != dir {
		t.Fatalf("StatsPath(%s) = %s, want a file inside that folder", dir, held)
	}
	if err := os.WriteFile(held, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(held)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		f.Close()
	}()
	if _, err := RemovePrepared(dir); err != nil {
		t.Fatalf("a holder that let go still failed the removal: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the folder is still there: %v", err)
	}
}
