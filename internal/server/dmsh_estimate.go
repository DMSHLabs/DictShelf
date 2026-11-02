// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/estimate"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// estimateView is what the full-text page shows for one dictionary: the band a
// build would add, and which measurement it came from. The page sums the rows
// the user ticks and says what the number is based on; it never invents one.
//
// An absent estimate means the index is already there (the row's caps say so),
// or nothing has been measured yet - the page measures on demand in that case
// and shows "Calculating" until it can show a band.
type estimateView struct {
	Low   int64  `json:"low"`
	High  int64  `json:"high"`
	Basis string `json:"basis"`
}

// attachEstimate adds the full-text band to a dictionary's list row.
//
// It runs after the row cache rather than inside it: the numbers come from the
// measurement cache beside the database (store.ReadStats), which changes on its
// own - a warm pass, a rebuild, a re-downloaded source - and a row cached
// before that would otherwise keep saying "unknown" until something unrelated
// invalidated the cache.
//
// Nothing here is authoritative about what is built: the caps on the row (from
// the plan the database was prepared with) decide whether a row is offered at
// all, and the build itself re-checks the real tables before it writes.
func (s *Server) attachEstimate(info *dictInfo) {
	if info == nil {
		return
	}
	// The index sizes first, and before every guard below: they are
	// measurements, they come from the cache the registry's background pass
	// fills (never from walking pages here), and a dictionary that already has
	// our index is exactly the one whose size a reader wants to see - so this
	// must not sit behind the "no estimate needed" return.
	if info.DBPath != "" {
		if stats, ok := store.ReadStats(info.DBPath); ok {
			if stats.ContainsIndexBytes > 0 {
				info.ContainsSize = stats.ContainsIndexBytes
			}
			if stats.FullTextIndexBytes > 0 {
				info.FullTextSize = stats.FullTextIndexBytes
			}
		}
	}
	if info.Error != "" || info.DBPath == "" || info.OurFullText || info.Job != nil {
		return
	}
	fi, err := os.Stat(info.DBPath)
	if err != nil {
		return
	}
	stats, _ := store.ReadStats(info.DBPath)
	zim := strings.EqualFold(info.Format, "zim")
	in := estimate.Inputs{
		ZIM:             zim,
		BodiesPresent:   zim && stats.ZIMForm == "articles",
		Entries:         int64(info.Entries),
		HeadwordBytes:   stats.HeadwordBytes,
		BodyBytes:       stats.BodyBytes,
		StoredBodyBytes: stats.StoredBodyBytes,
		DBBytes:         fi.Size(),
		ZIMArticles:     stats.ZIMArticles,
		ZIMTextBytes:    stats.ZIMTextBytes,
		ArchiveIndex:    info.ArchiveIndex,
		HasFullText:     info.OurFullText,
	}
	got := in.FullText()
	if got.Basis == estimate.BasisUnknown || got.Basis == estimate.BasisBuilt {
		return
	}
	info.Estimate = &estimateView{Low: got.Low, High: got.High, Basis: string(got.Basis)}
}
