// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// A dictionary whose full text is answered by its SOURCE rather than by an
// index of ours.
//
// The case is a Kiwix archive: it ships the full-text index it was built with,
// this fork reads it (internal/format/zim/glass), and the archive is about a
// third of the size of the index we would build on the same text. For such a
// dictionary full-text search works with nothing built at all, which is why the
// capability is advertised (dict.Caps.FTS) and why the routing has to know
// where the answer comes from.
//
// The interface lives here instead of in search.go (upstream's) for the fork's
// usual reason - it is a fork-only contract between the search and the handle
// that serves the archive, and nothing about upstream's search depends on it.

package search

// ArchiveFullText is implemented by a dictionary handle whose full-text index
// belongs to the source. A handle that does not implement it - which is every
// ordinary prepared database - is unaffected: the search keeps its FTS5 rungs.
type ArchiveFullText interface {
	// FullTextFromArchive reports whether a full-text query should be sent to
	// FullTextContext directly, skipping the query's relaxation rungs. Those
	// rungs are FTS5 expressions composed by internal/ftsq (a phrase, a
	// proximity, a bag of words), and an archive's index has no expression
	// language: it takes words, in ONE reading. Asking for the rungs anyway
	// would spend three queries to get the answer the fourth call returns.
	FullTextFromArchive() bool
}
