// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/store"
)

func TestRescanStreamsDictionaryAndArticleProgress(t *testing.T) {
	s := newTestServer(t)
	preparedDSL(t, s)
	req := newRequest("POST", "/api/rescan?stream=1", strings.NewReader(`{"new":{"index":true},"existing":{"index":"recreate"}}`))
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/x-ndjson" || !rec.Flushed {
		t.Fatalf("stream response: %d %v", rec.Code, rec.Header())
	}
	decoder := json.NewDecoder(rec.Body)
	var dictionary, articles, cleanup, completed bool
	for {
		var message struct {
			Type string `json:"t"`
			rescanIndexProgress
			Failed []string `json:"failed"`
		}
		if err := decoder.Decode(&message); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if completed {
			t.Fatal("event after final result")
		}
		switch message.Type {
		case "progress":
			if message.Stage == "dictionary" {
				if message.At < 1 || message.At > message.Total || message.Name == "" {
					t.Fatalf("invalid progress: %+v", message)
				}
				dictionary = true
				articles = articles || message.Done > 0
			}
			cleanup = cleanup || message.Stage == "cleanup"
		case "done":
			completed = true
			if len(message.Failed) != 0 {
				t.Fatal(message.Failed)
			}
		}
	}
	if !dictionary || !articles || !cleanup || !completed {
		t.Fatalf("missing events: dictionary=%v articles=%v cleanup=%v completed=%v", dictionary, articles, cleanup, completed)
	}
}

