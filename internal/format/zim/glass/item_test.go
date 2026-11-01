// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"encoding/binary"
	"strings"
	"testing"
)

// An item's size field covers the key AND the tag - Xapian writes "I K key X
// tag" where I is the length of the key-tag pair - so the tag ends three bytes
// past the size (the size field itself and the key length byte). Reading the
// size as the whole item cut every tag three bytes short, which showed up as
// article paths truncated in the middle of a character and as a posting list
// that ended one document before its own header said it did.
//
// The bytes below are the first docdata item of a real Kiwix archive, copied out
// of it: document 250122, key 83 d1 0a (the id in sort-preserving form), path
// "C/кларнетистка" - twelve Cyrillic characters, twenty-four bytes, so the item
// is 2 + 1 + 3 + 26 = 32 bytes and the size field says 29, which is 3 + 26.
func TestItemSizeCoversKeyAndTag(t *testing.T) {
	item := []byte{
		0x60, 0x1d, // the 2-byte value: flags 0x60 (first and last component), size 29
		0x03,             // key length
		0x83, 0xd1, 0x0a, // key: the document id
		0x43, 0x2f, 0xd0, 0xba, 0xd0, 0xbb, 0xd0, 0xb0, // "C/кла"
		0xd1, 0x80, 0xd0, 0xbd, 0xd0, 0xb5, 0xd1, 0x82, // "рнет"
		0xd0, 0xb8, 0xd1, 0x81, 0xd1, 0x82, 0xd0, 0xba, 0xd0, 0xb0, // "истка"
	}
	block := make([]byte, 64)
	copy(block, item)
	tag, flags, err := itemTag(block, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "C/кларнетистка"
	if string(tag) != want {
		t.Errorf("tag = %q, want %q", tag, want)
	}
	if flags != flagFirst|flagLast {
		t.Errorf("flags = %02x, want %02x", flags, flagFirst|flagLast)
	}
	if _, _, err := unpackPreservingSort(item[3:6], 0); err != nil {
		t.Fatal(err)
	}
	if id, _, _ := unpackPreservingSort(item[3:6], 0); id != 250122 {
		t.Errorf("key decodes to %d, want 250122", id)
	}
	// The same arithmetic on the postlist item of the term "кларнетист": size 70
	// with a 20-byte key, so the tag is 50 bytes and the chunk's 11 documents
	// decode completely.
	if 70-20 != 50 || !strings.HasPrefix("кларнетист", "кларн") {
		t.Fatal("the arithmetic this test documents has changed")
	}
}

// DocKey has to produce keys that sort the way document ids do, because the
// docdata table is walked in that order.
func TestDocKeySortsLikeTheIDs(t *testing.T) {
	ids := []uint32{1, 2, 127, 128, 255, 256, 2472, 250122, 1528188}
	for i := 1; i < len(ids); i++ {
		lo, hi := DocKey(ids[i-1]), DocKey(ids[i])
		if string(lo) >= string(hi) {
			t.Errorf("%d encodes to % x, %d to % x: not ascending", ids[i-1], lo, ids[i], hi)
		}
		if _, n, err := unpackPreservingSort(hi, 0); err != nil || n != len(hi) {
			t.Errorf("%d: %v, consumed %d of %d", ids[i], err, n, len(hi))
		}
	}
	if got := binary.BigEndian.Uint16(DocKey(1528188)); got == 0 {
		t.Error("a key must have bytes")
	}
}
