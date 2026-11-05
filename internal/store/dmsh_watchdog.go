// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"

	"github.com/DMSHLabs/DictShelf/internal/watchdog"
)

// The store's loops that stream a scan of the dictionary's own rows are watched
// for a stop in their movement (internal/watchdog; the requirement is
// indexconst's). What the record is depends on the loop, and each one names it:
//
//   - "article-ids"       the entry id being visited
//   - "article-id-lookup" ids resolved
//   - "browse-headwords"  headword rows scanned
//   - "browse-alphabet"   headword rows scanned
//   - "headword-export"   headwords written
//   - "entry-scan"        the entry id being visited
//
// What is watched is the loop that reads the rows, never a statement whose whole
// result arrives before the loop starts: the MATCH behind an article result is
// opaque and has no progress to watch (indexconst). A bounded read is not
// watched either - a Browse page, an article portion and a lookup by key are a
// statement each - and neither is an indexing job, which carries its own
// progress and its own Stop.
//
// The size is the prepared database's own, not the source's: it is the file the
// loop reads, and the file a stall is a fact about.
func (s *Store) watch(surface string) *watchdog.Watch {
	desc := watchdog.Desc{Surface: surface, Name: s.meta.Name, Path: s.meta.Path}
	if st, err := os.Stat(s.meta.Path); err == nil {
		desc.Size = st.Size()
	}
	return watchdog.Start(desc)
}
