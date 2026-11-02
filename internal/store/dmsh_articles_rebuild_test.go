// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// fullTextHeadwords is what a full-text query answers, as a sorted list of
// headwords: the shape both the ingest's build and a rebuild have to produce.
func fullTextHeadwords(t *testing.T, textDB, query string) []string {
	t.Helper()
	s, err := Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.FullTextContext(context.Background(), query, 20)
	if err != nil {
		t.Fatalf("full text %q: %v", query, err)
	}
	var out []string
	for _, r := range got {
		out = append(out, r.Headword)
	}
	return out
}

// A full-text index rebuilt from the database alone answers as the ingest's own
// build did - with the source deleted, so nothing here can have re-read it.
func TestArticleIndexRebuildsFromTheDatabase(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := writeSrc(t, filepath.Join(dir, "source.dsl"), "#NAME t\n\nword\n\tbody\n")
	r := &fakeReader{meta: dict.Meta{Name: "Rebuilt", Format: "dsl", Path: src},
		entries: []dict.Entry{
			h("alpha", "<p>one two</p>"),
			h("beta", "<p>three two</p>"),
			h("gamma", "<p>four</p>"),
			h("delta", "<p>two four</p>"),
		}}
	if _, err := IngestPlan(r, textDB, Plan{FullText: true, Contains: true}, nil); err != nil {
		t.Fatal(err)
	}
	want := fullTextHeadwords(t, textDB, "two")
	if len(want) != 3 {
		t.Fatalf("the ingest's own index answers %v, want three articles", want)
	}
	before := Inspect(textDB)

	// Break the index - a sibling that is not a database at all is still a file
	// the reader will take for the answer - and take the source away, so a
	// re-ingest is not merely wasteful but impossible.
	if err := os.WriteFile(ArticlesFilePath(textDB), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if !ArticleIndexRebuildable(textDB) {
		t.Fatal("a split database that stores its bodies is not rebuildable")
	}
	if err := RebuildArticleIndexContext(context.Background(), textDB, nil); err != nil {
		t.Fatal(err)
	}

	got := fullTextHeadwords(t, textDB, "two")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt index answers %v, want %v", got, want)
	}
	after := Inspect(textDB)
	if !after.Plan.FullText || after.Plan.ArticlesInside {
		t.Errorf("after the rebuild the plan is %+v, want full text in a file of its own", after.Plan)
	}
	if after.FullTextSize == 0 {
		t.Error("the rebuilt index has no recorded size")
	}
	// Nothing else moved: the headword tables are the ones the ingest wrote, and
	// the contains index is untouched.
	if after.Meta["entry_count"] != before.Meta["entry_count"] || after.Meta["headword_count"] != before.Meta["headword_count"] {
		t.Errorf("the headword counts moved: %q/%q, want %q/%q",
			after.Meta["entry_count"], after.Meta["headword_count"], before.Meta["entry_count"], before.Meta["headword_count"])
	}
	s, err := Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if hits, err := s.ExactContext(context.Background(), "alpha", 5); err != nil || len(hits) != 1 {
		t.Errorf("the headword index did not survive the rebuild: %d results, %v", len(hits), err)
	}
	if hits, err := s.ContainsContext(context.Background(), "lph", 5); err != nil || len(hits) == 0 {
		t.Errorf("the contains index did not survive the rebuild: %d results, %v", len(hits), err)
	}
}

