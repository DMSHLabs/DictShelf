// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

// The third form a prepared ZIM database can take: our own article index, and
// no article bodies.
//
// The read-through reader exists so that article text can reach an index of
// ours, and it stores the bodies while it is at it. Storing them buys nothing:
// reading an article out of the archive and reading it out of the database come
// out the same (tools/zimtime: medians 0.52-0.66 ms either way), while the
// bodies are four to five times the size of the index they were copied for - on
// the English Wiktionary 19.9 GB of bodies and headwords against 4.6 GB of
// index. So an archive that has no index of its own gets ONE of ours, with the
// text read and indexed and the bodies left in the archive.
//
// This is a wrapper rather than a mode of the reader because the reader is
// upstream's file: the body it yields is exactly what the ingest needs to index,
// and the only thing that changes is that the ingest must not keep it. That is
// said to the store by DropBodies, and the form is recorded in the meta as the
// title form plus a text index - index_kind "headwords" with ingest_level
// "text" - which is a combination that did not previously exist and needs no new
// key to describe.
type IndexReader struct {
	*Reader
}

// NewIndexReader prepares the searchable index of an archive whose bodies stay
// where they are.
func NewIndexReader(path string) (*IndexReader, error) {
	r, err := newReader(path, false)
	if err != nil {
		return nil, err
	}
	return &IndexReader{Reader: r}, nil
}

// DropBodies tells the ingest that what it reads is for the index, not for the
// database.
func (r *IndexReader) DropBodies() bool { return true }

// ExtraMeta records the form: the headword form, because the database holds no
// articles, with a text index, which is what ingest_level says.
func (r *IndexReader) ExtraMeta() map[string]string {
	m := r.Reader.ExtraMeta()
	m["index_kind"] = "headwords"
	return m
}
