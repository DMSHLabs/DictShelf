// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

// Whether a prepared database's SOURCE carries its own full-text index.
//
// A Kiwix archive ships the full-text index it was built with
// (X/fulltext/xapian), and this fork can search that index directly
// (internal/format/zim/glass) - it is 2.4-2.9x smaller than the one we would
// build, and on the same archive it is 518 MB against 1.29 GB. So for such an
// archive the prepared database is the title index and nothing else: no
// article_fts of ours, no copied article bodies, and full text still works.
//
// The fact is recorded in the database's own meta when it is built, because it
// is a fact about the source that does not change, and because the alternative
// - asking the archive - means opening a 48 GB container to draw a dictionary
// list row. The key is written by internal/format/zim (OwnIndexMetaKey, in
// dmsh_xapian.go) and read here; the two spellings have to agree.

// OwnFullTextKey is the meta key internal/format/zim writes for this fact.
const OwnFullTextKey = "own_ft_index"

// OwnFullText reports whether the database described by meta was built from a
// source that carries its own full-text index - so full text for it is answered
// from the source, and there is no article index of ours to build, remove or
// estimate. A database whose meta says nothing (every database built before
// this key existed, and every non-ZIM one) reports false.
func OwnFullText(meta map[string]string) bool { return meta[OwnFullTextKey] == "1" }

// OwnFullText reports whether this database was built from a source that
// answers full text itself. It is what the search reads to decide where a
// full-text query goes, and it costs nothing: the value came from the meta the
// database was opened with.
func (s *Store) OwnFullText() bool { return s.ownFullText }
