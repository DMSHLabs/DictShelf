// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

// fullTextHits is what a full-text query answers for one prepared database, as
// a list of headwords.
func fullTextHits(t *testing.T, textDB, query string) []string {
	t.Helper()
	st, err := store.Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.FullTextContext(context.Background(), query, 20)
	if err != nil {
		t.Fatalf("full text %q: %v", query, err)
	}
	var out []string
	for _, r := range got {
		out = append(out, r.Headword)
	}
	return out
}

// fullTextFixture prepares the one test dictionary with full text and returns
// its folder, its text.db and the article index file.
func fullTextFixture(t *testing.T, s *Server) (dir, textDB, index string) {
	t.Helper()
	dir = preparedDSL(t, s)
	e := s.reg.entryAt(store.TextDBPath(dir))
	if err := e.setFeatures(features{Contains: true, FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	textDB = store.TextDBPath(dir)
	index = store.ArticlesFilePath(textDB)
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("the fixture has no article index of its own: %v", err)
	}
	return dir, textDB, index
}

// The panel's "update" of the full-text index rebuilds that index, and nothing
// else: the headword tables and the contains index are the ingest's own work
// and stay as they are, and the database's plan does not move.
func TestFullTextUpdateRebuildsTheIndexOnly(t *testing.T) {
	s := newTestServer(t)
	_, textDB, index := fullTextFixture(t, s)
	before, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	want := fullTextHits(t, textDB, "muscular")
	if len(want) == 0 {
		t.Fatal("the fixture's full text answers nothing before the update")
	}

	updateIndexesReq(t, s, rescanIndexesRequest{
		New:      &indexOptions{},
		Existing: rescanIndexActions{Index: "keep", Contains: "keep", FullText: "update"},
	})

	after, err := os.Stat(index)
	if err != nil {
		t.Fatalf("the update left no article index: %v", err)
	}
	if !after.ModTime().After(before.ModTime()) {
		t.Error("the article index was not rebuilt at all")
	}
	if p := store.Inspect(textDB); !p.Plan.FullText || p.Plan.ArticlesInside || p.FullTextSize == 0 {
		t.Errorf("after the update the plan is %+v, article index %d bytes", p.Plan, p.FullTextSize)
	}
	if got := fullTextHits(t, textDB, "muscular"); !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt index answers %v, want %v", got, want)
	}
	st, err := store.Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if hits, err := st.ExactContext(context.Background(), "corazón", 5); err != nil || len(hits) != 1 {
		t.Errorf("the headword index did not survive the update: %d results, %v", len(hits), err)
	}
	if hits, err := st.ContainsContext(context.Background(), "oraz", 5); err != nil || len(hits) == 0 {
		t.Errorf("the contains index did not survive the update: %d results, %v", len(hits), err)
	}
}

// A rebuild of the article index needs no source: the index is a function of
// what text.db holds. The source is deleted before the request, so anything
// that tried to re-read it would fail - and the operation would fail with it.
//
// This drives the index-work path, the one that does not put an already
// prepared folder with no source up for removal halfway through.
func TestFullTextUpdateNeedsNoSource(t *testing.T) {
	s := newTestServer(t)
	defer s.jobs.wg.Wait()
	e := s.reg.all()[0]
	_, textDB, _ := fullTextFixture(t, s)
	want := fullTextHits(t, textDB, "muscular")
	if len(want) == 0 {
		t.Fatal("the fixture's full text answers nothing before the update")
	}
	if err := os.Remove(e.Path); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal([]indexOperation{{e.ID, "fts", "update"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DesktopJSON("http://127.0.0.1:6888", "startIndexWork", string(body)); err != nil {
		t.Fatal(err)
	}
	s.jobs.wait(bulkIndexKey)
	result := s.indexWorkStatus(bulkIndexKey)
	if len(result.Failed) > 0 || result.Done != 1 {
		t.Fatalf("the update reported %+v", result)
	}
	if got := fullTextHits(t, textDB, "muscular"); !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt index answers %v, want %v", got, want)
	}
	if p := store.Inspect(textDB); !p.Plan.FullText || p.FullTextSize == 0 {
		t.Errorf("the rebuilt index is not recorded: %+v, %d bytes", p.Plan, p.FullTextSize)
	}
}
