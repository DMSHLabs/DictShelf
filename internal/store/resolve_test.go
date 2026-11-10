// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// foreignClaim is the claim a library copied from another machine keeps: an
// absolute path of an operating system this one is not.
func foreignClaim(name string) string {
	if runtime.GOOS == "windows" {
		return "/storage/emulated/0/DictShelf/" + name
	}
	return `D:\DictShelf\` + name
}

// A prepared folder whose claim names the machine the library came from is this
// dictionary's prepared data, not a stranger's: adopting it keeps the databases
// and rewrites the claim, so nothing is re-indexed.
func TestResolveDirAdoptsAFolderPreparedElsewhere(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	src := writeSrc(t, filepath.Join(t.TempDir(), "copied.mdx"), "the same dictionary bytes")
	dir := prepare(t, src, "Copied")
	was := foreignClaim("copied.mdx")
	if err := writeInfo(dir, was); err != nil {
		t.Fatal(err)
	}
	if _, ok := LookupDir(src); ok {
		t.Fatal("a claim naming another machine's path matched this one")
	}

	got, ok := ResolveDir(src)
	if !ok || got != dir {
		t.Fatalf("ResolveDir = %q, %v; want %q", got, ok, dir)
	}
	if _, ok := LookupDir(src); !ok {
		t.Error("adoption did not re-point the claim")
	}
	if got := Stale(TextDBPath(dir), src); len(got) != 0 {
		t.Errorf("the adopted database does not answer for this source: %v", got)
	}

	info, err := readInfo(InfoPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info[adoptedKey] == "" || info[adoptedFromKey] != was {
		t.Errorf("the receipt does not record the adoption: %v", info)
	}
	// The record is history, so a regeneration carries it over like the keep
	// marker - otherwise the one line explaining a surprising dictionary would
	// last until the next media pack.
	if err := WriteInfo(dir); err != nil {
		t.Fatal(err)
	}
	if info, err = readInfo(InfoPath(dir)); err != nil || info[adoptedFromKey] != was {
		t.Errorf("a regenerated receipt lost the adoption record: %v, %v", info, err)
	}
}

// The size agreeing is not the file agreeing: the folder belongs to the
// dictionary it was prepared from and must be left alone.
func TestResolveDirRefusesDifferentBytes(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	src := writeSrc(t, filepath.Join(t.TempDir(), "a.mdx"), "AAAA")
	dir := prepare(t, src, "A")
	if err := writeInfo(dir, foreignClaim("a.mdx")); err != nil {
		t.Fatal(err)
	}
	// Same name, same size, different content - the only case that could make a
	// scripted rebuild look like the same dictionary.
	other := writeSrc(t, filepath.Join(t.TempDir(), "a.mdx"), "BBBB")
	if got, ok := ResolveDir(other); ok {
		t.Fatalf("adopted %q for different bytes", got)
	}
}

// The guard under every preparation: Reconcile asks the library before it opens
// the source, so no caller - a queue, a single-dictionary rebuild, the CLI - can
// build a second database beside one prepared from the same file.
func TestReconcileAdoptsInsteadOfBuilding(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	src := writeSrc(t, filepath.Join(t.TempDir(), "moved.mdx"), "the dictionary bytes")
	dir := prepare(t, src, "Moved")
	if err := writeInfo(dir, foreignClaim("moved.mdx")); err != nil {
		t.Fatal(err)
	}
	out, err := Reconcile(src, Target{}, Hooks{Reader: func() (dict.Reader, error) {
		t.Error("the source was opened for an ingest although its prepared folder was there")
		return nil, errors.New("unexpected ingest")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Rebuilt {
		t.Error("Reconcile rebuilt data it had")
	}
	if out.TextDB != TextDBPath(dir) {
		t.Errorf("Reconcile worked on %q, want the carried folder %q", out.TextDB, TextDBPath(dir))
	}
}

// A folder another source of this machine owns is not adoptable: taking it would
// leave that dictionary unowned, and two identical files are still two entries.
func TestResolveDirLeavesALiveFolderAlone(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	first := writeSrc(t, filepath.Join(t.TempDir(), "first.mdx"), "identical bytes")
	prepare(t, first, "First")
	second := writeSrc(t, filepath.Join(t.TempDir(), "second.mdx"), "identical bytes")
	if got, ok := ResolveDir(second); ok {
		t.Fatalf("took the folder a live source owns: %q", got)
	}
}

// A claim of another machine's absolute path is a source this machine does not
// have: absent, not unknowable. That is what makes a carried folder relinkable
// instead of an orphan.
func TestSourceGoneSeesAForeignPath(t *testing.T) {
	if !sourceGone(foreignClaim("x.dsl")) {
		t.Error("another OS's absolute path was treated as possibly present")
	}
	if sourceGone("relative/x.dsl") {
		t.Error("a relative claim was judged")
	}
	if sourceGone("") {
		t.Error("an empty claim was judged")
	}
}

// The list row asks CheapStale: the size and the revision, not the content
// sample. A same-size edit is therefore invisible to it and still caught where
// a rebuild is decided.
func TestCheapStaleSkipsTheContentSample(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	src := writeSrc(t, filepath.Join(t.TempDir(), "s.mdx"), "AAAA")
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	ingestSrc(t, src, textDB, Plan{})
	p := Inspect(textDB)
	if got := p.CheapStale(src); len(got) != 0 {
		t.Fatalf("CheapStale of a fresh build = %v", got)
	}
	// Same size, different bytes: the sample sees it, the cheap ask does not.
	writeSrc(t, src, "BBBB")
	if got := p.TextStale(src); len(got) == 0 {
		t.Fatal("TextStale did not see a same-size edit")
	}
	if got := p.CheapStale(src); len(got) != 0 {
		t.Errorf("CheapStale read the source's content: %v", got)
	}
	// A different size is cheap to notice and is noticed.
	writeSrc(t, src, "AAAAAAA")
	if got := p.CheapStale(src); len(got) == 0 {
		t.Error("CheapStale missed a changed size")
	}
}
