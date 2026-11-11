// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// A library copied beside the active one is removed; a folder that never held
// a prepared dictionary is not, whatever its name; the library itself, its
// logs and anything else stay.
func TestCleanSiblingLibraries(t *testing.T) {
	parent := t.TempDir()
	db := filepath.Join(parent, "db")
	if err := os.MkdirAll(db, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WUDICT_DB_DIR", db)
	library := func(dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, "Dict"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(TextDBPath(filepath.Join(dir, "Dict")), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	library(db)
	library(filepath.Join(parent, "db1"))
	library(filepath.Join(parent, "db2"))
	// Named like a copy, but holds no prepared dictionary: not ours to remove.
	foreign := filepath.Join(parent, "db3")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(parent, "db.logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, freed := CleanSiblingLibraries()
	if len(removed) != 2 || removed[0] != "db1" || removed[1] != "db2" {
		t.Fatalf("removed %v, want db1 and db2", removed)
	}
	if freed <= 0 {
		t.Errorf("freed %d bytes, want the size of what was removed", freed)
	}
	for _, gone := range []string{"db1", "db2"} {
		if _, err := os.Stat(filepath.Join(parent, gone)); !os.IsNotExist(err) {
			t.Errorf("%s survived: %v", gone, err)
		}
	}
	for _, kept := range []string{db, foreign, logs} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed: %v", filepath.Base(kept), err)
		}
	}
}
