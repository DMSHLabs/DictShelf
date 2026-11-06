// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// Integration against a real LSD pair; skips unless WUDICT_TEST_LSD names the
// dictionary. The companion is not a second variable: the reader looks for
// `<stem>_abrv.lsd` beside the source, and the fixture follows that rule.
//
// What the test drives is exactly what the abbreviation signature exists for -
// how a later change to the companion is judged - and it does it on a COPY of
// the pair: these are the user's own files, and nothing here opens them for
// writing. This is the only place the LSD reader's `abbrev_sha256` is pinned;
// that package has no golden test.
func TestIntegrationRealPair(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	src := os.Getenv("WUDICT_TEST_LSD")
	if src == "" {
		t.Skip("WUDICT_TEST_LSD not set")
	}
	if _, err := os.Stat(src); err != nil {
		t.Skipf("%s not readable", src)
	}
	// The pair is copied under its own names, so the reader's own rule finds it.
	dir := t.TempDir()
	copyPath := func(from, to string) {
		b, err := os.ReadFile(from)
		if err != nil {
			t.Skipf("%s not readable: %v", from, err)
		}
		if err := os.WriteFile(to, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stem := src[:len(src)-len(filepath.Ext(src))]
	abrvSrc := stem + "_abrv.lsd"
	if _, err := os.Stat(abrvSrc); err != nil {
		t.Skipf("the companion %s is missing, and it is the point of this test", abrvSrc)
	}
	dict := filepath.Join(dir, filepath.Base(src))
	companion := filepath.Join(dir, filepath.Base(abrvSrc))
	copyPath(src, dict)
	copyPath(abrvSrc, companion)

	st, err := os.Stat(companion)
	if err != nil {
		t.Fatal(err)
	}
	hash := fsx.HashSample(companion)
	if hash == "" {
		t.Fatal("the companion must hash")
	}

	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	r, err := NewReader(dict)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "text.db")
	if _, err := store.IngestPlan(r, dbPath, store.Plan{}, nil); err != nil {
		r.Close()
		t.Fatal(err)
	}
	r.Close()

	meta, err := store.ReadMeta(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if meta["abbrev_path"] != companion {
		t.Fatalf("companion not recorded: %q", meta["abbrev_path"])
	}
	if meta["abbrev_size"] != strconv.FormatInt(st.Size(), 10) {
		t.Fatalf("size %q, want %d", meta["abbrev_size"], st.Size())
	}
	if meta["abbrev_sha256"] != hash {
		t.Fatalf("hash %q, want %q", meta["abbrev_sha256"], hash)
	}
	if meta["abbrev_count"] == "" || meta["abbrev_count"] == "0" {
		t.Fatalf("the companion was not absorbed: count %q", meta["abbrev_count"])
	}
	if store.AbbrevChanged(dbPath, companion) {
		t.Fatal("a freshly ingested pair must not read as stale")
	}

	// A touch is not a change: the content decides, and this is what stops a
	// copied-but-identical companion from re-ingesting the whole dictionary.
	later := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(companion, later, later); err != nil {
		t.Fatal(err)
	}
	if store.AbbrevChanged(dbPath, companion) {
		t.Error("a touched companion with the same content must not read as stale")
	}

	// An edit that keeps the size is a change, and only the hash can see it.
	b, err := os.ReadFile(companion)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)/2] ^= 0x20 // same length, different content; nothing parses it here
	if err := os.WriteFile(companion, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if !store.AbbrevChanged(dbPath, companion) {
		t.Error("a same-size edit of the companion must read as stale")
	}
}
