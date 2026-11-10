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

// releaseBackends closes every open backend of a registry immediately.
func releaseBackends(reg *Registry) {
	reg.jobs.wg.Wait()
	for _, e := range reg.all() {
		e.ingestMu.Lock()
		e.closeNow()
		e.ingestMu.Unlock()
	}
}

// carriedRegistry is the fixture a copied library makes: a dictionary prepared
// on another machine, its prepared folder under this machine's library root,
// the source itself at a new path, and a rename on the way (a name is not an
// identity). It returns the phone-side source path and the folder that was
// carried.
func carriedRegistry(t *testing.T) (src, folder string) {
	return carriedRegistryWith(t, indexOptions{Contains: true})
}

// carriedRegistryWith is carriedRegistry for a machine whose index defaults were
// different, so the library can be carried with indexes this machine's defaults
// would not have built.
func carriedRegistryWith(t *testing.T, defaults indexOptions) (src, folder string) {
	t.Helper()
	isolatedDBDir(t)
	desktop := t.TempDir()
	desktopSrc := filepath.Join(desktop, "copied.dsl")
	if err := os.WriteFile(desktopSrc, []byte(sampleDSL), 0o644); err != nil {
		t.Fatal(err)
	}
	desktopPrefs := LoadPrefs(filepath.Join(t.TempDir(), StateFile))
	if err := desktopPrefs.mutate(func(f *prefsFile) { f.IndexDefaults = &defaults }); err != nil {
		t.Fatal(err)
	}
	desktopReg, err := NewRegistry([]string{desktop}, false,
		WithPrefs(desktopPrefs), WithComparisons(false))
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
	// Release the handles now, not at cleanup: a later test step may replace the
	// carried database, and Windows refuses a rename over an open one.
	releaseBackends(desktopReg)

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

// A carried library that already has a full-text index keeps it: the queue is
// preparation nobody asked to change, so it adds the indexes a dictionary lacks
// and never drops one it has (D151). Without this the first start after a copy
// rebuilt every dictionary that came with more than the current defaults.
func TestACarriedFullTextIndexIsKept(t *testing.T) {
	src, folder := carriedRegistryWith(t, indexOptions{Contains: true, FullText: true})
	textDB := store.TextDBPath(folder)
	if !store.Inspect(textDB).Plan.FullText {
		t.Fatal("the fixture did not get its full-text index")
	}
	before, err := os.Stat(textDB)
	if err != nil {
		t.Fatal(err)
	}

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
	after, err := os.Stat(textDB)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Errorf("the database was rewritten (%v -> %v)", before.ModTime(), after.ModTime())
	}
	if p := store.Inspect(textDB); !p.Plan.FullText || len(p.TextStale(src)) != 0 {
		t.Errorf("the full-text index was dropped or the data is now stale: %+v %v", p.Plan, p.TextStale(src))
	}
}

// A pending entry written by an interrupted run is resolved before it is
// believed: the queue asks the library for a folder prepared from the file, and
// what it finds clears the entry instead of rebuilding (the state a stopped
// batch leaves behind).
func TestAPendingEntryIsResolvedBeforeItIsQueued(t *testing.T) {
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
	entries := reg.all()
	if len(entries) != 1 {
		t.Fatalf("expected one discovered dictionary, got %d", len(entries))
	}
	key := entries[0].indexRemovalKey()
	if err := reg.prefs.mutate(func(f *prefsFile) {
		if f.DSLPending == nil {
			f.DSLPending = map[string]indexOptions{}
		}
		f.DSLPending[key] = indexOptions{Contains: true}
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.queueNewDictionaries(); err != nil {
		t.Fatal(err)
	}
	state, _ := reg.prefs.data()
	if pending := state.DSLPending; len(pending) != 0 {
		t.Fatalf("the resolved dictionary stayed queued: %v", pending)
	}
	if dir, ok := store.LookupDir(src); !ok || filepath.Clean(dir) != filepath.Clean(folder) {
		t.Fatalf("LookupDir = %q, %v; want the carried folder %q", dir, ok, folder)
	}
}

// A queued request is merged with what the database has while the queue is
// filled: adding full text must not drop the contains index that was already
// there, because that index was never asked about.
func TestAQueuedRequestKeepsTheIndexesTheDictionaryHas(t *testing.T) {
	src, folder := carriedRegistry(t) // headwords + contains, no full text
	reg, err := NewRegistry(nil, false,
		WithPrefs(LoadPrefs(filepath.Join(t.TempDir(), StateFile))), WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBackends(t, reg)
	if err := reg.SetDirsDeferred([]string{filepath.Dir(src)}); err != nil {
		t.Fatal(err)
	}
	entries := reg.all()
	if len(entries) != 1 {
		t.Fatalf("expected one discovered dictionary, got %d", len(entries))
	}
	// A request for full text that says nothing about contains: the contains
	// index is already there and must survive.
	if err := reg.prefs.mutate(func(f *prefsFile) {
		if f.DSLPending == nil {
			f.DSLPending = map[string]indexOptions{}
		}
		f.DSLPending[entries[0].indexRemovalKey()] = indexOptions{FullText: true}
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.queueNewDictionaries(); err != nil {
		t.Fatal(err)
	}
	state, _ := reg.prefs.data()
	plan, pending := state.DSLPending[entries[0].indexRemovalKey()]
	if !pending {
		t.Fatal("the request for full text was dropped instead of queued")
	}
	if !plan.FullText || !plan.Contains {
		t.Fatalf("the queued request lost an index the database has: %+v", plan)
	}
	if _, ok := store.LookupDir(src); !ok || filepath.Clean(folder) != folder {
		t.Fatalf("the carried folder is not the one resolved: %q", folder)
	}
}
