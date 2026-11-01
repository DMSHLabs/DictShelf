// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// A dictionary whose full text comes from its archive hands back headwords, and
// a page of them still has to become entry ids. Three things have to hold, and
// each of them is a way the paged list could go wrong: the ORDER is the rank
// order that was given (or page two would reshuffle page one), an article named
// twice appears once (or the list would show it twice), and a headword this
// database does not hold is dropped rather than mapped to somebody else.
func TestEntryIDsForHeadwordsKeepsOrderAndDropsTheUnknown(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := filepath.Join(dir, "source.dsl")
	if err := os.WriteFile(src, []byte("#NAME test\n\nword\n\tbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fakeReader{meta: dict.Meta{Name: "Order", Format: "dsl", Path: src},
		entries: []dict.Entry{h("beta", "<p>one</p>"), h("alpha", "<p>two</p>"), h("gamma", "<p>three</p>")}}
	if _, err := IngestPlan(r, textDB, Plan{Contains: true}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ids := map[string]int64{}
	db, err := sql.Open(driverName, "file:"+textDB+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT w, id FROM entry")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var w string
		var id int64
		if err := rows.Scan(&w, &id); err != nil {
			t.Fatal(err)
		}
		ids[w] = id
	}
	rows.Close()

	got, err := s.EntryIDsForHeadwords(context.Background(), []string{"gamma", "nothing-here", "beta", "gamma", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{ids["gamma"], ids["beta"], ids["alpha"]}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v (the unknown dropped, the duplicate once)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v (order must be the one given)", got, want)
		}
	}
	if empty, err := s.EntryIDsForHeadwords(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Errorf("no headwords = %v, %v; want nothing and no error", empty, err)
	}
}
