// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"bytes"
	"fmt"
)

// Posting is one document that contains a term, with the term's frequency in it.
type Posting struct {
	DocID uint32
	WDF   uint32
}

// Term is a term's posting list as far as this reader could read it. Complete
// says the list was read whole: the last chunk was reached and it held as many
// documents as the index says the term has. Xapian splits a long list across
// several table entries, so a reader that stops at the first one silently
// returns half an answer.
type Term struct {
	Postings []Posting
	// DocFreq is what the index records for the term (documents), CollFreq the
	// total number of occurrences.
	DocFreq  uint32
	CollFreq uint32
	Complete bool
}

// DocLenTerm is the name of the postlist that holds document lengths: Xapian
// stores them as a postlist with an EMPTY term name, whose key is the two bytes
// 00 e0 and whose "frequency" per document is the length of that document
// (common/pack.h, pack_glass_postlist_key; glass_postlist.cc, get_doclength,
// which reads the wdf of that list). Ranking needs it, and it is a term like
// any other to this reader, apart from the name.
const DocLenTerm = "\x00\xe0"

// chunkTermKey is what the key of a chunk holds before the document id: the
// term and a zero byte (common/pack.h, pack_glass_postlist_key). Any key of a
// term's postlist that is not the first chunk starts with it, which is how a
// walk can tell the list has ended and the next term has begun.
func chunkTermKey(term string) []byte {
	key := make([]byte, 0, len(term)+1)
	key = append(key, term...)
	return append(key, 0)
}

// Lookup reads a term's posting list. The term must be exactly what the index
// stores: lowercase, unaccented and stemmed with the archive's own stemmer
// (stem.go; HANDOFF.md, "The glass format").
func (db *DB) Lookup(term string) (Term, error) {
	var out Term
	docFreq, collFreq, complete, err := db.walkPostings(term, chunkTermKey(term), func(docid, wdf uint32) bool {
		out.Postings = append(out.Postings, Posting{DocID: docid, WDF: wdf})
		return true
	})
	if err != nil {
		return Term{}, err
	}
	out.DocFreq, out.CollFreq, out.Complete = docFreq, collFreq, complete
	return out, nil
}

