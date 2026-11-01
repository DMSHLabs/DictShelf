// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	zimformat "github.com/DMSHLabs/DictShelf/internal/format/zim"
)

// The key is written by internal/format/zim and read here, so the two spellings
// have to agree: a typo on either side would silently turn every archive's own
// index into "none" - which costs a 9 GB index built for nothing, and is
// invisible until somebody measures. The check lives in this package because it
// is the one that can import both.
func TestOwnFullTextKeyAgreesWithTheZIMWriter(t *testing.T) {
	if OwnFullTextKey != zimformat.OwnIndexMetaKey {
		t.Fatalf("the key this package reads (%q) is not the one the ZIM reader writes (%q)",
			OwnFullTextKey, zimformat.OwnIndexMetaKey)
	}
}

// A database with no such key reports false however it was built: every
// database written before this key existed, and every non-ZIM one.
func TestOwnFullTextReadsTheMeta(t *testing.T) {
	if OwnFullText(nil) || OwnFullText(map[string]string{}) {
		t.Error("a database with no meta recorded an archive index")
	}
	if OwnFullText(map[string]string{OwnFullTextKey: "0"}) {
		t.Error("a zero was read as a yes")
	}
	if !OwnFullText(map[string]string{OwnFullTextKey: "1", "index_kind": "headwords"}) {
		t.Error("the recorded fact was not read back")
	}
}

// A stats cache describes a database, and a rebuild into the other form leaves
// every other validity check passing: the uuid is the source's and the source
// has not changed. So the form itself has to be part of the check, or a ZIM
// rebuilt to the title index goes on being priced against article bodies it no
// longer stores.
func TestStatsCacheIsRefusedAfterAFormChange(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, "text.db")
	meta := map[string]string{
		"format": "zim", "index_kind": "articles", "ingest_level": "text",
		"dict_uuid": "u1", "source_size": "10", "source_mtime": "2020-01-01T00:00:00Z",
	}
	if err := writeMetaFixture(t, textDB, meta); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "a.zim")
	if err := os.WriteFile(src, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	mtime := fi.ModTime().UTC().Format(time.RFC3339)
	if err := writeMetaFixture(t, textDB, map[string]string{
		"source_path": src, "source_size": "10", "source_mtime": mtime,
	}); err != nil {
		t.Fatal(err)
	}
	if err := WriteStats(dir, Stats{Version: StatsVersion, IndexUUID: "u1",
		SourceSize: 10, SourceMTime: mtime, Entries: 1, ZIMForm: "articles"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadStats(textDB); !ok {
		t.Fatal("a cache that matches its database was refused")
	}
	// The same folder after a rebuild into the title form: same uuid, same
	// source, different form.
	if err := writeMetaFixture(t, textDB, map[string]string{"index_kind": "headwords", "ingest_level": "headwords"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadStats(textDB); ok {
		t.Error("a cache describing the other form was accepted")
	}
}

// writeMetaFixture creates a text.db holding only the meta rows given, which is
// all the stats cache reads.
func writeMetaFixture(t *testing.T, path string, meta map[string]string) error {
	t.Helper()
	db, err := sql.Open(driverName, "file:"+filepath.ToSlash(path))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT)"); err != nil {
		return err
	}
	for k, v := range meta {
		if _, err := db.Exec("INSERT OR REPLACE INTO meta(key, value) VALUES (?, ?)", k, v); err != nil {
			return err
		}
	}
	return nil
}
