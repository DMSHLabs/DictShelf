// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

// Document lengths, and the ranking that needs them.
//
// Xapian keeps every document's length in a postlist of its own, under an EMPTY
// term name: key 00 e0, then chunks keyed by the document each one starts at,
// and the "frequency" stored for a document IS its length
// (common/pack.h pack_glass_postlist_key; glass_postlist.cc get_doclength,
// which returns the wdf of that list). Ranking needs it because BM25 normalises
// by document length - without that, a list article that repeats a word a
// hundred times outranks the article that is actually about it, which is
// exactly what the length factor is for.
//
// This list is the one postlist that cannot be read with the term walk, for a
// reason visible in Xapian's writer: the first chunk is a DUMMY - zero entries,
// zero frequency, first document 1 - and it is flagged as the last chunk even
// though the real chunks follow it (glass_postlist.cc, merge_doclen_changes
// creates it "to ensure there's a first chunk"). A reader that trusts the
// flag, as the term walk does, sees an empty list and a search that quietly
// loses its length normalisation. So the walk here follows the keys instead,
// which is also what Xapian does when it seeks the list by document id.

// docLenPrefix is the key prefix all of the length list lives under.
var docLenPrefix = []byte(DocLenTerm)

// DocLengths reads the whole document-length list, ascending by document id.
//
// It is exported for the diagnostic tool and for tests. The sum of what it returns
// equals the total document length the version file records, which is how a test
// knows the walk was right rather than merely short (a search walks the list in
// tandem with its hits instead of calling this, because a 3.2M-document archive
// would be a 15 MB slice to sort fifty results).
func (db *DB) DocLengths() (docids, lens []uint32, err error) {
	_, _, _, err = db.walkPostings(DocLenTerm, docLenPrefix, func(docid, length uint32) bool {
		docids = append(docids, docid)
		lens = append(lens, length)
		return true
	})
	if err != nil {
		return nil, nil, err
	}
	return docids, lens, nil
}

// assignDocLengths stamps every hit with its document length, walking the
// length list in step with the hits - both are ascending by document id, so one
// pass serves them all and it stops at the last hit rather than reading a
// 3.2M-document list to rank fifty results.
//
// A hit the list does not know keeps a length of zero, which the formula reads
// as the shortest possible document (its min_normlen clamp). That cannot happen
// in an index Xapian wrote - every document has a length - and if it somehow
// does, scoring it as short is the honest reading of "no length recorded".
func (db *DB) assignDocLengths(hits []Hit) error {
	next := 0
	_, _, _, err := db.walkPostings(DocLenTerm, docLenPrefix, func(docid, length uint32) bool {
		for next < len(hits) && hits[next].DocID < docid {
			next++
		}
		if next == len(hits) {
			return false
		}
		if hits[next].DocID == docid {
			hits[next].DocLen = length
			next++
		}
		return next < len(hits)
	})
	return err
}
