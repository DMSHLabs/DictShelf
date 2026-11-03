// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
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
	if _, err := IngestPlan(r, textDB, Plan{Contains: true, FullText: true}, nil); err != nil {
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

// The split read path: the article index lives in the sibling file, and the
// reader answers out of it through its own connection - no ATTACH, because
// ATTACH belongs to one connection and this process pools them without a limit
// (the file's header has the argument). What the test insists on is
// equivalence: a split database answers exactly as one that keeps the index
// inside, for all three readings the reader has.
func TestSplitArticlesAnswersLikeAnUnsplittedOne(t *testing.T) {
	entries := []dict.Entry{h("alpha", "<p>one two</p>"), h("beta", "<p>three two</p>"), h("gamma", "<p>one three</p>")}
	build := func(dir string, inside bool) string {
		textDB := filepath.Join(dir, TextDBName)
		src := filepath.Join(dir, "source.dsl")
		if err := os.WriteFile(src, []byte(`#NAME t

word
`), 0o600); err != nil {
			t.Fatal(err)
		}
		r := &fakeReader{meta: dict.Meta{Name: "Split", Format: "dsl", Path: src}, entries: entries}
		if _, err := IngestPlan(r, textDB, Plan{Contains: true, FullText: true, ArticlesInside: inside}, nil); err != nil {
			t.Fatal(err)
		}
		return textDB
	}
	type answered struct {
		headwords []string
		ids       int
		matches   int
	}
	ask := func(textDB string) answered {
		s, err := Open(textDB)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if !s.HasArticleIndex() {
			t.Fatal("the reader does not see an article index")
		}
		ctx := context.Background()
		got, err := s.FullTextContext(ctx, "two", 10)
		if err != nil {
			t.Fatalf("full text: %v", err)
		}
		var out answered
		for _, r := range got {
			out.headwords = append(out.headwords, r.Headword)
		}
		sort.Strings(out.headwords)
		var ids []int64
		if err := s.StreamArticleIDs(ctx, "two", "fts", `"two"`, func(page []int64, total int, done bool) error {
			ids = append(ids, page...)
			return nil
		}); err != nil {
			t.Fatalf("stream: %v", err)
		}
		out.ids = len(ids)
		if m, err := s.FullTextMatch(ctx, `"two"`, 10); err != nil {
			t.Fatalf("match: %v", err)
		} else {
			out.matches = len(m)
		}
		return out
	}
	split := ask(build(t.TempDir(), true))
	whole := ask(build(t.TempDir(), false))
	if len(split.headwords) == 0 {
		t.Fatal("the split database answered nothing")
	}
	if !reflect.DeepEqual(split, whole) {
		t.Errorf("the split database answered %+v, the unsplit one %+v", split, whole)
	}
}

// Removing the full text from a split database is deleting one file, and the
// dictionary has to read as a dictionary without full text afterwards - not as
// one with an index that is missing.
func TestRemovingTheSplitSiblingEndsTheFullText(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := filepath.Join(dir, "source.dsl")
	if err := os.WriteFile(src, []byte(`#NAME t

word
`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fakeReader{meta: dict.Meta{Name: "Split", Format: "dsl", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>")}}
	if _, err := IngestPlan(r, textDB, Plan{FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ArticlesFilePath(textDB)); err != nil {
		t.Fatal(err)
	}
	s, err := Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.HasArticleIndex() {
		t.Error("the database claims an article index whose file is gone")
	}
	if _, err := s.FullTextContext(context.Background(), "two", 5); err == nil {
		t.Error("a full-text query answered from an index that is not there")
	}
	// The headwords are the point of the database and are untouched.
	got, err := s.ExactContext(context.Background(), "alpha", 5)
	if err != nil || len(got) != 1 {
		t.Errorf("the headword index did not survive the index's removal: %d results, %v", len(got), err)
	}
	if p := Inspect(textDB); p.Plan.FullText {
		t.Error("the prepared database still reports full text with its index file gone")
	}
}

// A loose database's article index carries the database's own name, the way
// media.db does: two loose databases copied into one directory must not come to
// share one index, and adoption must take the pair along.
func TestLooseArticleSiblingMirrorsItsName(t *testing.T) {
	dir := t.TempDir()
	if got, want := ArticlesFilePath(filepath.Join(dir, TextDBName)), filepath.Join(dir, ArticlesDBName); got != want {
		t.Errorf("bundle form: got %s, want %s", got, want)
	}
	if got, want := ArticlesFilePath(filepath.Join(dir, "some-dict.text.db")), filepath.Join(dir, "some-dict.articles.db"); got != want {
		t.Errorf("loose form: got %s, want %s", got, want)
	}
	if ArticlesFilePath(filepath.Join(dir, "a.text.db")) == ArticlesFilePath(filepath.Join(dir, "b.text.db")) {
		t.Error("two loose databases in one directory share one article index")
	}
	if got, want := ArticlesFilePath(filepath.Join(dir, "not-a-db")), filepath.Join(dir, ArticlesDBName); got != want {
		t.Errorf("neither shape: got %s, want %s", got, want)
	}
}

func TestAdoptionTakesTheLooseArticleIndexAlong(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WUDICT_DB_DIR", root)
	src := filepath.Join(root, "source.dsl")
	if err := os.WriteFile(src, []byte("#NAME t\n\nword\n\tbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(root, "Adopted-1234abcd.text.db")
	r := &fakeReader{meta: dict.Meta{Name: "Adopted", Format: "dsl", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>")}}
	if _, err := IngestPlan(r, loose, Plan{FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	if !fsx.FileExists(ArticlesFilePath(loose)) {
		t.Fatal("the loose build wrote no article index beside its database")
	}
	dir, err := adoptOne(loose)
	if err != nil {
		t.Fatal(err)
	}
	if dir == "" {
		t.Fatal("the loose database was not adopted")
	}
	if !fsx.FileExists(filepath.Join(dir, ArticlesDBName)) {
		t.Error("adoption moved the database without its article index")
	}
	if fsx.FileExists(ArticlesFilePath(loose)) {
		t.Error("the loose article index was left behind")
	}
	s, err := Open(TextDBPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.HasArticleIndex() {
		t.Error("the adopted database lost its full text")
	}
}

// The attached article temp is a throwaway exactly like the main one: no
// rollback journal, no synchronous commit. The pragmas that say so are per
// DATABASE, so a build that sets them on the main temp only leaves the largest
// file it writes - the article index of a read-through archive - journaled.
func TestTheAttachedArticleFileIsAThrowawayToo(t *testing.T) {
	dir := t.TempDir()
	main, side := filepath.Join(dir, TextDBName), filepath.Join(dir, ArticlesDBName)
	db, err := sql.Open(driverName, dsnIngest(main))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := createArticlesDB(db, side, "uuid-1"); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.QueryRow("PRAGMA articles.journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mode, "off") {
		t.Errorf("the attached temp is journaled: journal_mode=%q, want off", mode)
	}
	var sync int
	if err := db.QueryRow("PRAGMA articles.synchronous").Scan(&sync); err != nil {
		t.Fatal(err)
	}
	if sync != 0 {
		t.Errorf("the attached temp syncs its commits: synchronous=%d, want 0 (OFF)", sync)
	}
	var cacheKiB int
	if err := db.QueryRow("PRAGMA articles.cache_size").Scan(&cacheKiB); err != nil {
		t.Fatal(err)
	}
	if cacheKiB != -ingestCacheKiB {
		t.Errorf("the attached temp has cache_size=%d, want %d (the ingest cache)", cacheKiB, -ingestCacheKiB)
	}
	// Writing to it - inside a transaction, which is what a build does for
	// minutes - must leave no journal beside it.
	if _, err := db.Exec(`CREATE VIRTUAL TABLE articles.article_fts USING fts5(
		w, txt, content='', columnsize=0, tokenize='unicode61 remove_diacritics 2')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO articles.article_fts(rowid, w, txt) VALUES (1, 'alpha', 'one two')`); err != nil {
		t.Fatal(err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*-journal")); len(matches) != 0 {
		t.Errorf("a rollback journal appeared for the attached temp: %v", matches)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// A split build records the article index's real size. The measurement is the
// one place where "which database am I looking at" can go wrong silently: a
// walk that reads the main database's pages while filtering for the article
// table's finds nothing, and the dictionary list then shows no index size at
// all - which is exactly what an attempt to measure through articles.dbstat on
// the writing connection did, on a SQLite build where that schema-qualified
// table enumerates the MAIN database.
func TestASplitBuildRecordsTheArticleIndexSize(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := writeSrc(t, filepath.Join(dir, "source.dsl"), "#NAME t\n\nword\n\tbody\n")
	var entries []dict.Entry
	for i := 0; i < 40; i++ {
		entries = append(entries, h(fmt.Sprintf("word%02d", i), "<p>one two three four five six seven eight</p>"))
	}
	r := &fakeReader{meta: dict.Meta{Name: "Sized", Format: "dsl", Path: src}, entries: entries}
	if _, err := IngestPlan(r, textDB, Plan{FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	side, ok := ArticlesSibling(textDB)
	if !ok {
		t.Fatal("the build did not split the article index")
	}
	recorded, err := strconv.ParseInt(Inspect(textDB).Meta["full_text_index_bytes"], 10, 64)
	if err != nil || recorded <= 0 {
		t.Fatalf("the build recorded full_text_index_bytes=%q, want the index's real size", Inspect(textDB).Meta["full_text_index_bytes"])
	}
	if !dbstatAvailable(t, textDB) {
		return // this build cannot measure it; the recorded number is all we have
	}
	sideDB, err := openRO(side)
	if err != nil {
		t.Fatal(err)
	}
	defer sideDB.Close()
	if measured := sizeOfTable(context.Background(), sideDB, "article_fts"); measured != recorded {
		t.Errorf("the build recorded %d bytes, the article index measures %d", recorded, measured)
	}
}
