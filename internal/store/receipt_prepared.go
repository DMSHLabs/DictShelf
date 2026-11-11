// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"strconv"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// A prepared folder answers a LIST ROW from its receipt, not from its database.
//
// /api/dicts is asked once per page load (and once per Browse -> words click,
// which is a new document), and a row needs a handful of meta keys: the name,
// the counts, the plan, the version stamps Outdated compares, the source's
// cheap identity. Reading them from the database costs a SQLite open per
// dictionary - a millisecond on a desktop disk and tens of them on a phone's
// FUSE mount, which is the difference between a list that appears and one that
// takes seconds. The receipt already carries them (WriteInfo copies them from
// the meta), so a row that can be answered from it does not open anything.
//
// The receipt is a DERIVED file and is trusted exactly as receiptMeta trusts it:
// only while it is at least as new as the text.db it describes, and only when
// it carries the fields this build writes. Anything else - a folder prepared by
// an older build, a receipt whose write failed - answers false, and the caller
// reads the meta the slow way (Inspect).

// PreparedFromReceipt is Inspect answered from a folder's receipt.
func PreparedFromReceipt(dir string) (Prepared, bool) {
	textDB := TextDBPath(dir)
	db, err := os.Stat(textDB)
	if err != nil || db.IsDir() {
		return Prepared{}, false
	}
	receipt, err := os.Stat(InfoPath(dir))
	if err != nil || receipt.ModTime().Before(db.ModTime()) {
		return Prepared{}, false
	}
	info, err := readInfo(InfoPath(dir))
	if err != nil {
		return Prepared{}, false
	}
	// A receipt without the row fields predates them: no version stamp to judge,
	// no cheap identity to compare the source with.
	if info["ingest_version"] == "" || info["source_sha256_head"] == "" {
		return Prepared{}, false
	}
	meta := map[string]string{
		"name": info["name"], "format": info["format"],
		"entry_count": info["entries"], "headword_count": info["headword_count"],
		"source_path": info["source"], "source_size": info["source_size"],
		"source_mtime": info["source_mtime"], "source_revision": info["source_revision"],
		"source_sha256_1M": info["source_sha256_head"], "source_sha256_tail": info["source_sha256_tail"],
		"ingest_version": info["ingest_version"], "reader_version": info["reader_version"],
		"markup_version": info["markup_version"], "fold_version": info["fold_version"],
		"articles_split": info["articles_split"], "dict_uuid": info["uuid"],
		"created": info["imported"], "index_lang": info["language"],
		"contents_lang": info["contents_lang"],
		"index_kind":    info["index_kind"], "own_ft_index": info["own_ft_index"],
		"abbrev_size": info["abbrev_size"], "abbrev_sha256": info["abbrev_sha256"],
		"abbrev_mtime": info["abbrev_mtime"],
	}
	// The level and the trigram flag are the receipt's own spelling (index,
	// contains) rather than the meta's enumerated keys.
	if strings.HasPrefix(info["index"], "full text") {
		meta["ingest_level"] = levelText
	} else {
		meta["ingest_level"] = levelHeadwords
	}
	if info["contains"] == "1" {
		meta["has_trigram"] = "1"
	} else {
		meta["has_trigram"] = "0"
	}
	for k, v := range meta {
		if v == "" {
			delete(meta, k)
		}
	}
	// A receipt can only be written for a database this build read as its own
	// schema; an older one is caught by the version stamps in Outdated.
	p := Prepared{TextDB: textDB, Meta: meta, Schema: textSchemaMax, Plan: PlanFromMeta(meta)}
	// The same rule Inspect applies: a split index whose file is gone is no index,
	// so the row must not offer one.
	if p.Plan.FullText && !p.Plan.ArticlesInside {
		if _, ok := ArticlesSibling(textDB); !ok {
			p.Plan.FullText = false
		}
	}
	if art, ok := ArticlesSibling(textDB); ok {
		if fi, err := os.Stat(art); err == nil {
			p.FullTextSize = fi.Size()
		}
	}
	if sib := MediaSibling(textDB); sib != "" && fsx.FileExists(sib) {
		p.Media = true
	}
	return p, true
}

// FindPreparedFromReceipt is FindPrepared's cheap twin: the same folder and
// the same currency test on the source, answered from the receipt.
func FindPreparedFromReceipt(srcPath string) (Prepared, bool) {
	dir, ok := LookupDir(srcPath)
	if !ok {
		return Prepared{}, false
	}
	p, ok := PreparedFromReceipt(dir)
	if !ok || !p.Current(srcPath) {
		return Prepared{}, false
	}
	return p, true
}

// receiptCurrent reports that a receipt carries the fields this build writes.
// A receipt from before the marker is rewritten even when the fields it lacks
// are absent by nature for that dictionary (index_kind on a non-ZIM, say).
func receiptCurrent(info map[string]string) bool {
	n, err := strconv.Atoi(info["receipt_version"])
	return err == nil && n >= receiptVersion &&
		info["ingest_version"] != "" && info["source_sha256_head"] != ""
}

// UpgradeReceipts rewrites the receipts that predate the row fields, so the
// first listing of a library prepared by an older build pays the database read
// once instead of forever. Best-effort and idempotent: a receipt that already
// carries them is left alone, and a folder whose meta cannot be read is skipped
// (Inspect will answer for it as before). It returns how many were rewritten.
func UpgradeReceipts() int {
	folders, err := Folders()
	if err != nil {
		return 0
	}
	n := 0
	for _, f := range folders {
		info, err := readInfo(InfoPath(f.Dir))
		if err == nil && receiptCurrent(info) {
			continue
		}
		if err := WriteInfo(f.Dir); err == nil {
			n++
		}
	}
	return n
}
