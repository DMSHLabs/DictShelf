// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"fmt"
	"math/bits"
)

// packPreservingSort encodes an unsigned integer so that byte order equals
// numeric order: Xapian's pack_uint_preserving_sort, the "glass and newer"
// variant (common/pack.h). Glass uses it for the keys of the docdata table and
// for the keys that carry the later chunks of a posting list, and the ordering
// is the reason: those keys are looked up and walked in sorted order.
//
// The encoding is a run of set bits marking the length, then the value:
//
//	[0x0000, 0x7fff]                 AAAAAAAA BBBBBBBB
//	[0x8000, 0x3fffff]              10AAAAAA BBBBBBBB CCCCCCCC
//	[0x400000, 0x1fffffff]          110AAAAA BBBBBBBB CCCCCCCC DDDDDDDD
//	... up to five bytes for a 32-bit value
//
// The count of leading set bits is two less than the number of bytes, so the
// shortest form has no marker at all: a value below 0x8000 is written as two
// plain big-endian bytes. Two details are easy to get wrong, and were:
//
//   - The first form has NO marker byte, so a small document id is not written
//     in one byte. A reader that packs it in one looks up a key nothing holds;
//     a reader that packs it with a 0x40 marker finds a DIFFERENT document,
//     because 0x40 0x09 0xa8 is the encoding of a larger id (see
//     unpackPreservingSort on why the two forms agree above 0x8000 and differ
//     below it, which is what made the mistake hard to see).
//   - The marker is a unary length, not two bits of length: it is what makes
//     the encodings sort correctly.
func packPreservingSort(v uint32) []byte {
	return appendPreservingSort(nil, v)
}

// appendPreservingSort is packPreservingSort onto a key that is already being
// built, which is what the later chunks of a posting list need: their key is a
// term, a zero byte and this.
func appendPreservingSort(dst []byte, v uint32) []byte {
	if v < 0x8000 {
		return append(dst, byte(v>>8), byte(v))
	}
	// Number of bytes: Xapian's ((bits + 5) - clz) / 7, which is this.
	length := (bits.Len32(v) + 5) / 7
	// The marker is length-2 set bits and a clear one before the value's
	// leading bits, and the remaining bytes follow most significant first.
	dst = append(dst, byte(0xff)<<uint(10-length)|byte(v>>uint(8*(length-1))))
	for i := length - 1; i >= 1; i-- {
		dst = append(dst, byte(v>>uint(8*(i-1))))
	}
	return dst
}

// unpackPreservingSort is the inverse; it reports how many bytes it used.
func unpackPreservingSort(b []byte, p int) (uint32, int, error) {
	if p >= len(b) {
		return 0, p, fmt.Errorf("glass: a sort-preserving integer runs past the end")
	}
	first := b[p]
	// The leading set bits are the length marker: their count is two less than
	// the number of bytes the value occupies, two bytes being the shortest.
	n := 0
	for n < 8 && first&(0x80>>uint(n)) != 0 {
		n++
	}
	if n > 7 {
		return 0, p, fmt.Errorf("glass: a sort-preserving integer starts with an impossible marker %#x", first)
	}
	if n == 7 {
		// 0xfe is the nine-byte form, for values that do not fit in 32 bits.
		return 0, p, fmt.Errorf("glass: a sort-preserving integer is wider than 32 bits")
	}
	total := n + 2
	if p+total > len(b) {
		return 0, p, fmt.Errorf("glass: a sort-preserving integer of %d bytes runs past the end", total)
	}
	v := uint32(first &^ (byte(0xff) << uint(8-n)))
	for i := 1; i < total; i++ {
		v = v<<8 | uint32(b[p+i])
	}
	return v, p + total, nil
}

// DocKey is the key the docdata table stores a document under: the id in
// sort-preserving form, so the keys sort the way the ids do. Exported because a
// test or a diagnostic has to name a document's key without guessing.
func DocKey(docid uint32) []byte { return packPreservingSort(docid) }
