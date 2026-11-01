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

import (
	"context"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

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

// ArchiveStrict is implemented by a handle whose full text comes from an
// archive, for the two readings that index cannot answer by itself.
//
// It has no positions - libzim indexed the articles with
// index_text_without_positions - so "are these words adjacent, in this order"
// and "are they within ten of each other" cannot be asked of it. They are
// answered instead by verifying them in the article text of the candidates the
// index returns, bounded by maxCheck: a promise about what was checked, which
// is why the caller is told the count rather than handed a silent prefix.
type ArchiveStrict interface {
	FullTextFromArchive() bool
	ArchivePhrase(ctx context.Context, query string, limit, maxCheck int) ([]dict.Result, error)
	ArchiveNear(ctx context.Context, distance int, query string, limit, maxCheck int) ([]dict.Result, error)
}

// IsFullText reports whether a mode asks the article index a text question.
//
// The three readings are one family: each highlights the words it searched for,
// each is answered by the article full text, and each is the same kind of answer
// to the reader. A caller that checked "== FullText" to mean "mark the matches"
// stops marking anything the day a reading is added, so the check belongs here,
// where the family is declared.
func (m Mode) IsFullText() bool { return m == FullText || m == Phrase || m == Near }