// walkPostings walks a term's posting list, calling fn for every posting in
// ascending document order until the list ends or fn returns false.
//
// A common term does not fit in one table entry, so Xapian breaks its list into
// chunks of roughly CHUNKSIZE bytes and gives each its own key - the first the
// term itself, the rest the term plus the chunk's first document id
// (glass_postlist.cc, PostlistChunkWriter::append and next_chunk). The chunk
// header says whether it is the last and how far it reaches; the entries follow
// as a first frequency and then a gap and a frequency for every further
// document, each gap counted from the document before it.
//
// It is a walk rather than a slice for the two callers that need one: ranking
// reads document lengths in step with its own hits and stops at the last of
// them, and a search never wants 1.4M postings materialised just to sort fifty.
//
// chunkPrefix is what the later chunks are keyed by before the document id.
// For a term it is the term and a zero byte; for the document-length list - a
// postlist of a term whose name is EMPTY - it is the two bytes of that key
// alone, with no separator, because there is no term name to terminate
// (common/pack.h, pack_glass_postlist_key).
//
// complete reports that the walk reached the end of the list the index
// describes, rather than stopping because the caller asked it to, because a
// chunk's bytes ran out, or because the next key belonged to another term.
func (db *DB) walkPostings(term string, chunkPrefix []byte, fn func(docid, wdf uint32) bool) (docFreq, collFreq uint32, complete bool, err error) {
	tag, found, err := db.keyTag(TablePostlist, []byte(term))
	if err != nil || !found {
		return 0, 0, false, err
	}
	// Every variable the chunk loop touches is declared here on purpose: a :=
	// inside the loop would shadow one of them, and a shadowed p or tag reads
	// the second chunk from the first chunk's position.
	var (
		p       int
		did     uint32
		lastDid uint32
		wdf     uint32
		span    uint32
		key     = []byte(term)
	)
	if docFreq, p, err = unpackUint(tag, p); err != nil {
		return 0, 0, false, fmt.Errorf("glass: term %q: document frequency: %w", term, err)
	}
	if collFreq, p, err = unpackUint(tag, p); err != nil {
		return 0, 0, false, fmt.Errorf("glass: term %q: collection frequency: %w", term, err)
	}
	// The first document of the first chunk, stored one less than it is. (The
	// document-length list leaves this field zeroed; the walk does not size
	// anything from it, and the first chunk's own header corrects it.)
	if did, p, err = unpackUint(tag, p); err != nil {
		return 0, 0, false, fmt.Errorf("glass: term %q: first document: %w", term, err)
	}
	did++
	for {
		var last bool
		if last, span, p, err = chunkHeader(tag, p); err != nil {
			return 0, 0, false, fmt.Errorf("glass: term %q: chunk: %w", term, err)
		}
		lastDid = did + span
		if wdf, p, err = unpackUint(tag, p); err != nil {
			return 0, 0, false, fmt.Errorf("glass: term %q: first frequency: %w", term, err)
		}
		for {
			if !fn(did, wdf) {
				return docFreq, collFreq, false, nil
			}
			if did >= lastDid {
				break
			}
			if p >= len(tag) {
				// The chunk says it reaches further than its bytes do. Stop
				// rather than fail: what was read is usable, as long as the
				// caller knows it is not all of it.
				return docFreq, collFreq, false, nil
			}
			var gap uint32
			if gap, p, err = unpackUint(tag, p); err != nil {
				return docFreq, collFreq, false, nil
			}
			did += gap + 1
			if wdf, p, err = unpackUint(tag, p); err != nil {
				return docFreq, collFreq, false, nil
			}
		}
		if last {
			return docFreq, collFreq, true, nil
		}
		// The rest of the list is in the next table entries that belong to this
		// term, each tagged with the document it starts at. They are not
		// adjacent documents: the chunk after this one starts at the next
		// document that holds the term, and the walk is the only thing that
		// knows where that is.
		nextKey, nextTag, next, err := db.nextKey(TablePostlist, key)
		if err != nil {
			return 0, 0, false, fmt.Errorf("glass: term %q: next chunk: %w", term, err)
		}
		if !next || !bytes.HasPrefix(nextKey, chunkPrefix) {
			return docFreq, collFreq, false, nil // ends earlier than its last chunk claims
		}
		if did, _, err = unpackPreservingSort(nextKey, len(chunkPrefix)); err != nil {
			return 0, 0, false, fmt.Errorf("glass: term %q: next chunk start: %w", term, err)
		}
		key, tag, p = nextKey, nextTag, 0
	}
}

// chunkHeader reads the start of a chunk: whether it is the last one, and how
// far past its first document it reaches. The flag is a byte, '0' or '1'
// (common/pack.h, pack_bool).
func chunkHeader(tag []byte, p int) (last bool, span uint32, next int, err error) {
	if p >= len(tag) {
		return false, 0, p, fmt.Errorf("the header runs past the end of the tag")
	}
	switch tag[p] {
	case '0':
		last = false
	case '1':
		last = true
	default:
		return false, 0, p, fmt.Errorf("the last-chunk flag is %q, neither '0' nor '1'", tag[p])
	}
	p++
	if span, p, err = unpackUint(tag, p); err != nil {
		return false, 0, p, fmt.Errorf("chunk end: %w", err)
	}
	return last, span, p, nil
}

// DocPath returns the article a document id stands for, as the index stores it.
// The docdata table is keyed by the id in sort-preserving form and holds one
// record per document; libzim writes the article's path there with its "C/"
// namespace (src/writer/xapianWorker.cpp).
func (db *DB) DocPath(docid uint32) (string, error) {
	tag, found, err := db.Find(TableDocdata, packPreservingSort(docid))
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("glass: document %d has no data", docid)
	}
	return string(tag), nil
}
