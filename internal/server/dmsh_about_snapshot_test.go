// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// The About reports the size of the dictionary's source files - what the
// information dialog prints above the headword count. It is stat'd, never
// read, and a source that is gone reports nothing rather than failing.
func TestDesktopAboutSnapshotReportsSourceSize(t *testing.T) {
	s, id := aboutServer(t, "")
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	var want int64
	for _, path := range dict.SourceFiles(e.Path) {
		want += fsx.FileSize(path)
	}
	if want <= 0 {
		t.Fatalf("the fixture has no source files to measure")
	}
	value, err := s.DesktopJSON(aboutBase, "about", id)
	if err != nil {
		t.Fatal(err)
	}
	if got := value.(aboutInfo).FileSize; got != want {
		t.Fatalf("fileSize = %d, want %d (the files on disk)", got, want)
	}
	if err := os.Remove(e.Path); err != nil {
		t.Fatal(err)
	}
	value, err = s.DesktopJSON(aboutBase, "about", id)
	if err != nil {
		t.Fatal(err)
	}
	if gone := value.(aboutInfo).FileSize; gone != 0 {
		t.Fatalf("fileSize after the source was removed = %d, want 0", gone)
	}
}

func TestDesktopAboutSnapshotWithoutRouter(t *testing.T) {
	s, id := aboutServer(t, "#LANGUAGE \"English\"\nPublished by nobody.\n")
	req := newRequest("GET", "/api/about?dict="+id, nil)
	req.Host = "127.0.0.1:6888"
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status %d: %s", recorder.Code, recorder.Body.String())
	}
	var want aboutInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &want); err != nil {
		t.Fatal(err)
	}

	mux := s.mux
	s.mux = http.NewServeMux()
	defer func() { s.mux = mux }()
	gotValue, err := s.DesktopJSON(aboutBase, "about", id)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gotValue.(aboutInfo)
	if !ok {
		t.Fatalf("native result has type %T, want aboutInfo", gotValue)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP and native About differ:\nHTTP: %+v\nnative: %+v", want, got)
	}
	e, _ := s.reg.get(id)
	e.dMu.RLock()
	opened := e.d != nil
	e.dMu.RUnlock()
	if opened {
		t.Fatal("native About opened the dictionary")
	}
}

func TestDesktopAboutSnapshotErrorsMatchHTTP(t *testing.T) {
	s := newTestServer(t)
	request := newRequest("GET", "/api/about?dict=missing", nil)
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, request)
	var payload struct {
		Error string `json:"error"`
	}
	if recorder.Code != http.StatusNotFound || json.Unmarshal(recorder.Body.Bytes(), &payload) != nil {
		t.Fatalf("HTTP error response %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := s.DesktopJSON(aboutBase, "about", "missing"); err == nil || err.Error() != payload.Error {
		t.Fatalf("native unknown-dictionary error = %v, want %q", err, payload.Error)
	}
	if _, err := s.DesktopJSON("", "about", "missing"); err == nil || err.Error() != "invalid desktop server address" {
		t.Fatalf("native invalid-origin error = %v", err)
	}
}
