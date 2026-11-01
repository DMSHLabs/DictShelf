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

// Term is a term's posting list as far as this reader could read it. Complete
// says whether every chunk was read and every document in it: Xapian splits a
// long list into several items, and the reader stops rather than failing when a
// chunk does not end where its header says it should.
type Term struct {
	Postings []Posting
	// DocFreq is what the index records for the term (documents), CollFreq the
	// total number of occurrences.
	DocFreq  uint32
	CollFreq uint32
	Complete bool
	// Leftover counts the bytes of the first chunk that were not consumed, which
	// is a decoding problem rather than a property of the index.
	Leftover int
}

// Lookup reads a term's posting list. The term must be exactly what the index
// stores: for libzim that is the lowercase, unaccented word (HANDOFF.md, "The
// glass format").
func (db *DB) Lookup(term string) (Term, error) {
	tag, found, err := db.keyTag(TablePostlist, []byte(term))
	if err != nil {
		return Term{}, err
	}
	if !found {
		return Term{}, nil
	}
	out := Term{}
	p := 0
	if out.DocFreq, p, err = unpackUint(tag, p); err != nil {
		return Term{}, fmt.Errorf("glass: term %q: document frequency: %w", term, err)
	}
	if out.CollFreq, p, err = unpackUint(tag, p); err != nil {
		return Term{}, fmt.Errorf("glass: term %q: collection frequency: %w", term, err)
	}
	// The first chunk: the first document id (stored minus one), then the header
	// shared with every chunk - a bool written as '0' or '1' saying whether this
	// is the last chunk, and how far the chunk reaches - then the frequency of
	// the first document, then a gap and a frequency for each further document.
	var did uint32
	if did, p, err = unpackUint(tag, p); err != nil {
		return Term{}, fmt.Errorf("glass: term %q: first document: %w", term, err)
	}
	did++
	if p >= len(tag) {
		out.Complete = true
		return out, nil
	}
	lastByte := tag[p]
	p++
	if lastByte != '0' && lastByte != '1' {
		return Term{}, fmt.Errorf("glass: term %q: chunk flag %q is neither '0' nor '1'", term, lastByte)
	}
	lastChunk := lastByte == '1'
	var span uint32
	if span, p, err = unpackUint(tag, p); err != nil {
		return Term{}, fmt.Errorf("glass: term %q: chunk end: %w", term, err)
	}
	lastDid := did + span
	var wdf uint32
	if wdf, p, err = unpackUint(tag, p); err != nil {
		return Term{}, fmt.Errorf("glass: term %q: first frequency: %w", term, err)
	}
	for {
		out.Postings = append(out.Postings, Posting{DocID: did, WDF: wdf})
		if did >= lastDid {
			break
		}
		if p >= len(tag) {
			// The chunk says it reaches further than its bytes do. Return what
			// was read and let the caller decide; a search can still use the
			// documents that were decoded.
			out.Leftover = 0
			out.Complete = false
			return out, nil
		}
		var gap uint32
		if gap, p, err = unpackUint(tag, p); err != nil {
			out.Leftover = len(tag) - p
			out.Complete = false
			return out, nil
		}
		did += gap + 1
		if wdf, p, err = unpackUint(tag, p); err != nil {
			out.Leftover = len(tag) - p
			out.Complete = false
			return out, nil
		}
	}
	out.Complete = lastChunk && p == len(tag)
	out.Leftover = len(tag) - p
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
