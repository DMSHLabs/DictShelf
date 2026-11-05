// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"os"

	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
	"github.com/DMSHLabs/DictShelf/internal/watchdog"
)

// The archive's read loops are ours, so they are watched for a stop in their
// movement (internal/watchdog; the requirement is indexconst's). Two surfaces,
// one per phase of a full-text read:
//
//   - "archive-index"   the walks over the archive's own Xapian index
//     (glass.walkPostings): the record is the document id the walk is on
//   - "archive-records" the loops that turn the index's answer into articles -
//     resolving a ranked hit through the index's docdata, then reading and
//     decompressing the entry: the record is the document id, or the entry's
//     own index in the archive's directory where the loop has only that
//
// A phase ends its watch before the next phase starts: a finished loop that
// keeps a watch registered would look like a stopped one while the next phase
// runs, and the next phase's own loop is what should be named.
//
// The size is the ARCHIVE's, the file the reader thinks in, not the index
// window's - the index is a fraction of the archive and a stall is a fact about
// the file the reader has.
func (d *Dict) watch(surface string) *watchdog.Watch {
	desc := watchdog.Desc{Surface: surface, Name: d.meta.Name, Path: d.meta.Path}
	if st, err := os.Stat(d.meta.Path); err == nil {
		desc.Size = st.Size()
	}
	return watchdog.Start(desc)
}

// searchIndex runs one query against the archive's own index under the index
// watch. The hook is set for exactly this call and cleared after it, because
// the database carries no idea of who is reading it, and the walk reports
// movement through it (glass.DB.Progress).
func (d *Dict) searchIndex(db *glass.DB, query string) (glass.Result, error) {
	watch := d.watch("archive-index")
	db.Progress = func(docid uint32) { watch.Moved(int64(docid)) }
	res, err := db.Search(query)
	db.Progress = nil
	watch.Done()
	return res, err
}
