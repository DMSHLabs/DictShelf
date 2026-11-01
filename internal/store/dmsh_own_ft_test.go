// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"testing"

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
