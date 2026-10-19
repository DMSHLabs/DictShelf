// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lemmas

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/wuweidict/wudict/internal/fsx"
)

// The catalogue a page can draw before the network answers. Reading it from
// its host takes a second or more on a phone, and the list is what the page is
// for; so the last catalogue read is kept on disk, and a copy is built into
// wudict for the time before the first read. Both are drawn, never installed
// from: an install reads the host again (internal/server), so a stale copy can
// show a language but never hand over a file the host no longer publishes.

// SnapshotURL is the catalogue built into wudict (snapshot.json): the default
// LEMMA_URL's manifest as it was when this build was made, refreshed with
// `make lemma-snapshot`.
const SnapshotURL = "https://raw.githubusercontent.com/wuweidict/lemmas/main/manifest.json"

//go:embed snapshot.json
var snapshot []byte

// Snapshot is the built-in catalogue when src is the one it was taken from,
// else nil: a LEMMA_URL pointed elsewhere publishes other languages.
func Snapshot(src string) *Catalog {
	if src != SnapshotURL {
		return nil
	}
	cat, err := Parse(snapshot, src)
	if err != nil {
		return nil // TestSnapshot holds the file to Parse
	}
	return cat
}

// CacheName is the file in LEMMA_DIR that keeps the last catalogue read from
// its host. morph.LemmaFile does not read it as lemma data, so Installed never
// lists it.
const CacheName = ".catalogue.json"

type cacheFile struct {
	Source  string          `json:"source"`
	Checked time.Time       `json:"checked"`
	Catalog json.RawMessage `json:"catalogue"`
}

// SaveCache keeps cat, read from src at checked, in dir.
func SaveCache(dir, src string, cat *Catalog, checked time.Time) error {
	body, err := json.Marshal(cat)
	if err != nil {
		return err
	}
	b, err := json.Marshal(cacheFile{Source: src, Checked: checked.UTC(), Catalog: body})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return fsx.WriteAtomic(filepath.Join(dir, CacheName), b, 0o644)
}

// LoadCache is the catalogue SaveCache kept for src and when it was read from
// there; nil when there is none, it was read from another src, or it no
// longer passes Parse.
func LoadCache(dir, src string) (*Catalog, time.Time) {
	b, err := fsx.ReadBounded(filepath.Join(dir, CacheName), 2*maxManifest)
	if err != nil {
		return nil, time.Time{}
	}
	var f cacheFile
	if json.Unmarshal(b, &f) != nil || f.Source != src {
		return nil, time.Time{}
	}
	cat, err := Parse(f.Catalog, src)
	if err != nil {
		return nil, time.Time{}
	}
	return cat, f.Checked
}
