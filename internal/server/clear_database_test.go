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

	"github.com/wuweidict/wudict/internal/store"
)

func clearDatabaseReq(t *testing.T, s *Server, body string) {
	t.Helper()
	req := newRequest("POST", "/api/clear-database", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var report struct {
		Failed []string `json:"failed"`
	}
	if rec.Code != 200 {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 {
		t.Fatalf("clear failures: %v", report.Failed)
	}
}

func TestClearDatabaseIndependentIndexes(t *testing.T) {
	for _, target := range []string{"contains", "fullText", "index"} {
		t.Run(target, func(t *testing.T) {
			s := newTestServer(t)
			dir := preparedDSL(t, s)
			e := s.reg.entryAt(store.TextDBPath(dir))
			if err := e.setFeatures(features{Contains: true, FullText: true}, nil); err != nil {
				t.Fatal(err)
			}
			clearDatabaseReq(t, s, `{"`+target+`":true}`)
			d, err := e.open()
			if err != nil {
				t.Fatal(err)
			}
			caps := d.Caps()
			if caps.Contains != (target != "contains") || caps.FTS != (target != "fullText") {
				t.Fatalf("wrong capabilities: %+v", caps)
			}
			if _, err := os.Stat(e.Path); err != nil {
				t.Fatalf("source removed: %v", err)
			}
			if result, err := d.Exact("corazon", 10); err != nil || len(result) == 0 {
				t.Fatalf("base lookup: %v %v", result, err)
			}
		})
	}
}

func TestRescanClearsPreviouslyImportedAndDebris(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	if err := store.MarkKept(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.reg.Dirs()[0], "test.dsl")); err != nil {
		t.Fatal(err)
	}
	if err := s.reg.SetUseCached(true); err != nil {
		t.Fatal(err)
	}
	if e := s.reg.entryAt(store.TextDBPath(dir)); e != nil {
		if _, err := e.open(); err != nil {
			t.Fatal(err)
		}
	}
	debris := filepath.Join(store.DefaultDBDir(), "old.media.db")
	if err := os.WriteFile(debris, []byte("old cache"), 0600); err != nil {
		t.Fatal(err)
	}
	rec := localReq(t, s, "GET", "/api/rescan")
	if rec.Code != 200 {
		t.Fatalf("rescan: %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{dir, debris} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("leftover %s: %v", path, err)
		}
	}
	if s.reg.entryAt(store.TextDBPath(dir)) != nil {
		t.Fatal("removed cache still registered")
	}
}

func TestClearDatabaseObsoleteAndSourceProtection(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	media := store.MediaDBPath(dir)
	if err := os.WriteFile(media, []byte("legacy media"), 0600); err != nil {
		t.Fatal(err)
	}
	clearDatabaseReq(t, s, `{"obsolete":true}`)
	if _, err := os.Stat(media); !os.IsNotExist(err) {
		t.Fatalf("media remains: %v", err)
	}
	if _, err := os.Stat(store.TextDBPath(dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.reg.Dirs()[0], "test.dsl.files.zip")); err != nil {
		t.Fatalf("source media removed: %v", err)
	}
	req := newRequest("POST", "/api/clear-database", strings.NewReader(`{"index":true}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("remote clear: %d", rec.Code)
	}
}

func TestRescanClearsUnselectedSourcesButKeepsActiveIndexes(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	original := filepath.Join(s.reg.Dirs()[0], "test.dsl")
	sourceMedia := original + ".files.zip"
	next := t.TempDir()
	if err := os.WriteFile(filepath.Join(next, "new.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.reg.SetDirs([]string{next}); err != nil {
		t.Fatal(err)
	}
	if err := s.reg.cleanupLibrary(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("unselected cache remains: %v", err)
	}
	for _, path := range []string{original, sourceMedia} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	e, err := s.reg.get(idOf(t, s, "new.dsl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.open(); err != nil {
		t.Fatal(err)
	}
	active, ok := store.LookupDir(e.Path)
	if !ok {
		t.Fatal("active index missing")
	}
	if err := s.reg.cleanupLibrary(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.TextDBPath(active)); err != nil {
		t.Fatalf("active index removed: %v", err)
	}
}

func TestClearDatabasePreservesExplicitDatabaseSource(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	if err := s.reg.SetDirs([]string{dir}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.TextDBPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	clearDatabaseReq(t, s, `{"index":true,"contains":true,"fullText":true,"obsolete":true}`)
	after, err := os.ReadFile(store.TextDBPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("database source modified")
	}
}

func TestCleanupKeepsInactiveDSLVariant(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, e := range reg.all() {
		if _, err := e.open(); err != nil {
			t.Fatal(err)
		}
		prepared, ok := store.LookupDir(e.Path)
		if !ok {
			t.Fatal("variant index missing")
		}
		paths = append(paths, store.TextDBPath(prepared))
	}
	if len(paths) != 2 {
		t.Fatalf("expected two variants: %v", paths)
	}
	reg.prefs.mu.Lock()
	reg.prefs.dslParser = "original"
	reg.prefs.mu.Unlock()
	if err := reg.cleanupLibrary(); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("variant removed: %v", err)
		}
	}
}
