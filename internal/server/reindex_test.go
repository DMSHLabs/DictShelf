// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
)

// bumpReader makes every dictionary of format prepared so far read as built by
// an older Reader - what an upgrade that changes that Reader does to a library.
func bumpReader(t *testing.T, format string) {
	t.Helper()
	old := dict.ReaderVersion(format)
	dict.RegisterReaderVersion(format, old+1)
	t.Cleanup(func() { dict.RegisterReaderVersion(format, old) })
}

func reindexCall(t *testing.T, s *Server, method string) (int, ReindexStatus) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest(method, "/api/reindex", nil))
	var st ReindexStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("%s /api/reindex: bad JSON (%v): %s", method, err, rec.Body.String())
	}
	return rec.Code, st
}

// The whole Rebuild: an outdated dictionary is reported, rebuilt by the job,
// and comes out current with the indexes and packed media it had - the plan is
// the user's, and a rebuild they asked for to bring data current must not
// change it.
func TestReindexRebuildsOutdatedKeepingPlanAndMedia(t *testing.T) {
	s := newTestServer(t)
	id := getDicts(t, s, "/api/dicts")[0].ID
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	sse(t, s, "/api/ingest?dict="+id+"&fts=1&contains=1&media=1")
	want := features{FullText: true, Contains: true, Media: true}
	if f := s.currentFeatures(e); f != want {
		t.Fatalf("setup: %+v, want %+v", f, want)
	}
	if entryOutdated(e.Path) || getDicts(t, s, "/api/dicts")[0].Outdated {
		t.Fatal("setup: a freshly prepared dictionary must not be outdated")
	}
	if code, st := reindexCall(t, s, "POST"); code != 200 || st.Running || st.Total != 0 {
		t.Fatalf("nothing outdated: got %d %+v, want 200 and no job", code, st)
	}

	bumpReader(t, "dsl")
	if !entryOutdated(e.Path) {
		t.Fatal("a dictionary built by an older Reader must be outdated")
	}
	if !getDicts(t, s, "/api/dicts")[0].Outdated {
		t.Fatal("/api/dicts must report the outdated dictionary")
	}

	code, st := reindexCall(t, s, "POST")
	if code != 202 || st.Total != 1 {
		t.Fatalf("start: got %d %+v, want 202 over 1 dictionary", code, st)
	}
	waitUntil(t, "the rebuild to finish", func() bool { return !s.reindexStatus().Running })

	st = s.reindexStatus()
	if st.Done != 1 || len(st.Failed) != 0 || st.Canceled {
		t.Fatalf("finished: %+v, want 1 done, none failed", st)
	}
	textDB, ok := validPrepared(e.Path)
	if !ok {
		t.Fatal("the dictionary is no longer prepared")
	}
	if r := store.Stale(textDB, e.Path); len(r) != 0 {
		t.Fatalf("still stale after the rebuild: %v", r)
	}
	if f := s.currentFeatures(e); f != want {
		t.Errorf("after the rebuild: %+v, want %+v (the plan and the media are kept)", f, want)
	}
	if getDicts(t, s, "/api/dicts")[0].Outdated {
		t.Error("/api/dicts still reports it outdated")
	}
	if code, st := reindexCall(t, s, "POST"); code != 200 || st.Running {
		t.Errorf("second start: got %d %+v, want 200 and nothing running", code, st)
	}
}

// A source edited in place keeps its path and its library folder. "Rescan
// folders" must see the edit and let go of the backend opened against the old
// edition. DSL prepares on open, so the next open rebuilds it - with the plan
// the user chose (store.KeptPlan), not the default - and serves the new edition.
func TestRescanSeesEditedSource(t *testing.T) {
	s := newTestServer(t)
	id := getDicts(t, s, "/api/dicts")[0].ID
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	sse(t, s, "/api/ingest?dict="+id+"&fts=1&contains=1")
	if _, err := e.open(); err != nil {
		t.Fatal(err)
	}
	dir, ok := store.LookupDir(e.Path)
	if !ok {
		t.Fatal("setup: not prepared")
	}
	before, _ := store.ReadMetaValue(store.TextDBPath(dir), "dict_uuid")

	if err := os.WriteFile(e.Path, []byte(sampleDSL+"\nperro\n\tanimal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(3 * time.Second)
	if err := os.Chtimes(e.Path, future, future); err != nil {
		t.Fatal(err)
	}
	if !entryOutdated(e.Path) {
		t.Error("an edited source must be outdated until it is prepared again")
	}
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	if d, _, _ := entryState(e); d != nil {
		t.Fatalf("rescan kept the backend opened against the old edition: %T", d)
	}

	d, err := e.open()
	if err != nil {
		t.Fatal(err)
	}
	if hits, err := d.Exact("perro", 10); err != nil || len(hits) != 1 {
		t.Errorf("the new edition is not served: %d hits, %v", len(hits), err)
	}
	textDB := store.TextDBPath(dir)
	if after, _ := store.ReadMetaValue(textDB, "dict_uuid"); after == before {
		t.Error("the edited source was not prepared again")
	}
	if got := store.KeptPlan(textDB); got != (store.Plan{FullText: true, Contains: true}) {
		t.Errorf("re-prepared with %+v, want the plan the user chose", got)
	}
	if entryOutdated(e.Path) {
		t.Error("still outdated after it was prepared again")
	}
}
