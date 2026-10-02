// Copyright (C) 2026 glowinthedark
//
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

// preparedDSL opens the test server's DSL dictionary, which prepares it (DSL
// is always ingested on open), and returns its library folder.
func preparedDSL(t *testing.T, s *Server) string {
	t.Helper()
	e, err := s.reg.get(idOf(t, s, "test.dsl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.open(); err != nil {
		t.Fatal(err)
	}
	dir, ok := store.LookupDir(e.Path)
	if !ok {
		t.Fatal("opening the DSL prepared no folder")
	}
	return dir
}

func orphansReq(t *testing.T, s *Server, remote bool, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := newRequest("POST", "/api/orphans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if !remote {
		req.RemoteAddr = "127.0.0.1:5555"
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func listOrphans(t *testing.T, s *Server) []store.Orphan {
	t.Helper()
	rec := localReq(t, s, "GET", "/api/orphans")
	if rec.Code != 200 {
		t.Fatalf("GET /api/orphans = %d: %s", rec.Code, rec.Body.String())
	}
	var j struct {
		Orphans []store.Orphan `json:"orphans"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	return j.Orphans
}

// The user deletes a dictionary's files by hand; the rescan offers its
// prepared folder, and answering "delete" removes it - even while the library
// is in use and the orphan is listed and open.
func TestOrphanOfferedAndDeleted(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	src := s.reg.Dirs()[0]
	for _, p := range []string{"test.dsl", "test.dsl.files.zip"} {
		if err := os.Remove(filepath.Join(src, p)); err != nil {
			t.Fatal(err)
		}
	}
	s.reg.SetUseCached(true) // rescans; the orphan is now listed from the library
	if e := s.reg.entryAt(store.TextDBPath(dir)); e == nil {
		t.Fatal("the orphan is not listed from the library")
	} else if _, err := e.open(); err != nil {
		t.Fatal(err)
	}

	list := listOrphans(t, s)
	if len(list) != 1 || list[0].Folder != filepath.Base(dir) {
		t.Fatalf("orphans = %+v", list)
	}

	if rec := orphansReq(t, s, true, `{"delete":["`+list[0].Folder+`"]}`); rec.Code != 403 {
		t.Fatalf("remote POST = %d, want 403", rec.Code)
	}
	if rec := orphansReq(t, s, false, `{"delete":["../x"]}`); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "not a library folder name") {
		t.Fatalf("a path was not refused: %d %s", rec.Code, rec.Body.String())
	}

	rec := orphansReq(t, s, false, `{"delete":["`+list[0].Folder+`"]}`)
	var rep orphanReport
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil || rec.Code != 200 {
		t.Fatalf("POST = %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	if len(rep.Deleted) != 1 || len(rep.Failed) != 0 || rep.Freed <= 0 {
		t.Fatalf("report = %+v", rep)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("folder survived: %v", err)
	}
	if s.reg.entryAt(store.TextDBPath(dir)) != nil {
		t.Fatal("the registry still lists the deleted orphan")
	}
}

// "Keep" is remembered, and so is removing the files from the app while
// keeping the prepared data: neither is offered again.
func TestOrphanKept(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	if err := os.Remove(filepath.Join(s.reg.Dirs()[0], "test.dsl")); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Base(dir)
	rec := orphansReq(t, s, false, `{"keep":["`+folder+`"]}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"kept":[{"folder"`) {
		t.Fatalf("keep = %d %s", rec.Code, rec.Body.String())
	}
	if list := listOrphans(t, s); len(list) != 0 {
		t.Fatalf("a kept orphan is still offered: %+v", list)
	}
	if _, err := os.Stat(store.TextDBPath(dir)); err != nil {
		t.Fatalf("keeping deleted it: %v", err)
	}
	// Named in both lists: refused, not guessed.
	if rec := orphansReq(t, s, false, `{"delete":["a"],"keep":["a"]}`); !strings.Contains(rec.Body.String(), "more than once") {
		t.Fatalf("conflict = %s", rec.Body.String())
	}
}

// A dictionary file moved to another place in the scanned folder is not an
// orphan: the rescan re-links its prepared folder and it is served from it.
func TestRescanRelinksAMovedSource(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	src := s.reg.Dirs()[0]
	sub := filepath.Join(src, "moved")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"test.dsl", "test.dsl.files.zip"} {
		if err := os.Rename(filepath.Join(src, p), filepath.Join(sub, p)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	if list := listOrphans(t, s); len(list) != 0 {
		t.Fatalf("a moved source left an orphan: %+v", list)
	}
	e, err := s.reg.get(idOf(t, s, "test.dsl"))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := store.LookupDir(e.Path); !ok || got != dir {
		t.Fatalf("LookupDir(%s) = %q, %v; want %q", e.Path, got, ok, dir)
	}
}
