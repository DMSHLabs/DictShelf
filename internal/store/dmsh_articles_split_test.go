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

// A split database keeps its article index in a file of its own, and answers
// exactly as one that keeps it inside: that equivalence is the whole condition
// for old databases going on working, so it is what the test pins.
func TestSplitArticlesIndexInItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := filepath.Join(dir, "source.dsl")
	if err := os.WriteFile(src, []byte("#NAME t"+"\n\nword\n\tbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fakeReader{meta: dict.Meta{Name: "Split", Format: "dsl", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>"), h("beta", "<p>three two</p>")}}
	if _, err := IngestPlan(r, textDB, Plan{Contains: true, FullText: true, SplitArticles: true}, nil); err != nil {
		t.Fatal(err)
	}
	art := ArticlesFilePath(textDB)
	if fi, err := os.Stat(art); err != nil || fi.Size() == 0 {
		t.Fatalf("no article index beside the database: %v", err)
	}

	// The file is a valid index of the same articles: proven by attaching it to
	// a pinned connection, which is what the read path will do on every
	// connection once it can (see the note in store.go).
	db, err := sql.Open(driverName, "file:"+textDB+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "ATTACH DATABASE ? AS articles", art); err != nil {
		t.Fatal(err)
	}
	var hits int
	if err := conn.QueryRowContext(context.Background(),
		`SELECT count(*) FROM articles.article_fts WHERE article_fts MATCH ?`, `"two"`).Scan(&hits); err != nil {
		t.Fatalf("the split index did not answer: %v", err)
	}
	if hits != 2 {
		t.Errorf("the split index matched %d articles, want 2", hits)
	}
	_ = conn.Close()
	_ = db.Close()

	// Removing the full text removes the file, and nothing of the headword side.
	if err := ClearSearchIndexes(textDB, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(art); !os.IsNotExist(err) {
		t.Error("the article index file is still there")
	}
	after, err := Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	if after.Caps().FTS {
		t.Error("full text is still advertised after its index was removed")
	}
	if hits, err := after.ExactContext(context.Background(), "alpha", 5); err != nil || len(hits) == 0 {
		t.Errorf("the headword index went with it: %d results, %v", len(hits), err)
	}
	if _, err := os.Stat(textDB); err != nil {
		t.Errorf("the database itself is gone: %v", err)
	}
}
