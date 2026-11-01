// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import "fmt"

// Posting is one document that contains a term, with the term's frequency in it.
type Posting struct {
	DocID uint32
	WDF   uint32
}

// Term is a term's posting list as far as this reader can read it. Complete says
// whether every chunk was read: Xapian splits a long list into chunks that live
// in separate items of the same table, and following them needs a cursor this
// reader does not have yet, so a long list comes back as its first chunk only.
type Term struct {
	Postings []Posting
	// DocFreq is what the index records for the term (the number of documents),
	// CollFreq the total number of occurrences.
	DocFreq  uint32
	CollFreq uint32
	Complete bool
}

// Lookup reads a term's posting list. The term must be exactly what the index
// stores: for libzim that is the lowercase, unaccented word (HANDOFF.md, "The
// glass format"). Failures carry the tag's bytes, because a misread offset in
// this format is otherwise indistinguishable from a corrupt file.
func (db *DB) Lookup(term string) (out Term, err error) {
	tag, found, err := db.keyTag(TablePostlist, []byte(term))
	if err != nil {
		return Term{}, err
	}
	if !found {
		return Term{}, nil
	}
	defer func() {
		if err != nil {
			err = fmt.Errorf("glass: term %q (tag % x): %w", term, tag, err)
		}
	}()

	p := 0
	if out.DocFreq, p, err = unpackUint(tag, p); err != nil {
		return Term{}, err
	}
	if out.CollFreq, p, err = unpackUint(tag, p); err != nil {
		return Term{}, err
	}
	// The first chunk: the first document id (stored minus one), then the header
	// shared with every other chunk - whether it is the last one and which
	// document ends it - then the frequency of the first document, then a gap
	// and a frequency for each further document.
	var did uint32
	if did, p, err = unpackUint(tag, p); err != nil {
		return Term{}, err
	}
	did++
	if p >= len(tag) {
		out.Complete = true
		return out, nil
	}
	last := tag[p]
	p++
	var span uint32
	if span, p, err = unpackUint(tag, p); err != nil {
		return Term{}, err
	}
	lastDid := did + span
	var wdf uint32
	if wdf, p, err = unpackUint(tag, p); err != nil {
		return Term{}, err
	}
	for {
		out.Postings = append(out.Postings, Posting{DocID: did, WDF: wdf})
		if did >= lastDid {
			break
		}
		if p >= len(tag) {
			err = fmt.Errorf("the chunk ends after %d of %d documents", len(out.Postings), out.DocFreq)
			return Term{}, err
		}
		var gap uint32
		if gap, p, err = unpackUint(tag, p); err != nil {
			return Term{}, err
		}
		did += gap + 1
		if wdf, p, err = unpackUint(tag, p); err != nil {
			return Term{}, err
		}
	}
	out.Complete = last != 0
	return out, nil
}

// DocPath returns the article a document id stands for, as the index stores it.
// The docdata table is keyed by the id in sort-preserving form and holds one
// record per document.
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
