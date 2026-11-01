// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import "fmt"

// packPreservingSort encodes an unsigned integer so that byte order equals
// numeric order (Xapian's C_pack_uint_preserving_sort, common/pack.h): a first
// byte holding the length minus one in its top two bits and the value's top six
// bits below them, then that many bytes of the rest, most significant first.
// Glass uses it for the keys of the docdata table and for the keys that carry
// the later chunks of a posting list, and the ordering is the reason: those keys
// are looked up and walked in sorted order.
func packPreservingSort(v uint32) []byte {
	var rest [4]byte
	n := 0
	for v > 0x3f {
		rest[3-n] = byte(v)
		v >>= 8
		n++
	}
	out := make([]byte, 0, n+1)
	out = append(out, byte(n)<<6|byte(v))
	return append(out, rest[4-n:]...)
}

// unpackPreservingSort is the inverse; it reports how many bytes it used.
func unpackPreservingSort(b []byte, p int) (uint32, int, error) {
	if p >= len(b) {
		return 0, p, fmt.Errorf("glass: a sort-preserving integer runs past the end")
	}
	first := b[p]
	p++
	n := int(first>>6) + 1
	if n > 4 || p+n > len(b) {
		return 0, p, fmt.Errorf("glass: a sort-preserving integer of %d extra bytes does not fit", n)
	}
	v := uint32(first & 0x3f)
	for i := 0; i < n; i++ {
		v = v<<8 | uint32(b[p+i])
	}
	return v, p + n, nil
}

// DocKey is the key the docdata table stores a document under: the id in
// sort-preserving form, so the keys sort the way the ids do. Exported because a
// test or a diagnostic has to name a document's key without guessing.
func DocKey(docid uint32) []byte { return packPreservingSort(docid) }
