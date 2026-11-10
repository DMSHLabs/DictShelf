// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

// carriedRegistry is the fixture a copied library makes: a dictionary prepared
// on another machine, its prepared folder under this machine's library root,
// the source itself at a new path, and a rename on the way (a name is not an
// identity). It returns the phone-side source path and the folder that was
// carried.
func carriedRegistry(t *testing.T) (src, folder string) {
	t.Helper()
	isolatedDBDir(t)
	desktop := t.TempDir()
	desktopSrc := filepath.Join(desktop, "copied.dsl")
	if err := os.WriteFile(desktopSrc, []byte(sampleDSL), 0o644); err != nil {
		t.Fatal(err)
	}
	desktopReg, err := NewRegistry([]string{desktop}, false,
		WithPrefs(LoadPrefs(filepath.Join(t.TempDir(), StateFile))), WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, desktopReg)
	desktopDB, ok := store.PreparedFor(desktopSrc)
	if !ok {
		t.Fatal("the source was not prepared on its own machine")
	}
	folder = filepath.Dir(desktopDB)
	closeBackends(t, desktopReg)

	phone := t.TempDir()
	src = filepath.Join(phone, "renamed.dsl")
	if err := os.WriteFile(src, []byte(sampleDSL), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(desktop); err != nil { // that machine, and its path, are gone
		t.Fatal(err)
	}
	return src, folder
}

// The preparation decision asks the library before it believes "no index": the
// folder carried from another machine is re-claimed here, so the queue never
// starts building beside it.
func TestACopiedLibraryIsAdoptedBeforeIndexing(t *testing.T) {
	src, folder := carriedRegistry(t)
	reg, err := NewRegistry(nil, false,
		WithPrefs(LoadPrefs(filepath.Join(t.TempDir(), StateFile))), WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBackends(t, reg)
	if err := reg.SetDirsDeferred([]string{filepath.Dir(src)}); err != nil {
		t.Fatal(err)
	}
	if err := reg.queueNewDictionaries(); err != nil {
		t.Fatal(err)
	}
	state, _ := reg.prefs.data()
	if pending := state.DSLPending; len(pending) != 0 {
		t.Fatalf("the carried dictionary was queued for indexing: %v", pending)
	}
	if dir, ok := store.LookupDir(src); !ok || filepath.Clean(dir) != filepath.Clean(folder) {
		t.Fatalf("LookupDir = %q, %v; want the carried folder %q", dir, ok, folder)
	}
}

// The whole start, on the machine that received the copy: no second index, no
// second folder, and the dictionary answers from the databases that came with
// the library.
func TestACopiedLibraryIsAdoptedInsteadOfRebuilt(t *testing.T) {
	src, folder := carriedRegistry(t)
	reg, err := NewRegistry([]string{filepath.Dir(src)}, false,
		WithPrefs(LoadPrefs(filepath.Join(t.TempDir(), StateFile))), WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBackends(t, reg)
	waitNewDictionaryIndexing(t, reg)

	state, _ := reg.prefs.data()
	if pending := state.DSLPending; len(pending) != 0 {
		t.Fatalf("the carried dictionary was queued for indexing: %v", pending)
	}
	if dir, ok := store.LookupDir(src); !ok || filepath.Clean(dir) != filepath.Clean(folder) {
		t.Fatalf("LookupDir = %q, %v; want the carried folder %q", dir, ok, folder)
	}
	if _, ok := store.PreparedFor(src); !ok {
		t.Error("the carried databases do not answer for this source")
	}
	folders, err := store.Folders()
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 {
		t.Errorf("the library holds %d folders, want the one that was carried", len(folders))
	}
}
