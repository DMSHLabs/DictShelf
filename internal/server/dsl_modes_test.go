// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/fsx"
)

func TestDSLGlobalParser(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), StateFile)
	r, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	s := New(r)
	for _, mode := range []string{"original", "gd", "both"} {
		groupCall(t, s, "PUT", "/api/dsl-mode", map[string]any{"global": true, "mode": mode}, 200)
		for _, e := range r.all() {
			if r.dslAvailable(e) != (mode == "both" || mode == e.dslVariant) {
				t.Fatal("wrong availability", mode, e.dslVariant)
			}
			if r.dslView(e).Parser != mode {
				t.Fatal("parser metadata missing")
			}
		}
		if LoadPrefs(state).parserSelection() != mode {
			t.Fatal("parser not persisted")
		}
	}
}

func TestDSLModesKeepPreparedFiles(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), StateFile)
	reg, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg)
	entries := reg.all()
	if len(entries) != 2 {
		t.Fatalf("want original and GD, got %d", len(entries))
	}
	var original, gd *entry
	for _, e := range entries {
		if e.dslVariant == "gd" {
			gd = e
		} else {
			original = e
		}
		if _, err := e.open(); err != nil {
			t.Fatal(err)
		}
	}
	if original == nil || gd == nil || original.dslSource != gd.dslSource {
		t.Fatal("not paired")
	}
	type savedFile struct {
		path string
		body []byte
		time int64
	}
	var files []savedFile
	for _, e := range entries {
		info := s.dictInfoFor(e)
		if info.TextDB == "" {
			t.Fatal("not prepared")
		}
		for _, path := range []string{e.Path, info.TextDB} {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			st, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, savedFile{path, b, st.ModTime().UnixNano()})
		}
	}
	for _, mode := range []string{"original", "gd", "both"} {
		groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"dict": gd.ID, "mode": mode}, 200)
		// Order and appearance saves must not overwrite the independent selection.
		putPrefs(t, s, `{"dicts":[{"id":"`+original.ID+`"},{"id":"`+gd.ID+`"}],"ui":{"fontSize":24}}`)
		reg.prefs = LoadPrefs(state)
		for _, e := range entries {
			want := mode == "both" || mode == e.dslVariant
			if reg.dslAvailable(e) != want || s.dictInfoFor(e).Unavailable == want {
				t.Fatalf("%s %s availability", mode, e.dslVariant)
			}
		}
		for _, scope := range []string{"all", original.ID + "," + gd.ID} {
			r := httptest.NewRecorder()
			s.ServeHTTP(r, newRequest("GET", "/api/search?q=hello&dict="+scope, nil))
			if r.Code != 200 {
				t.Fatal(r.Body.String())
			}
			var begin streamMsg
			if err := json.NewDecoder(strings.NewReader(r.Body.String())).Decode(&begin); err != nil {
				t.Fatal(err)
			}
			want := 1
			if mode == "both" {
				want = 2
			}
			if len(begin.Slots) != want {
				t.Fatalf("%s search: %s", mode, r.Body.String())
			}
		}
		if mode != "both" {
			hidden := gd
			if mode == "gd" {
				hidden = original
			}
			groupCall(t, s, "GET", "/api/search?q=hello&dict="+hidden.ID, nil, 404)
		}
	}
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"mode": "original"}, 200)
	if reg.dslAvailable(gd) {
		t.Fatal("bulk selection ignored")
	}
	for _, mode := range []string{"", "none", "invalid"} {
		groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"mode": mode}, 400)
	}
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"dict": "missing", "mode": "both"}, 404)
	for _, f := range files {
		b, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(f.path)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(f.body) || st.ModTime().UnixNano() != f.time {
			t.Fatalf("selection changed %s", f.path)
		}
	}
	// Source folders may be detached while the library remains opted in.
	if err := reg.SetUseCached(true); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetDirs(nil); err != nil {
		t.Fatal(err)
	}
	if len(reg.all()) != 2 {
		t.Fatal("cached pair missing")
	}
	for _, e := range reg.all() {
		if e.dslSource != original.dslSource {
			t.Fatal("cached family identity lost")
		}
		if reg.dslAvailable(e) != (e.dslVariant == "original") {
			t.Fatal("cached selection lost")
		}
	}
}

func TestDSLModeSingleVariantAndSaveFailure(t *testing.T) {
	s, _ := newPrefsServer(t)
	e := s.reg.all()[0]
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"dict": e.ID, "mode": "gd"}, 409)
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"mode": "gd"}, 200)
	if !s.reg.dslAvailable(e) {
		t.Fatal("bulk hid the only available copy")
	}
	state := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(state, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	s.reg.prefs.file.path = func() string { return filepath.Join(state, "state.json") }
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"mode": "both"}, 500)
	if s.reg.prefs.dslMode(e.dslSource) != "gd" {
		t.Fatal("failed save changed selection")
	}
}

