// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wuweidict/wudict/internal/store"
)

// entryState reads the parts of an entry a rescan is supposed to re-derive.
func entryState(e *entry) (d interface{}, err error, backing string) {
	e.dMu.RLock()
	defer e.dMu.RUnlock()
	return e.d, e.err, e.backing
}

// A library folder deleted from outside the app - a file manager, a cleanup
// script - must survive "Rescan folders": the dictionary falls back to its
// source and the explicit rescan preparation path rebuilds it, rather than holding a
// handle to the deleted database and answering every search with SQLite's
// "unable to open database file", with neither preparation lane willing to
// rebuild it.
func TestRescanRecoversFromDeletedPreparedFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows refuses to delete a database the app holds open: the folder cannot vanish underneath it")
	}
	restorePower(t)
	s, e := unpreparedEntry(t)
	if err := e.setFeatures(features{}, nil, true); err != nil {
		t.Fatalf("explicit index creation: %v", err)
	}
	if _, err := e.open(); err != nil {
		t.Fatalf("open after preparing: %v", err)
	}
	if d, _, backing := entryState(e); backing == "" {
		t.Fatalf("prepared entry recorded no backing database (backend %T)", d)
	} else if _, ok := d.(storeBacked); !ok {
		t.Fatalf("prepared entry is not store-backed: %T", d)
	}

	dir, ok := store.LookupDir(e.Path)
	if !ok {
		t.Fatal("prepared folder not found")
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}

	d, err, backing := entryState(e)
	if d != nil || err != nil || backing != "" {
		t.Fatalf("rescan kept stale state: backend=%T err=%v backing=%q", d, err, backing)
	}
	// Search falls back to the source and never recreates the index.
	hits := searchStream(t, s, "/api/search?q=beta&mode=prefix&dict="+e.ID)
	if len(hits) != 1 || hits[0].Error != "" || len(hits[0].Results) != 1 {
		t.Fatalf("search after external index deletion = %+v", hits)
	}
	if prepared(e) {
		t.Fatal("search recreated the deleted index")
	}
}

// entry.open memoizes failures on purpose (a fan-out must not retry a broken
// file per keystroke), but the memo must not outlive a rescan: that button is
// the user saying "look again". Set directly rather than provoked,
// because what is under test is the lifetime of the memo, not any one failure.
func TestRescanClearsMemoizedOpenError(t *testing.T) {
	s, e := unpreparedEntry(t)

	e.dMu.Lock()
	e.err = errors.New("boom")
	e.dMu.Unlock()
	if _, err := e.open(); err == nil {
		t.Fatal("open should have returned the memoized failure")
	}

	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	if _, err, _ := entryState(e); err != nil {
		t.Fatalf("rescan kept the memoized error: %v", err)
	}
	if _, err := e.open(); err != nil {
		t.Fatalf("open after rescan: %v", err)
	}
}

// A dictionary that vanishes from the scan stops resolving - and must also stop
// costing. Its backend is unreachable once the entry leaves the registry, so
// nothing else can ever close it: descriptors and, in preview mode, a headword
// map of hundreds of bytes per entry.
func TestRescanClosesVanishedDictionary(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.fake", "b.fake"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	isolatedDBDir(t)
	reg, err := NewRegistry([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	var vanishing *entry
	for _, e := range reg.all() {
		if _, err := e.open(); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(e.Path) == "a.fake" {
			vanishing = e
		}
	}
	if vanishing == nil {
		t.Fatal("a.fake was not discovered")
	}

	if err := os.Remove(filepath.Join(dir, "a.fake")); err != nil {
		t.Fatal(err)
	}
	if err := reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	if n := reg.Count(); n != 1 {
		t.Fatalf("registry still lists %d dictionaries", n)
	}
	if _, err := reg.get(vanishing.ID); err == nil {
		t.Fatal("a deleted dictionary is still addressable by id")
	}
	if d, _, _ := entryState(vanishing); d != nil {
		t.Fatalf("the backend of a vanished dictionary was left open: %T", d)
	}
}