func TestBackgroundPreparationReusesAndDeduplicates(t *testing.T) {
	for _, rebuild := range []bool{false, true} {
		t.Run(map[bool]string{false: "reuse", true: "rebuild"}[rebuild], func(t *testing.T) {
			s := newTestServer(t)
			dir := preparedDSL(t, s)
			e := s.reg.entryAt(store.TextDBPath(dir))
			e.closeNow()
			before, err := os.ReadFile(store.TextDBPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			old := filepath.Join(store.DefaultDBDir(), "old-background-copy")
			if err := os.Mkdir(old, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.TextDBPath(old), before, 0644); err != nil {
				t.Fatal(err)
			}
			if err := store.WriteInfo(old); err != nil {
				t.Fatal(err)
			}
			plan := store.KeptPlan(store.TextDBPath(dir))
			if rebuild {
				plan.Contains = !plan.Contains
			}
			if err := e.setIndexRemoved(true); err != nil {
				t.Fatal(err)
			}
			if err := s.reg.prefs.mutate(func(f *prefsFile) {
				if f.DSLPending == nil {
					f.DSLPending = make(map[string]dslIndexOptions)
				}
				f.DSLPending[e.indexRemovalKey()] = dslIndexOptions{Index: true, Contains: plan.Contains, FullText: plan.FullText}
			}); err != nil {
				t.Fatal(err)
			}
			s.reg.prepareNewDSL()
			if e.indexBlocked() {
				t.Fatal("background preparation did not finish")
			}
			if _, err := os.Stat(old); !os.IsNotExist(err) {
				t.Fatalf("duplicate survived: %v", err)
			}
			after, err := os.ReadFile(store.TextDBPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			if !rebuild && !bytes.Equal(before, after) {
				t.Fatal("ready index was rebuilt")
			}
			if got := store.KeptPlan(store.TextDBPath(dir)); got != plan {
				t.Fatalf("plan: %+v want %+v", got, plan)
			}
		})
	}
}

func updateIndexesReq(t *testing.T, s *Server, body rescanIndexesRequest) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/rescan", strings.NewReader(string(data)))
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var report struct {
		Failed []string `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) > 0 {
		t.Fatal(report.Failed)
	}
}

func TestRescanIndexActionsPreserveExactFeatures(t *testing.T) {
	for _, base := range []string{"keep", "recreate"} {
		for _, action := range []string{"keep", "update", "delete"} {
			for _, present := range []bool{false, true} {
				t.Run(base+"/"+action+"/"+map[bool]string{false: "absent", true: "present"}[present], func(t *testing.T) {
					s := newTestServer(t)
					dir := preparedDSL(t, s)
					e := s.reg.entryAt(store.TextDBPath(dir))
					if err := e.setFeatures(features{Contains: present, FullText: present}, nil); err != nil {
						t.Fatal(err)
					}
					updateIndexesReq(t, s, rescanIndexesRequest{New: &dslIndexOptions{Index: true}, Existing: rescanIndexActions{Index: base, Contains: action, FullText: action}})
					check := func() {
						t.Helper()
						have := store.KeptPlan(store.TextDBPath(dir))
						want := present && action != "delete"
						if have.Contains != want || have.FullText != want {
							t.Fatalf("features: %+v want %v", have, want)
						}
					}
					check()
					if err := s.reg.Rescan(); err != nil {
						t.Fatal(err)
					}
					waitDSLDefaults(t, s.reg)
					check()
				})
			}
		}
	}
}

func TestRescanNewDictionariesUseOnlySelectedIndexes(t *testing.T) {
	for _, contains := range []bool{false, true} {
		for _, fts := range []bool{false, true} {
			t.Run(map[bool]string{false: "no-contains", true: "contains"}[contains]+"/"+map[bool]string{false: "no-fts", true: "fts"}[fts], func(t *testing.T) {
				isolatedDBDir(t)
				dir := t.TempDir()
				state := filepath.Join(t.TempDir(), StateFile)
				reg, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
				if err != nil {
					t.Fatal(err)
				}
				s := New(reg)
				// Previously queued defaults must not leak into the requested result.
				if err := reg.prefs.mutate(func(f *prefsFile) {
					f.DSLDefaults = &dslDefaults{Original: dslIndexOptions{Index: true, Contains: true, FullText: true}}
				}); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "new.dsl"), []byte(sampleDSL), 0600); err != nil {
					t.Fatal(err)
				}
				updateIndexesReq(t, s, rescanIndexesRequest{New: &dslIndexOptions{Index: true, Contains: contains, FullText: fts}, Existing: rescanIndexActions{Index: "keep", Contains: "keep", FullText: "keep"}})
				check := func(r *Registry) {
					t.Helper()
					for _, e := range r.all() {
						_, ok := store.LookupDir(e.Path)
						selected := e.dslVariant == "original"
						if ok != selected {
							t.Fatalf("variant %s prepared=%v", e.dslVariant, ok)
						}
						if selected {
							path, _ := store.LookupDir(e.Path)
							have := store.KeptPlan(store.TextDBPath(path))
							if have.Contains != contains || have.FullText != fts {
								t.Fatalf("unexpected features: %+v", have)
							}
						}
					}
				}
				check(reg)
				restarted, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
				if err != nil {
					t.Fatal(err)
				}
				waitDSLDefaults(t, restarted)
				check(restarted)
				if err := os.WriteFile(filepath.Join(dir, "later.dsl"), []byte(sampleDSL), 0600); err != nil {
					t.Fatal(err)
				}
				if err := restarted.Rescan(); err != nil {
					t.Fatal(err)
				}
				waitDSLDefaults(t, restarted)
				check(restarted)
			})
		}
	}
}

func TestRescanKeepDoesNotRestoreRemovedBase(t *testing.T) {
	s := newTestServer(t)
	state := filepath.Join(t.TempDir(), StateFile)
	s.reg.prefs = LoadPrefs(state)
	preparedDSL(t, s)
	id := idOf(t, s, "test.dsl")
	if _, err := s.reg.Remove(id, true, false); err != nil {
		t.Fatal(err)
	}
	updateIndexesReq(t, s, rescanIndexesRequest{New: &dslIndexOptions{Index: true, Contains: true, FullText: true}, Existing: rescanIndexActions{Index: "keep", Contains: "update", FullText: "update"}})
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.LookupDir(e.Path); ok || !e.indexBlocked() {
		t.Fatal("removed base was restored")
	}
	restarted, err := NewRegistry(s.reg.Dirs(), false, WithPrefs(LoadPrefs(state)), WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitDSLDefaults(t, restarted)
	for _, other := range restarted.all() {
		if _, ok := store.LookupDir(other.Path); ok || !other.indexBlocked() {
			t.Fatal("restart restored a removed base")
		}
	}
	updateIndexesReq(t, s, rescanIndexesRequest{New: &dslIndexOptions{Index: true}, Existing: rescanIndexActions{Index: "recreate", Contains: "update", FullText: "update"}})
	dir, ok := store.LookupDir(e.Path)
	if !ok || e.indexBlocked() {
		t.Fatal("explicit recreate did not restore base")
	}
	if plan := store.KeptPlan(store.TextDBPath(dir)); plan.Contains || plan.FullText {
		t.Fatal("update created missing optional indexes")
	}
}

func TestRescanDeletesUnselectedPreparedVariant(t *testing.T) {
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
	var unwanted string
	for _, e := range reg.all() {
		if _, err := e.open(); err != nil {
			t.Fatal(err)
		}
		if e.dslVariant == "gd" {
			path, _ := store.LookupDir(e.Path)
			unwanted = path
			if err := e.setIndexRemoved(true); err != nil {
				t.Fatal(err)
			}
		}
	}
	updateIndexesReq(t, s, rescanIndexesRequest{New: &dslIndexOptions{Index: true}, Existing: rescanIndexActions{Index: "keep", Contains: "keep", FullText: "keep"}})
	if _, err := os.Stat(unwanted); !os.IsNotExist(err) {
		t.Fatalf("unselected variant's data remains: %v", err)
	}
}
