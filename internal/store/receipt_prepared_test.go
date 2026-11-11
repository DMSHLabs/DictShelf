// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// keysOfInterest are what a list row reads out of a prepared dictionary.
var keysOfInterest = []string{
	"name", "format", "entry_count", "headword_count", "ingest_level",
	"has_trigram", "articles_split", "ingest_version", "reader_version",
	"markup_version", "fold_version", "source_size", "source_sha256_1M",
	"abbrev_size", "abbrev_sha256", "index_lang", "contents_lang",
}

// A fold is one thing however it is read: the receipt must answer exactly what
// the database answers for every field a row uses, or the row would change when
// the receipt happens to be fresh.
func TestPreparedFromReceiptMatchesTheDatabase(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	src := writeSrc(t, filepath.Join(t.TempDir(), "s.mdx"), "source bytes")
	dir := prepare(t, src, "S")
	fromReceipt, ok := PreparedFromReceipt(dir)
	if !ok {
		t.Fatal("a folder this build regenerated has no usable receipt")
	}
	direct := Inspect(TextDBPath(dir))
	if fromReceipt.Plan != direct.Plan {
		t.Errorf("plan: receipt %+v, database %+v", fromReceipt.Plan, direct.Plan)
	}
	for _, k := range keysOfInterest {
		if fromReceipt.Meta[k] != direct.Meta[k] {
			t.Errorf("%s: receipt %q, database %q", k, fromReceipt.Meta[k], direct.Meta[k])
		}
	}
	if fromReceipt.Media != direct.Media {
		t.Errorf("media: receipt %v, database %v", fromReceipt.Media, direct.Media)
	}

	// The same currency test FindPrepared applies, answered from the receipt.
	if _, ok := FindPreparedFromReceipt(src); !ok {
		t.Fatal("the source is not resolved to its folder from the receipt")
	}
}

// A receipt older than the database it describes, or one written before these
// fields existed, is not an answer: the row reads the meta instead.
func TestPreparedFromReceiptRefusesAStaleOrOldReceipt(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	src := writeSrc(t, filepath.Join(t.TempDir(), "s.mdx"), "source bytes")
	dir := prepare(t, src, "S")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(InfoPath(dir), old, old); err != nil {
		t.Fatal(err)
	}
	if _, ok := PreparedFromReceipt(dir); ok {
		t.Error("a receipt older than its database was trusted")
	}
	if _, ok := FindPreparedFromReceipt(src); ok {
		t.Error("the receipt answered for a source after refusing its own receipt")
	}
	// The slow twin answers, which is what the row falls back to.
	if _, ok := FindPrepared(src); !ok {
		t.Error("the database path did not answer after the receipt was refused")
	}

	// Back to a fresh receipt, then strip the row fields a pre-change receipt
	// does not have.
	if err := WriteInfo(dir); err != nil {
		t.Fatal(err)
	}
	if _, ok := PreparedFromReceipt(dir); !ok {
		t.Fatal("a regenerated receipt is not usable")
	}
	info, err := readInfo(InfoPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	delete(info, "ingest_version")
	var body string
	for k, v := range info {
		body += k + " = " + v + "\n"
	}
	if err := os.WriteFile(InfoPath(dir), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := PreparedFromReceipt(dir); ok {
		t.Error("a receipt without the row fields was trusted")
	}
}

// The pass that upgrades a library prepared by an older build: only the bare
// receipts are rewritten, and running it again does nothing.
func TestUpgradeReceiptsRewritesOnlyBareReceipts(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	srcA := writeSrc(t, filepath.Join(t.TempDir(), "a.mdx"), "a")
	fresh := prepare(t, srcA, "A")
	srcB := writeSrc(t, filepath.Join(t.TempDir(), "b.mdx"), "b")
	bare := prepare(t, srcB, "B")
	info, err := readInfo(InfoPath(bare))
	if err != nil {
		t.Fatal(err)
	}
	delete(info, "ingest_version")
	var body string
	for k, v := range info {
		body += k + " = " + v + "\n"
	}
	if err := os.WriteFile(InfoPath(bare), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(InfoPath(fresh))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := PreparedFromReceipt(bare); ok {
		t.Fatal("a bare receipt answered as if it carried the row fields")
	}
	if n := UpgradeReceipts(); n != 1 {
		t.Fatalf("upgraded %d receipts, want 1", n)
	}
	if _, ok := PreparedFromReceipt(bare); !ok {
		t.Error("the upgraded receipt is still not usable")
	}
	if after, err := os.Stat(InfoPath(fresh)); err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("a fresh receipt was rewritten: %v", err)
	}
	if n := UpgradeReceipts(); n != 0 {
		t.Errorf("a second pass rewrote %d receipts", n)
	}
}
