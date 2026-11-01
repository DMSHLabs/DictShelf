// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// Xapian's packed integers are LEB128; this is the only place in the format
// where a value is not fixed width, so it is worth pinning against the values
// Xapian itself writes.
func TestUnpackUint(t *testing.T) {
	for _, tc := range []struct {
		in   []byte
		want uint32
	}{
		{[]byte{0x00}, 0},
		{[]byte{0x7f}, 127},
		{[]byte{0x80, 0x01}, 128},
		{[]byte{0xff, 0x01}, 255},
		{[]byte{0x80, 0x80, 0x01}, 16384},
		{[]byte{0xff, 0xff, 0xff, 0xff, 0x0f}, 0xffffffff},
	} {
		got, p, err := unpackUint(tc.in, 0)
		if err != nil {
			t.Fatalf("% x: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("% x = %d, want %d", tc.in, got, tc.want)
		}
		if p != len(tc.in) {
			t.Errorf("% x: consumed %d of %d", tc.in, p, len(tc.in))
		}
	}
	if _, _, err := unpackUint([]byte{0x80}, 0); err == nil {
		t.Error("a truncated integer must be an error, not a value")
	}
	if _, _, err := unpackUint([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, 0); err == nil {
		t.Error("an integer longer than 32 bits must be refused")
	}
}

// A single-file database addresses its blocks from the end of the version file,
// with one numbering shared by all six tables - the thing a reader written for
// the multi-file layout gets wrong. The version file is padded up to a block
// boundary, so block 1 is always the first block of the area.
func TestBlockOffsetIsFromTheVersionEnd(t *testing.T) {
	db := &DB{data: make([]byte, 3*8192), Roots: [numTables]Root{{BlockSize: 8192}}}
	db.versionBytes = 200
	off, err := db.blockOffset(1)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(8192); off != want {
		t.Errorf("block 1 at %d, want %d", off, want)
	}
	if off, err = db.blockOffset(2); err != nil || off != 2*8192 {
		t.Errorf("block 2 at %d (%v), want %d", off, err, 2*8192)
	}
	if _, err := db.blockOffset(0); err == nil {
		t.Error("block 0 does not exist (blockNone)")
	}
	if _, err := db.blockOffset(3); err == nil {
		t.Error("a block past the end of the data must be refused")
	}
}

// A block's directory is a list of 2-byte big-endian item offsets after the
// 11-byte header; the search takes the last item whose key is not greater than
// the one asked for, which is how Xapian's find_in_leaf descends.
func TestDirectorySearch(t *testing.T) {
	b := make([]byte, 64)
	// two leaf items at offsets 40 and 50: I2 K1 key
	place := func(off int, key string) {
		binary.BigEndian.PutUint16(b[off:], uint16(3+len(key)))
		b[off+2] = byte(len(key))
		copy(b[off+3:], key)
	}
	place(40, "bee")
	place(50, "dee")
	binary.BigEndian.PutUint16(b[11:], 40)
	binary.BigEndian.PutUint16(b[13:], 50)
	binary.BigEndian.PutUint16(b[9:], 15) // directory ends right after the two entries
	items, err := directoryItems(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0] != 40 || items[1] != 50 {
		t.Fatalf("items = %v, want [40 50]", items)
	}
	if got := itemKey(b, 50, false); !bytes.Equal(got, []byte("dee")) {
		t.Errorf("key = %q, want dee", got)
	}
	for _, tc := range []struct {
		key  string
		want int
	}{{"aaa", -1}, {"bee", 0}, {"cee", 0}, {"dee", 1}, {"zzz", 1}} {
		if got := searchItems(b, items, []byte(tc.key), false); got != tc.want {
			t.Errorf("search %q = %d, want %d", tc.key, got, tc.want)
		}
	}
}

// A branch item puts its child block number in the first four bytes and the key
// length after that, two bytes further along than a leaf item puts them. Reading
// one with the other's offsets compares the wrong bytes and the descent goes
// quietly astray, so this pins both layouts.
func TestItemKeysOfBothKinds(t *testing.T) {
	leaf := []byte{0x00, 0x05, 0x02, 'h', 'i', 0x00, 0x00}
	if got := itemKey(leaf, 0, false); !bytes.Equal(got, []byte("hi")) {
		t.Errorf("leaf key = %q, want hi", got)
	}
	branch := []byte{0x00, 0x00, 0x01, 0x02, 0x03, 'a', 'b', 'c', 0x00, 0x01}
	if got := itemKey(branch, 0, true); !bytes.Equal(got, []byte("abc")) {
		t.Errorf("branch key = %q, want abc", got)
	}
	if block := binary.BigEndian.Uint32(branch[:4]); block != 0x00000102 {
		t.Errorf("child block = %d, want 0x102", block)
	}
}
