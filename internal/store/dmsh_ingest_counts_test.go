// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// What the panel and the dictionary-information window print is what the ingest
// COUNTED, not what the source declared: articles are the entries a reader can
// open, headwords are those plus every alias pointing at one. The fixture
// exercises all three ways a headword is added - a second headword on an entry,
// a redirect that resolves, and a redirect that resolves to nothing and is
// dropped - and declares an EntryCount that is deliberately wrong, so a
// regression to "trust the header" cannot pass by luck.
func TestIngestRecordsCountedHeadwordsAndArticles(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	r := &fakeReader{
		meta: dict.Meta{Name: "Counts", Format: "dsl", Path: filepath.Join(dir, "counts.dsl"), EntryCount: 99},
		entries: []dict.Entry{
			h("alpha", "<p>one</p>"),
			{Headwords: []string{"beta", "second"}, Body: "<p>two</p>", Kind: dict.BodyHTML},
			{Headwords: []string{"gamma"}, LinkTo: "alpha"},
			{Headwords: []string{"delta"}, LinkTo: "nowhere"},
		},
	}
	if _, err := IngestPlan(r, textDB, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	meta, err := ReadMeta(textDB)
	if err != nil {
		t.Fatal(err)
	}

	// Asserted against the tables a reader will query, not against the ingest's
	// own bookkeeping: the two must not be able to drift apart.
	db, err := openRO(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var entries, aliases int64
	if err := db.QueryRow("SELECT count(*) FROM entry").Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM alias").Scan(&aliases); err != nil {
		t.Fatal(err)
	}
	if entries != 2 || aliases != 2 {
		t.Fatalf("fixture: %d entries and %d aliases, want 2 and 2", entries, aliases)
	}
	if got, want := meta["entry_count"], fmt.Sprint(entries); got != want {
		t.Errorf("entry_count = %q, want %q (the articles)", got, want)
	}
	if got, want := meta["headword_count"], fmt.Sprint(entries+aliases); got != want {
		t.Errorf("headword_count = %q, want %q (articles + aliases, not the declared 99)", got, want)
	}
	if got, want := meta["sub_entries"], "0"; got != want {
		t.Errorf("sub_entries = %q, want %q (no @-prefixed entry here)", got, want)
	}
}
