// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
)

func TestDSLComparisonSearchAndResources(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "test.dsl")
	if err := os.WriteFile(p, []byte("#NAME \"Compare\"\ngive\n~ up\n\t[ref]some [b]word[/b][/ref]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry([]string{dir, dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, r)
	if r.Count() != 2 {
		t.Fatalf("count: %d", r.Count())
	}
	var original, alternative *entry
	for _, e := range r.all() {
		if e.Path == p {
			original = e
		} else {
			alternative = e
		}
	}
	if original == nil || alternative == nil || original.ID == alternative.ID {
		t.Fatal("missing independent entries")
	}
	for _, e := range []*entry{original, alternative} {
		d, err := e.open()
		if err != nil {
			t.Fatal(err)
		}
		results, err := d.Exact("give up", 10)
		if err != nil {
			t.Fatal(err)
		}
		if e == original && len(results) != 0 {
			t.Fatal("current parser changed", results)
		}
		if e == alternative && (len(results) != 1 || !strings.Contains(results[0].Body, `some <b>word</b></a>`)) {
			t.Fatal("GD lookup", results)
		}
		if e == alternative && d.Meta().Name != "Compare GD" {
			t.Fatal(d.Meta())
		}
	}
	currentDB, _ := store.PreparedFor(original.Path)
	gdDB, _ := store.PreparedFor(alternative.Path)
	if currentDB == "" || gdDB == "" || currentDB == gdDB {
		t.Fatal(currentDB, gdDB)
	}
	// The GD view owns only its reference, never the shared dictionary/media.
	files := dict.SourceFiles(alternative.Path)
	for _, f := range files {
		if f == p || strings.HasSuffix(f, "image.png") {
			t.Fatal("GD owns shared source", files)
		}
	}
	s := New(r)
	rows := getDicts(t, s, "/api/dicts")
	if len(rows) != 2 {
		t.Fatal(rows)
	}
	hits := searchStream(t, s, "/api/search?q=give&dict=all")
	if len(hits) != 2 {
		t.Fatal(hits)
	}
	asset := getJSON(t, s, "/res/"+alternative.ID+"/image.png", nil)
	if asset.Code != 200 || asset.Body.String() != "asset" {
		t.Fatal(asset.Code, asset.Body.String())
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	if r.Count() != 2 {
		t.Fatalf("rescan duplicated comparison: %d", r.Count())
	}
	// Cached-only mode must keep the same GD identity and allow rebuilding it.
	if err := r.SetDirs(nil); err != nil {
		t.Fatal(err)
	}
	if r.Count() != 2 {
		t.Fatalf("cached count: %d", r.Count())
	}
	if e, err := r.get(alternative.ID); err != nil || e.Path != alternative.Path {
		t.Fatal(e, err)
	}
	rep, err := r.Remove(alternative.ID, true, true)
	if err != nil || !rep.Gone {
		t.Fatal(rep, err)
	}
	if r.Count() != 1 {
		t.Fatalf("removal recreated GD: %d", r.Count())
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("shared DSL deleted", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "image.png")); err != nil {
		t.Fatal("shared media deleted", err)
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	if r.Count() != 1 {
		t.Fatal("GD removal not persistent")
	}
}

func TestDSLComparisonOrphans(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "orphan.dsl")
	if err := os.WriteFile(source, []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry([]string{dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, r)
	for _, e := range r.all() {
		if _, err := e.open(); err != nil {
			t.Fatal(err)
		}
	}
	s := New(r)
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"mode": "original"}, 200)
	if list := listOrphans(t, s); len(list) != 0 {
		t.Fatal("disabled GD index is not an orphan", list)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	list := listOrphans(t, s)
	if len(list) != 2 {
		t.Fatalf("both indexes lost their real source, got %+v", list)
	}
	for _, orphan := range list {
		if e := r.entryAt(store.TextDBPath(filepath.Join(store.DefaultDBDir(), orphan.Folder))); e != nil {
			if _, err := e.open(); err != nil {
				t.Fatal(err)
			}
		}
	}
	folders := []string{list[0].Folder, list[1].Folder}
	report := r.ResolveOrphans(folders, nil)
	if len(report.Deleted) != 2 || len(report.Failed) != 0 {
		t.Fatalf("delete open indexes: %+v", report)
	}
	if list := listOrphans(t, s); len(list) != 0 {
		t.Fatal(list)
	}
}
