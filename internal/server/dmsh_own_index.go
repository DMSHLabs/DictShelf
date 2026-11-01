// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Full text that the SOURCE answers, not the prepared database.
//
// A Kiwix archive ships the full-text index it was built with, and this fork
// reads it directly (internal/format/zim/glass): about a third of the size of
// an index of ours on the same archive, and already built. For such an archive
// the prepared database is the headword index and nothing else, so a full-text
// query has to be routed to the archive - which is what this file does, in the
// handle the search actually holds.
//
// It is a pair of small additions to registry.go's delegating methods rather
// than a file of its own declarations, for the fork's usual reason: registry.go
// is upstream's and already declares the two methods that need the guard.
// Everything the guard DOES lives here, so a sync resolves upstream's side of
// those methods by re-adding one call at the top.

// Note what the routing also fixes. Without it, "full text" on a title-only ZIM
// means full text over TITLES: that is all a title-only database ever held, and
// the bodies were fetched afterwards from the archive (titleResults). With it,
// the query reaches the article text the archive indexed.

package server

import (
	"context"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// Caps advertises full text for a prepared database whose archive answers it.
// The store cannot know: it sees a title-only database with no article index,
// which is exactly what it is. What makes full text available is the source
// behind it, and only this handle can see that (store.OwnFullText, recorded in
// the database's meta when it was built).
func (u *upgraded) Caps() dict.Caps {
	c := u.Store.Caps()
	if u.Store.OwnFullText() {
		c.FTS = true
	}
	return c
}

// FullTextFromArchive satisfies search.ArchiveFullText, which is how the search
// learns to skip its FTS5 rungs for this handle (they are expressions composed
// by internal/ftsq, and the archive's index has no expression language - it
// takes words, in one reading).
func (u *upgraded) FullTextFromArchive() bool { return u.Store.OwnFullText() }

// archiveFullText answers a full-text query from the archive's own index. ok is
// false when this handle has no archive index and the caller should ask the
// database instead; an error with ok true means the archive was the intended
// answer and did not give one - reported rather than hidden, because an empty
// result would read as "nothing matched".
func (u *upgraded) archiveFullText(ctx context.Context, query string, limit int) (res []dict.Result, ok bool, err error) {
	if !u.Store.OwnFullText() {
		return nil, false, nil
	}
	src, err := u.source()
	if err != nil {
		return nil, true, err
	}
	searcher, has := src.(interface {
		FullText(string, int) ([]dict.Result, error)
	})
	if !has {
		// A source that cannot answer full text (another format, or some
		// future reader): fall back to the database rather than fail.
		return nil, false, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	res, err = searcher.FullText(query, limit)
	return res, true, err
}
