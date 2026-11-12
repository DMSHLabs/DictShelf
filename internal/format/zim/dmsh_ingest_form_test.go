// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

// A fresh index-only build from a REAL archive must leave the body column empty.
//
// The phone carried a database whose meta said index_kind=headwords while all
// 589 871 rows held markup (4.18 GB) - built by a path that no longer exists.
// This fixture walks the production path (NewIndexReader -> store.IngestPlan)
// over a synthetic archive and pins the two facts that make that impossible
// now: the form says headwords, and not one row has a body.
func TestIndexReaderLeavesNoBodiesInTheDatabase(t *testing.T) {
	path := writeZIM(t, sample())
	r, err := NewIndexReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	textDB := filepath.Join(t.TempDir(), store.TextDBName)
	if _, err := store.IngestPlan(r, textDB, store.Plan{Contains: true, FullText: true}, nil); err != nil {
		t.Fatal(err)
	}

	p := store.Inspect(textDB)
	if got := p.Meta["index_kind"]; got != "headwords" {
		t.Fatalf("index_kind = %q, want headwords", got)
	}
	if got := p.Meta["stored_body_bytes"]; got != "0" {
		t.Fatalf("stored_body_bytes = %q, want 0", got)
	}

	db, err := sql.Open("sqlite3", "file:"+textDB+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var kept int
	if err := db.QueryRow(`SELECT count(*) FROM entry WHERE m IS NOT NULL AND length(m) > 0`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 0 {
		t.Fatalf("%d rows kept a body; the reader asked for none to be kept", kept)
	}
}