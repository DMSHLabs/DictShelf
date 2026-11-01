// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// dropBodyReader is a reader whose bodies exist to be indexed and not to be
// kept - what internal/format/zim's IndexReader is for an archive that has no
// index of its own.
type dropBodyReader struct{ *fakeReader }

func (r *dropBodyReader) DropBodies() bool { return true }

func (r *dropBodyReader) ExtraMeta() map[string]string {
	return map[string]string{"index_kind": "headwords"}
}

// The article text is indexed and the bodies are not written: the point of the
// third form, and the reason it is worth having. The form has to remain
// derivable from the meta too, or a later rebuild would read the database as the
// title form and quietly drop the index.
func TestIndexOnlyReaderKeepsTheIndexAndNotTheBodies(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := filepath.Join(dir, "source.zim")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &dropBodyReader{&fakeReader{meta: dict.Meta{Name: "Indexonly", Format: "zim", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>"), h("beta", "<p>three two</p>")}}}
	if _, err := IngestPlan(r, textDB, Plan{Contains: true, FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	got, err := s.FullTextContext(context.Background(), "two", 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("the index did not answer: %d results, %v", len(got), err)
	}

	db, err := sql.Open(driverName, "file:"+textDB+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var withBodies int
	if err := db.QueryRow(`SELECT count(*) FROM entry WHERE m IS NOT NULL AND length(m) > 0`).Scan(&withBodies); err != nil {
		t.Fatal(err)
	}
	if withBodies != 0 {
		t.Errorf("%d entries kept a body; the reader asked for none to be kept", withBodies)
	}

	meta, err := ReadMeta(textDB)
	if err != nil {
		t.Fatal(err)
	}
	headwords, articles := ZIMForm(meta)
	if !headwords || articles {
		t.Errorf("the form reads as headwords=%v articles=%v; want the title form", headwords, articles)
	}
	if !PlanFromMeta(meta).FullText {
		t.Error("the meta does not say the article text is indexed, so a rebuild would drop it")
	}
}

// The receipt says which of the three forms the folder holds, because it is
// what a reader looks at when the source is long gone: "articles + search
// index" would send them looking for article text that this form deliberately
// does not keep, and "headwords + search index" would hide the article index
// that is there.
func TestReceiptNamesTheIndexOnlyForm(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, TextDBName)
	src := filepath.Join(dir, "source.zim")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &dropBodyReader{&fakeReader{meta: dict.Meta{Name: "Indexonly", Format: "zim", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two</p>")}}}
	if _, err := IngestPlan(r, textDB, Plan{Contains: true, FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	if err := WriteInfo(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(InfoPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if want := "# files: " + TextDBName + " (headwords + article index (article text stays in the archive))"; !strings.Contains(string(data), want) {
		t.Errorf("the receipt does not name the form; want %q in:\n%s", want, data)
	}
}
