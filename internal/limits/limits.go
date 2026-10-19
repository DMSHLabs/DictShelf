// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package limits holds the bounds on how much a dictionary file or an import
// may make wudict allocate or write: the guards against a corrupt or hostile
// file whose size field, compressed block or archive entry claims more than
// any real dictionary needs. They are the MAX_* config keys (internal/config),
// set once at startup and read where each bound applies.
//
// Each value is a ceiling, not a target: nothing is allocated up front, and a
// real dictionary stays orders of magnitude below every default here.
package limits

import (
	"fmt"
	"strconv"
	"sync/atomic"
)

// Limits is one value per guard. A field that is zero or negative stands for
// its default (Set), so a guard can be raised or lowered but never switched
// off.
type Limits struct {
	ZIMCluster    int64 // MAX_ZIM_CLUSTER: one ZIM cluster, decompressed
	SlobBin       int64 // MAX_SLOB_BIN: one slob bin, decompressed
	MDictBlock    int64 // MAX_MDICT_BLOCK: one MDict key or record block, decompressed
	StarDictIndex int64 // MAX_STARDICT_INDEX: a gzipped StarDict index, decompressed (floor; 16x the file counts too)
	BGLBlock      int64 // MAX_BGL_BLOCK: one Babylon block
	LemmaSize     int64 // MAX_LEMMA_SIZE: one lemma data file, decompressed
	ImportSize    int64 // MAX_IMPORT_SIZE: one dictionary extracted from an archive or downloaded
	ImportRatio   int64 // MAX_IMPORT_RATIO: unpacked to packed size of one archive entry
	UploadSize    int64 // MAX_UPLOAD_SIZE: one archive uploaded to the server
}

// Default is every guard as wudict ships it.
var Default = Limits{
	ZIMCluster:    256 << 20,
	SlobBin:       256 << 20,
	MDictBlock:    256 << 20,
	StarDictIndex: 256 << 20,
	BGLBlock:      64 << 20,
	LemmaSize:     64 << 20,
	ImportSize:    256 << 30,
	ImportRatio:   2000,
	UploadSize:    16 << 30,
}

var cur atomic.Pointer[Limits]

func init() { Set(Default) }

// Get returns the limits in effect.
func Get() Limits { return *cur.Load() }

// Set replaces the limits in effect; a field that is zero or negative takes
// its default.
func Set(l Limits) {
	pick := func(v, d int64) int64 {
		if v > 0 {
			return v
		}
		return d
	}
	d := Default
	l = Limits{
		ZIMCluster:    pick(l.ZIMCluster, d.ZIMCluster),
		SlobBin:       pick(l.SlobBin, d.SlobBin),
		MDictBlock:    pick(l.MDictBlock, d.MDictBlock),
		StarDictIndex: pick(l.StarDictIndex, d.StarDictIndex),
		BGLBlock:      pick(l.BGLBlock, d.BGLBlock),
		LemmaSize:     pick(l.LemmaSize, d.LemmaSize),
		ImportSize:    pick(l.ImportSize, d.ImportSize),
		ImportRatio:   pick(l.ImportRatio, d.ImportRatio),
		UploadSize:    pick(l.UploadSize, d.UploadSize),
	}
	cur.Store(&l)
}

// Exceeded is the error a guard returns: what was refused, the key that sets
// the bound, and the bound - so the message names the setting a user would
// change for a dictionary they trust.
func Exceeded(what, key string, limit int64) error {
	return fmt.Errorf("%s exceeds the %s limit of %s", what, key, Format(limit))
}

// Format renders a value the way the config reads it back: a size in the
// largest unit that divides it exactly ("256MB", "16GB"), else a plain number
// (a ratio, or bytes). Zero and below are "0".
func Format(n int64) string {
	if n <= 0 {
		return "0"
	}
	for _, u := range []struct {
		mult int64
		name string
	}{{1 << 40, "TB"}, {1 << 30, "GB"}, {1 << 20, "MB"}, {1 << 10, "KB"}} {
		if n%u.mult == 0 {
			return strconv.FormatInt(n/u.mult, 10) + u.name
		}
	}
	return strconv.FormatInt(n, 10)
}
