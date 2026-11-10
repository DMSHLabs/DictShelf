// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// Resolving a source to its prepared folder: by claim first, by content after.
//
// A folder's receipt names the path it was prepared from. That claim is an
// optimisation and a provenance record, never an identity: it is an absolute
// path of the machine that wrote it, so a library carried to a phone, a
// dictionary moved to another folder and a renamed file all leave a perfectly
// usable database "unowned" - and an unowned database is one the app rebuilds
// from scratch, minutes per dictionary, while the copy it could have used sits
// beside it.
//
// The identity is the source's CONTENT, which the preparing machine recorded in
// the database: its size, and the hashes of its first and last MiB (ingest.go).
// For the formats this app reads the tail is a checksum of the whole file -
// gzip's CRC32 and ISIZE, a ZIM's MD5 - so the pair checks the content rather
// than sampling it (fsx.HashTail). Adoption only re-points the receipt: the
// databases are not touched, so nothing is re-indexed.
//
// Two levels, and the order is what keeps the read paths cheap: the claim costs
// one stat per candidate name, and the scan runs only when no claim names the
// file, and only over folders no file of this machine claims.

// The receipt's record of an adoption: the source a folder it was not prepared
// from is now claimed for, and what it claimed before. History, not ownership -
// the receipt's `source` line remains the claim. Carried across regenerations
// like the keep marker, and dropped when a rebuild re-claims the folder.
const (
	adoptedKey     = "adopted"
	adoptedFromKey = "adopted_from"
	adoptedValue   = "by content"
)

const adoptedComment = "# adopted by content: these databases were prepared from the same file\n" +
	"# under another path, so nothing was re-indexed.\n"

// ResolveDir returns the library folder prepared for a source file: the folder
// whose receipt claims it, or - when no claim names this file - the folder
// prepared from the same content under another path.
//
// This is the resolution the preparation decisions use (Reconcile, the
// new-dictionary queue, rescan). LookupDir stays the read-path question, "is
// there a folder this source owns", because it is one stat and the listing asks
// it for every dictionary.
func ResolveDir(srcPath string) (string, bool) {
	if dir, ok := LookupDir(srcPath); ok {
		return dir, true
	}
	return adoptPrepared(srcPath)
}

// adoptPrepared is ResolveDir's second level: the folder whose recorded source
// signature is the file at srcPath, re-claimed for it.
func adoptPrepared(srcPath string) (string, bool) {
	st, err := os.Stat(srcPath)
	if err != nil || !st.Mode().IsRegular() {
		return "", false
	}
	folders, err := Folders()
	if err != nil || len(folders) == 0 {
		return "", false
	}
	var head, tail string
	hashed := false
	for _, f := range folders { // name order, so two copies of one library resolve the same way twice
		if !claimAbsent(f.Source) {
			continue
		}
		size, wantHead, wantTail, ok := folderSignature(f.Dir)
		if !ok || size != st.Size() {
			continue
		}
		if !hashed {
			head, tail, hashed = fsx.HashSample(srcPath), fsx.HashTail(srcPath), true
		}
		if head == "" || head != wantHead {
			continue
		}
		// A folder prepared before the tail was recorded carries the head
		// alone; adopting it is what writes the missing half (writeInfo).
		if wantTail != "" && wantTail != tail {
			continue
		}
		if err := writeInfo(f.Dir, srcPath); err != nil {
			continue
		}
		markAdopted(f.Dir, f.Source)
		IndexDiagnostic("adopted prepared folder by content folder=%q source=%q was=%q",
			filepath.Base(f.Dir), srcPath, f.Source)
		return f.Dir, true
	}
	return "", false
}

// claimAbsent reports that a folder's claim cannot name a file of this machine:
// it is empty, it vanished, or it is the absolute path of another OS. A live
// claim is another dictionary's, and taking it would leave that one unowned, so
// only a folder nobody here can reach is adoptable.
func claimAbsent(src string) bool {
	if src == "" || fsx.ForeignAbs(src) {
		return true
	}
	_, err := os.Stat(src)
	return errors.Is(err, fs.ErrNotExist)
}

// folderSignature is what a folder says about the source it was prepared from:
// the size, and the hashes of that source's head and tail. The receipt answers
// (it is the library's fast source index), and a folder this build has
// regenerated carries both hashes there; a receipt written before that falls
// back to the database's meta once, which adopting it rewrites.
func folderSignature(dir string) (size int64, head, tail string, ok bool) {
	if info, err := readInfo(InfoPath(dir)); err == nil {
		if n, err := strconv.ParseInt(info["source_size"], 10, 64); err == nil && info["source_sha256_head"] != "" {
			return n, info["source_sha256_head"], info["source_sha256_tail"], true
		}
	}
	meta, err := ReadMeta(TextDBPath(dir))
	if err != nil {
		return 0, "", "", false
	}
	n, err := strconv.ParseInt(meta["source_size"], 10, 64)
	if err != nil || meta["source_sha256_1M"] == "" {
		return 0, "", "", false
	}
	return n, meta["source_sha256_1M"], meta["source_sha256_tail"], true
}

// markAdopted appends the adoption record to a folder's receipt. Best-effort:
// the claim is the ownership, and a receipt that cannot be appended to must not
// undo an adoption that already happened.
func markAdopted(dir, was string) {
	f, err := os.OpenFile(InfoPath(dir), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(f, "\n%s%s = %s\n%s = %s\n", adoptedComment, adoptedKey, adoptedValue, adoptedFromKey, was)
	_ = f.Close()
}
