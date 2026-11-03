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

// A holder that outlives the retry must not leave HALF a dictionary in place of
// one. The removal moves the folder out of the way before deleting anything, and
// on Windows a held file blocks a rename exactly as it blocks an unlink - so a
// removal that cannot even move the folder leaves the folder whole and says so,
// where deleting file by file would have taken text.db away and left the caches.
func TestARemovalThatCannotMoveTheFolderLeavesItWhole(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WUDICT_DB_DIR", root)
	dir := filepath.Join(root, "dict")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{TextDBName, ArticlesDBName, "icon.cache", "stats.cache"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("published"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	held, err := os.Open(TextDBPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	if _, err := RemovePrepared(dir); err == nil {
		t.Fatal("a holder that outlives the retry must fail the removal")
	}
	for _, name := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("the failed removal took %s away: %v", name, err)
		}
	}
	if debris, _ := filepath.Glob(filepath.Join(root, "*"+removalInfix+"*")); len(debris) != 0 {
		t.Errorf("a removal that could not move the folder left debris anyway: %v", debris)
	}

	// With the holder gone the same call does its whole job.
	held.Close()
	if _, err := RemovePrepared(dir); err != nil {
		t.Fatalf("the removal did not succeed once the holder let go: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the folder is still there: %v", err)
	}
}
