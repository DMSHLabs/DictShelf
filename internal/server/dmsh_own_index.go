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
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	zimformat "github.com/DMSHLabs/DictShelf/internal/format/zim"
	"github.com/DMSHLabs/DictShelf/internal/indexconst"
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

// noteArchiveFullText fills the row's ArchiveFullText for a ZIM this fork would
// now prepare as the title index: its archive answers full text and the database
// still holds an index of ours.
//
// The probe opens the archive's container (a header read and a directory
// lookup), which is why it is asked only where the answer can change anything:
// once a database has been converted, own_ft_index is in its meta and the row
// knows without opening anything. The row cache absorbs the rest.
func noteArchiveFullText(info *dictInfo) {
	if info == nil || info.OwnFullText || info.Error != "" || !strings.EqualFold(info.Format, "zim") {
		return
	}
	info.ArchiveFullText = archiveAnswersFullText(info.Path)
}

// archiveAnswersFullText reports whether the archive itself carries the
// full-text index, which is what decides the form of the prepared database the
// ingest builds: the read-through form exists so that article text can reach an
// index of ours, and an archive like this needs no such index (see the comment
// on the decision in registry.go).
//
// It opens the archive's container - the header, the MIME list and a directory
// lookup - and nothing else: no article is decompressed, and the index itself
// is not read.
func archiveAnswersFullText(path string) bool {
	own, err := zimformat.HasOwnFullTextIndex(path)
	return err == nil && own
}

// archiveArticleIDs enumerates the whole answer for the result cache - the list
// behind "N+ results" and Browse, which pages through every match instead of
// the first few.
//
// The store's own implementation reads the ids from SQLite, which a database
// built from an archive does not have for full text; this answers from the
// archive instead, keeping the archive's rank order. That order is the same one
// the interactive search produced, because both ask the same reader the same
// question - which is what lets page two continue page one rather than reshuffle
// it.
//
// Batching matches the store's: ids are looked up in batches of a few hundred
// (one statement each), and emitted in pages of indexconst.BrowsePageSize, with
// the running total and the final done flag, exactly as the SQLite path emits
// them. A headword this database does not hold is dropped - by choice, not by
// accident: it means the archive's index names an entry the prepared database
// never had, and inventing an id for it is not possible.
//
// A source that cannot enumerate its answer reports ErrUnsupported rather than
// an empty list: the caller treats the two differently, and "no results" would
// be a lie about a search that has plenty.
func (u *upgraded) archiveArticleIDs(ctx context.Context, query string, emit func([]int64, int, bool) error) error {
	src, err := u.source()
	if err != nil {
		return err
	}
	streamer, ok := src.(interface {
		StreamFullTextHeadwords(string, func(string) error) error
	})
	if !ok {
		return dict.ErrUnsupported
	}
	var (
		pending []string
		ids     []int64
		total   int
	)
	// flush moves the headwords gathered so far through the id lookup and into
	// the cache, leaving whatever does not fill a page for the next call.
	flush := func(done bool) error {
		if len(pending) > 0 {
			got, err := u.Store.EntryIDsForHeadwords(ctx, pending)
			if err != nil {
				return err
			}
			ids = append(ids, got...)
			pending = pending[:0]
		}
		for len(ids) >= indexconst.BrowsePageSize || (done && len(ids) > 0) {
			n := min(indexconst.BrowsePageSize, len(ids))
			total += n
			chunk, rest := ids[:n], ids[n:]
			ids = rest
			if err := emit(chunk, total, false); err != nil {
				return err
			}
		}
		if done {
			return emit(nil, total, true)
		}
		return nil
	}
	err = streamer.StreamFullTextHeadwords(query, func(headword string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		pending = append(pending, headword)
		if len(pending) >= 4*indexconst.BrowsePageSize {
			return flush(false)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return flush(true)
}
