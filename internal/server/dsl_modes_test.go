// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wuweidict/wudict/internal/fsx"
)

func TestSingleDSLIndexRemoval(t *testing.T) {
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
	if len(reg.all()) != 1 {
		t.Fatal("DSL duplicated")
	}
	e := reg.all()[0]
	s := New(reg)
	if err := e.setFeatures(features{Contains: true, FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	global := s.rowsGlobal(s.groupsNow())
	info, _ := s.dictInfoFor(e, global, rowKey(e, global))
	db := info.TextDB
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	reg.prefs.file.path = func() string { return filepath.Join(blocked, StateFile) }
	if rec := deleteReq(t, s, "/api/library?dict="+e.ID+"&prepared=1&source=0"); rec.Code != 400 {
		t.Fatal("failed state save accepted deletion", rec.Code)
	}
	if !fsx.FileExists(db) || e.indexBlocked() {
		t.Fatal("failed state save removed index")
	}
	reg.prefs.file.path = func() string { return state }
	if rec := deleteReq(t, s, "/api/library?dict="+e.ID+"&prepared=1&source=0"); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	groupCall(t, s, "GET", "/api/dicts", nil, 200)
	groupCall(t, s, "GET", "/api/search?q=hello&dict="+e.ID, nil, 404)
	if fsx.FileExists(db) || !e.indexBlocked() {
		t.Fatal("removed index restored implicitly")
	}
	restarted, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, restarted)
	if !restarted.all()[0].indexBlocked() {
		t.Fatal("restart restored index")
	}
	groupCall(t, s, "GET", "/api/ingest?dict="+e.ID+"&fts=1", nil, 200)
	if info, _ := s.dictInfoFor(e, global, rowKey(e, global)); !info.Caps.FTS || info.Caps.Contains || e.indexBlocked() {
		t.Fatal("explicit restore failed")
	}
	if body, err := os.ReadFile(source); err != nil || string(body) != sampleDSL {
		t.Fatal("source changed")
	}
}
