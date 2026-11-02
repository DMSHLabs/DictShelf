// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// archiveSource is a dictionary that answers full text itself, which is what a
// Kiwix archive does through internal/format/zim/glass. It counts the calls so
// the test can tell "the archive answered" from "the database did".
type archiveSource struct {
	*fakeDict
	calls int
	body  string
}

func (a *archiveSource) FullText(query string, limit int) ([]dict.Result, error) {
	a.calls++
	return []dict.Result{{Headword: strings.ToLower(query), Body: a.body}}, nil
}

func init() {
	dict.RegisterFormat(".ownft", func(p string) (dict.Dictionary, error) {
		return &archiveSource{fakeDict: &fakeDict{words: []string{"beta"}, path: p}, body: "<p>from the archive</p>"}, nil
	})
	// The control: a source of the same shape that cannot answer full text, so
	// the fallback to the database is exercised without touching a real format's
	// files (a DSL source would be held open the way the janitor holds it, and
	// the test's temp directory would never be removable).
	dict.RegisterFormat(".noft", func(p string) (dict.Dictionary, error) {
		return &fakeDict{words: []string{"beta"}, path: p}, nil
	})
}

// ownFTFile writes a source file of the format above: enough for dict.Open to
// hand back a dictionary that answers full text.
func ownFTFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "own.ownft")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// recordedOwnFullText pokes the meta key into a prepared database, the way the
// ZIM reader records it at ingest: what the row and the routing both read.
func recordedOwnFullText(t *testing.T, textDB string) {
	t.Helper()
	db, err := sql.Open(sqliteDriver(), "file:"+textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// One connection, and it waits for a lock instead of failing on sight. This
	// database is live: the registry opens it at startup and then measures it,
	// and the measurement walks the whole file (SELECT ... FROM dbstat holds a
	// read lock while it does). Every connection the application opens carries a
	// busy timeout for exactly this reason (store's dsnRO/dsnWildcardCache); a
	// raw one would answer "database is locked (5)" the moment the pass and this
	// write overlap, which is what this test used to do about once in sixty runs
	// (-count=60).
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		t.Fatal(err)
	}
	// ingest_level=text is what an article index would say; the point of the
	// test is that full text is still advertised, because it comes from the
	// archive. The table itself is left alone - what the row reads is the meta.
	if _, err := db.Exec("UPDATE meta SET value = 'headwords' WHERE key = 'ingest_level'"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT OR REPLACE INTO meta(key, value) VALUES (?, '1')", store.OwnFullTextKey); err != nil {
		t.Fatal(err)
	}
}

// A prepared database whose archive carries its own full-text index advertises
// full text even though nothing in the database holds article text: Caps.FTS and
// the new ArchiveIndex both come from the meta the database was built with -
// while OurFullText, which is about an index of ours, stays false.
func TestOwnFullTextRowAndCaps(t *testing.T) {
	s := newTestServer(t)
	preparedDSL(t, s)
	id := getDicts(t, s, "/api/dicts")[0].ID
	textDB := getDicts(t, s, "/api/dicts")[0].DBPath
	if textDB == "" {
		t.Fatal("the fixture prepared no database")
	}
	recordedOwnFullText(t, textDB)
	s.dictRows.flush() // the row cache keys on stat, and an in-place write can hide

	row := getDicts(t, s, "/api/dicts")[0]
	if row.ID != id {
		t.Fatalf("row for %q came back as %q", id, row.ID)
	}
	if !row.ArchiveIndex {
		t.Error("a database recording its archive's own index did not report it")
	}
	if !row.Caps.FTS {
		t.Error("full text was not advertised, although the archive answers it")
	}
	if row.OurFullText {
		t.Error("this database holds no article index of ours, but claimed one")
	}
}

// The routing: full text goes to the archive when it has one, and to the
// database when it does not. What the archive answers end to end is the
// diagnostic tool's job on real archives; what is pinned here is that the
// handle asks the right one, and that a source which cannot answer is not
// mistaken for one that can.
func TestArchiveFullTextRouting(t *testing.T) {
	s := newTestServer(t)
	preparedDSL(t, s)
	textDB := getDicts(t, s, "/api/dicts")[0].DBPath
	if textDB == "" {
		t.Fatal("the fixture prepared no database")
	}
	recordedOwnFullText(t, textDB)
	st, err := store.Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	src := ownFTFile(t)
	u := &upgraded{Store: st, srcPath: src}
	t.Cleanup(func() { u.releaseSource() })
	if !u.Caps().FTS || !u.FullTextFromArchive() {
		t.Fatalf("caps = %+v, archive = %v", u.Caps(), u.FullTextFromArchive())
	}
	// The search skips its FTS5 rungs for this handle, so this reading must
	// answer nothing rather than answer an expression it cannot honour.
	if got, err := u.FullTextMatch(context.Background(), `"beta"`, 5); err != nil || len(got) != 0 {
		t.Errorf("FullTextMatch = %v, %v; want nothing and no error", got, err)
	}
	got, err := u.FullTextContext(context.Background(), "Beta", 5)
	if err != nil || len(got) != 1 || got[0].Body != "<p>from the archive</p>" {
		t.Fatalf("FullTextContext = %+v, %v; want the archive's own answer", got, err)
	}

	// The same handle with a source that cannot answer full text at all (any
	// format other than the archive): the database answers, exactly as before
	// this routing existed.
	noFT := filepath.Join(t.TempDir(), "plain.noft")
	if err := os.WriteFile(noFT, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fallback := &upgraded{Store: st, srcPath: noFT}
	t.Cleanup(func() { fallback.releaseSource() })
	if _, ok, err := fallback.archiveFullText(context.Background(), "beta", 5); ok || err != nil {
		t.Errorf("a source without a full-text index was treated as an archive that answers: ok=%v err=%v", ok, err)
	}
}
