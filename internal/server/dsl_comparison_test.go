// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleDSLSearchAndResources(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "test.dsl")
	if err := os.WriteFile(source, []byte("#NAME \"Compare\"\ngive\n~ up\n\t[ref]some [b]word[/b][/ref]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte("asset"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry([]string{dir, dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, r)
	if r.Count() != 1 || r.UserCount() != 1 {
		t.Fatal("duplicated DSL")
	}
	e := r.all()[0]
	d, err := e.open()
	if err != nil {
		t.Fatal(err)
	}
	hits, err := d.Exact("give up", 10)
	if err != nil || len(hits) != 1 || !strings.Contains(hits[0].Body, "some <b>word</b></a>") {
		t.Fatal(hits, err)
	}
	if d.Meta().Name != "Compare" {
		t.Fatal("generated suffix", d.Meta())
	}
	s := New(r)
	asset := getJSON(t, s, "/res/"+e.ID+"/image.png", nil)
	if asset.Code != 200 || asset.Body.String() != "asset" {
		t.Fatal(asset.Body.String())
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	if r.Count() != 1 {
		t.Fatal("duplicate after rescan")
	}
	if err := r.SetDirs(nil); err != nil {
		t.Fatal(err)
	}
	if r.Count() != 1 {
		t.Fatal("duplicate cached dictionary")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	orphans := listOrphans(t, s)
	if len(orphans) != 1 {
		t.Fatal(orphans)
	}
	for _, cached := range r.all() {
		if _, err := cached.open(); err != nil {
			t.Fatal(err)
		}
	}
	report := r.ResolveOrphans([]string{orphans[0].Folder}, nil)
	if len(report.Deleted) != 1 || len(report.Failed) != 0 {
		t.Fatal(report)
	}
}
