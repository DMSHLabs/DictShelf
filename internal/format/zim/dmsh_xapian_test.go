// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import "testing"

// An archive with a full-text index, and one without: the question every ingest
// asks before deciding whether to build an index of its own. The bytes are
// synthetic, so what is being tested is the lookup (a namespace, a path, and
// the cluster's own info byte), not somebody's archive.
func TestHasFullTextIndex(t *testing.T) {
	index := bEntry{ns: 'X', path: "fulltext/xapian", mime: "application/octet-stream",
		body: []byte("0f 0d Xapian Glass ..."), cluster: 0}

	with := sample()
	with.entries = append(with.entries, index)
	path := writeZIM(t, with)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !d.HasFullTextIndex() {
		t.Error("an archive holding X/fulltext/xapian reported no index of its own")
	}
	// The probe is a promise about the reader: whatever it says yes to,
	// XapianIndex has to be able to hand out.
	if _, n, err := d.XapianIndex(); err != nil || n <= 0 {
		t.Errorf("the probe said yes and the reader could not deliver: n=%d err=%v", n, err)
	}
	if ok, err := HasOwnFullTextIndex(path); err != nil || !ok {
		t.Errorf("HasOwnFullTextIndex = %v, %v; want true, nil", ok, err)
	}

	if plain := openSample(t); plain.HasFullTextIndex() {
		t.Error("the fixture without an index reported one")
	}

	// An index inside a COMPRESSED cluster answers no on purpose: reading it
	// would mean decompressing the whole thing (up to 3 GB) before the first
	// term could be looked up, so the useful answer for a build decision is that
	// there is nothing usable and the archive gets an index of ours.
	packed := sample()
	packed.entries = append(packed.entries, bEntry{ns: 'X', path: "fulltext/xapian",
		mime: "application/octet-stream", body: []byte("0f 0d Xapian Glass ..."), cluster: 1})
	pd, err := Open(writeZIM(t, packed))
	if err != nil {
		t.Fatal(err)
	}
	defer pd.Close()
	if pd.HasFullTextIndex() {
		t.Error("an index in a compressed cluster was reported as readable in place")
	}
	if _, _, err := pd.XapianIndex(); err == nil {
		t.Error("the probe said no and the reader still handed out a window")
	}
}

// The reader records the fact while the archive is open anyway, so a dictionary
// list row never has to open a 48 GB container to say where full text comes
// from. It rides beside index_kind, which the ingest already reads.
func TestReaderRecordsItsOwnIndex(t *testing.T) {
	f := sample()
	f.entries = append(f.entries, bEntry{ns: 'X', path: "fulltext/xapian",
		mime: "application/octet-stream", body: []byte("0f 0d Xapian Glass ..."), cluster: 0})

	r, err := NewHeadwordReader(writeZIM(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.ExtraMeta()[OwnIndexMetaKey] != "1" {
		t.Errorf("%s not recorded in %v", OwnIndexMetaKey, r.ExtraMeta())
	}

	plain, err := NewHeadwordReader(writeZIM(t, sample()))
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if _, ok := plain.ExtraMeta()[OwnIndexMetaKey]; ok {
		t.Error("an archive without its own index recorded one")
	}
}