// Full text added to a database that never had it, from that database alone -
// and the recorded plan follows, which is what the reader reads.
func TestFullTextIsAddedFromTheDatabase(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := writeSrc(t, filepath.Join(dir, "source.dsl"), "#NAME t\n\nword\n\tbody\n")
	r := &fakeReader{meta: dict.Meta{Name: "Added", Format: "dsl", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>"), h("beta", "<p>three</p>")}}
	if _, err := IngestPlan(r, textDB, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	if Inspect(textDB).Plan.FullText {
		t.Fatal("the fixture was built with full text")
	}
	if !ArticleIndexRebuildable(textDB) {
		t.Fatal("a database that stores its bodies cannot have full text built from them")
	}
	if err := RebuildArticleIndexContext(context.Background(), textDB, nil); err != nil {
		t.Fatal(err)
	}
	p := Inspect(textDB)
	if !p.Plan.FullText || p.Plan.ArticlesInside {
		t.Errorf("the plan is %+v, want full text as a file of its own", p.Plan)
	}
	if got := fullTextHeadwords(t, textDB, "two"); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Errorf("full text answers %v, want alpha", got)
	}
}

// The two shapes that cannot be rebuilt from the database alone say so instead
// of writing something wrong.
func TestArticleIndexRebuildRefusesWhereItCannot(t *testing.T) {
	dir := t.TempDir()
	src := writeSrc(t, filepath.Join(dir, "source.dsl"), "#NAME t\n\nword\n\tbody\n")

	// 1. The index still lives inside text.db: taking it out is the migration,
	// and that one rewrites the file.
	inside := filepath.Join(t.TempDir(), TextDBName)
	r := &fakeReader{meta: dict.Meta{Name: "Inside", Format: "dsl", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>")}}
	if _, err := IngestPlan(r, inside, Plan{FullText: true, ArticlesInside: true}, nil); err != nil {
		t.Fatal(err)
	}
	if ArticleIndexRebuildable(inside) {
		t.Error("a database whose index lives inside itself claims to be rebuildable")
	}
	if err := RebuildArticleIndexContext(context.Background(), inside, nil); !errors.Is(err, ErrArticleNeedsReingest) {
		t.Errorf("inside: %v, want ErrArticleNeedsReingest", err)
	}
	if fsx.FileExists(ArticlesFilePath(inside)) {
		t.Error("a refused rebuild published a file anyway")
	}

	// 2. A reader that drops body columns: the article text is not in this
	// database at all, so there is nothing to index - the same answer its meta
	// gives, and the source path once the key is gone and the data is asked.
	dropped := filepath.Join(t.TempDir(), TextDBName)
	r2 := &bodyLessReader{&fakeReader{meta: dict.Meta{Name: "Dropped", Format: "dsl", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>"), h("beta", "<p>three two</p>")}}}
	if _, err := IngestPlan(r2, dropped, Plan{FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	if got := Inspect(dropped).Meta["stored_body_bytes"]; got != "0" {
		t.Fatalf("the body-less fixture recorded stored_body_bytes=%q, want 0", got)
	}
	if ArticleIndexRebuildable(dropped) {
		t.Error("a database that stores no bodies claims to be rebuildable")
	}
	if err := RebuildArticleIndexContext(context.Background(), dropped, nil); !errors.Is(err, ErrArticleNeedsReingest) {
		t.Errorf("body-less: %v, want ErrArticleNeedsReingest", err)
	}
	// The articles are still searchable: this is a refusal to REBUILD, not a
	// database that lost its index.
	if got := fullTextHeadwords(t, dropped, "two"); len(got) != 2 {
		t.Errorf("the body-less database answers %v, want both articles", got)
	}
	db, err := liveWrite(context.Background(), dropped)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM meta WHERE key='stored_body_bytes'`); err != nil {
		t.Fatal(err)
	}
	if !ArticleIndexRebuildable(dropped) {
		t.Error("with the key removed the shape is unknown, and a rebuild should be tried")
	}
	if err := RebuildArticleIndexContext(context.Background(), dropped, nil); !errors.Is(err, ErrArticleNeedsReingest) {
		t.Errorf("body-less, no key: %v, want ErrArticleNeedsReingest once the rows turned out empty", err)
	}
	if got := fullTextHeadwords(t, dropped, "two"); len(got) != 2 {
		t.Errorf("a refused rebuild left the index broken: %v", got)
	}
}

// bodyLessReader is a dictionary whose bodies stay in the source, which is what
// a ZIM reader that keeps its articles in the archive says (DropBodies).
type bodyLessReader struct{ *fakeReader }

func (b *bodyLessReader) DropBodies() bool { return true }

// A rebuild that is cancelled mid-flight publishes nothing and leaves the
// working index alone, and the one that runs reports the ingest's own unit.
func TestArticleIndexRebuildPublishesOnlyWhenWhole(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := writeSrc(t, filepath.Join(dir, "source.dsl"), "#NAME t\n\nword\n\tbody\n")
	// Past one batch, so the loop has a cancellation point inside it.
	entries := make([]dict.Entry, 0, articleIndexBatch+100)
	for i := 0; i < articleIndexBatch+100; i++ {
		entries = append(entries, h(fmt.Sprintf("word%06d", i), "<p>body two</p>"))
	}
	r := &fakeReader{meta: dict.Meta{Name: "Cancelled", Format: "dsl", Path: src}, entries: entries}
	if _, err := IngestPlan(r, textDB, Plan{FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	// A term every row carries, read back in rank order: the list the rebuilt
	// index has to reproduce, hit for hit and in the same order.
	want := fullTextHeadwords(t, textDB, "two")
	if len(want) < 20 {
		t.Fatalf("the fixture answers %d articles for a term every row carries", len(want))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := RebuildArticleIndexContext(ctx, textDB, func(done, total int) {
		if total != len(entries) {
			t.Errorf("progress says total=%d, want %d", total, len(entries))
		}
		cancel() // the user stops it while the index is being built
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled rebuild returned %v, want context.Canceled", err)
	}
	got := fullTextHeadwords(t, textDB, "two")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a cancelled rebuild changed the index: %d hits, want %d", len(got), len(want))
	}
	if _, err := os.Stat(ArticlesFilePath(textDB)); err != nil {
		t.Errorf("a cancelled rebuild removed the working index: %v", err)
	}

	var seen int
	if err := RebuildArticleIndexContext(context.Background(), textDB, func(done, total int) { seen++ }); err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Error("the rebuild reported no progress at all")
	}
	if got := fullTextHeadwords(t, textDB, "two"); !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt index answers %d hits, want %d", len(got), len(want))
	}
}