func TestDSLModeDoesNotPrepareUntilSearch(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg)
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]string{"mode": "gd"}, 200)
	for _, e := range reg.all() {
		if s.dictInfoFor(e).TextDB != "" {
			t.Fatal("selection prepared an index")
		}
	}
	groupCall(t, s, "GET", "/api/search?q=hello&dict=all", nil, 200)
	for _, e := range reg.all() {
		prepared := s.dictInfoFor(e).TextDB != ""
		if prepared != (e.dslVariant == "gd") {
			t.Fatal("search prepared the wrong variant")
		}
	}
}

func TestDSLIndexRemovalRequiresExplicitRebuild(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "demo.dsl")
	if err := os.WriteFile(source, []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), StateFile)
	reg, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, reg)
	s := New(reg)
	entries := reg.all()
	for _, e := range entries {
		if _, err := e.open(); err != nil {
			t.Fatal(err)
		}
		if err := e.setFeatures(features{FullText: true}, nil); err != nil {
			t.Fatal(err)
		}
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	reg.prefs.file.path = func() string { return filepath.Join(blocked, StateFile) }
	failed := entries[0]
	failedDB := s.dictInfoFor(failed).TextDB
	if rec := deleteReq(t, s, "/api/library?dict="+failed.ID+"&prepared=1&source=0"); rec.Code != 400 {
		t.Fatalf("failed state save: %d %s", rec.Code, rec.Body.String())
	}
	if !fsx.FileExists(failedDB) || failed.indexBlocked() {
		t.Fatal("failed state save deleted or disabled index")
	}
	reg.prefs.file.path = func() string { return state }
	for _, removed := range entries {
		var other *entry
		for _, e := range entries {
			if e != removed {
				other = e
			}
		}
		otherDB := s.dictInfoFor(other).TextDB
		before, err := os.ReadFile(otherDB)
		if err != nil {
			t.Fatal(err)
		}
		stamp, err := os.Stat(otherDB)
		if err != nil {
			t.Fatal(err)
		}
		removedDB := s.dictInfoFor(removed).TextDB
		if rec := deleteReq(t, s, "/api/library?dict="+removed.ID+"&prepared=1&source=0"); rec.Code != 200 {
			t.Fatalf("delete index: %d %s", rec.Code, rec.Body.String())
		}
		groupCall(t, s, "GET", "/api/dicts", nil, 200)
		if _, err := os.Stat(removedDB); !os.IsNotExist(err) {
			t.Fatalf("%s index recreated by metadata: %v", removed.dslVariant, err)
		}
		if info := s.dictInfoFor(removed); info.TextDB != "" || !info.DSL.SourceAvailable {
			t.Fatalf("removed index metadata: %+v", info)
		}
		after, err := os.ReadFile(otherDB)
		if err != nil {
			t.Fatal(err)
		}
		newStamp, err := os.Stat(otherDB)
		if err != nil || string(before) != string(after) || !stamp.ModTime().Equal(newStamp.ModTime()) {
			t.Fatal("removal changed the other index")
		}
		for _, e := range entries {
			if _, err := os.Stat(e.Path); err != nil {
				t.Fatalf("removal changed source/descriptor: %v", err)
			}
		}
		body, err := os.ReadFile(source)
		if err != nil || string(body) != sampleDSL {
			t.Fatal("removal changed DSL source")
		}
		groupCall(t, s, "GET", "/api/search?q=hello&dict="+removed.ID, nil, 404)
		reg.prefs = LoadPrefs(state)
		groupCall(t, s, "GET", "/api/dicts", nil, 200)
		if reg.dslAvailable(removed) || !removed.indexBlocked() {
			t.Fatal("removed index enabled after reloading saved preferences")
		}
		if _, err := removed.open(); err == nil {
			t.Fatal("removed index can be implicitly opened")
		}
		groupCall(t, s, "GET", "/api/ingest?dict="+removed.ID+"&fts=1", nil, 200)
		if info := s.dictInfoFor(removed); info.TextDB == "" || !info.Caps.FTS || removed.indexBlocked() {
			t.Fatal("explicit full text did not restore index")
		}
		groupCall(t, s, "GET", "/api/ingest?dict="+removed.ID+"&fts=0", nil, 200)
		if s.dictInfoFor(removed).Caps.FTS || !s.dictInfoFor(other).Caps.FTS {
			t.Fatal("full text toggle affected the other variant")
		}
		groupCall(t, s, "GET", "/api/ingest?dict="+removed.ID+"&fts=1", nil, 200)
	}
	for _, e := range entries {
		if rec := deleteReq(t, s, "/api/library?dict="+e.ID+"&prepared=1&source=0"); rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
	}
	restarted, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, restarted)
	groupCall(t, New(restarted), "GET", "/api/dicts", nil, 200)
	for _, e := range restarted.all() {
		if restarted.dslAvailable(e) || New(restarted).dictInfoFor(e).TextDB != "" {
			t.Fatal("restart restored a removed index")
		}
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if s.dictInfoFor(e).DSL.SourceAvailable {
			t.Fatal("missing source marked rebuildable")
		}
	}
}
