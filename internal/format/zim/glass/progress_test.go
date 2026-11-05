// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"encoding/binary"
	"testing"
)

// postlistFixture builds a one-block database holding one term's posting list,
// so a walk can be driven without an archive. The tag is what the walk reads
// (postlist.go, and glass_postlist.cc for the field order): the term's document
// frequency, its collection frequency, the first document stored one less, the
// chunk header ('1' for the last chunk, then how far past the first document it
// reaches), the first document's frequency, and then a gap and a frequency per
// further document.
func postlistFixture(term string, docids, wdfs []uint32) []byte {
	const blockSize = 8192
	data := make([]byte, 2*blockSize)
	copy(data, buildVersionFile(1, blockSize))

	var coll uint32
	for _, w := range wdfs {
		coll += w
	}
	tag := packUint(nil, uint32(len(docids)))
	tag = packUint(tag, coll)
	tag = packUint(tag, docids[0]-1)
	tag = append(tag, '1')
	tag = packUint(tag, docids[len(docids)-1]-docids[0])
	tag = packUint(tag, wdfs[0])
	for i := 1; i < len(docids); i++ {
		tag = packUint(tag, docids[i]-docids[i-1]-1)
		tag = packUint(tag, wdfs[i])
	}

	// A leaf item is I2 K1 key tag, with I the length of the key-tag pair and
	// the flags in the top three bits of its first byte.
	size := len(term) + len(tag)
	item := []byte{flagFirst | flagLast | byte(size>>8), byte(size), byte(len(term))}
	item = append(item, term...)
	item = append(item, tag...)

	block := data[blockSize:]
	off := blockSize - len(item)
	copy(block[off:], item)
	binary.BigEndian.PutUint32(block[0:], 1)                  // revision
	block[4] = 0                                              // level 0: a leaf
	binary.BigEndian.PutUint16(block[9:], uint16(dirStart+2)) // DIR_END: one entry
	binary.BigEndian.PutUint16(block[11:], uint16(off))       // the item's offset
	return data
}

// A walk reports every posting it visits through DB.Progress, once per posting
// and in the list's own order: that is the movement a caller watching for a
// walk that stops is reading (internal/watchdog), and the document it reports
// is the record a stall is named with.
func TestTheWalkReportsEveryPosting(t *testing.T) {
	db, err := Open(postlistFixture("кларнетист", []uint32{7, 1060, 34193}, []uint32{2, 5, 1}))
	if err != nil {
		t.Fatal(err)
	}
	var seen []uint32
	db.Progress = func(docid uint32) { seen = append(seen, docid) }

	term, err := db.Lookup("кларнетист")
	if err != nil {
		t.Fatal(err)
	}
	if term.DocFreq != 3 || term.CollFreq != 8 || len(term.Postings) != 3 || !term.Complete {
		t.Fatalf("term = %+v, want three documents, eight occurrences, read whole", term)
	}
	want := []uint32{7, 1060, 34193}
	if len(seen) != len(want) {
		t.Fatalf("the walk reported %v, want the list's own documents %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("the walk reported %v, want the list's own documents %v", seen, want)
		}
	}

	// A term the index does not hold visits nothing, so it reports nothing: a
	// miss is not movement.
	if _, err := db.Lookup("нет-такого-терма"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(want) {
		t.Errorf("a term that is not there reported %v", seen)
	}
}
