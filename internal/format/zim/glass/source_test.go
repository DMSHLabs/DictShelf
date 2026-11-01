// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"bytes"
	"testing"
)

// A database read through a Source and the same bytes in memory must read the
// same thing: Open takes views of the array it was given and OpenSource reads
// into a fresh buffer per block, and the two address blocks the same way. The
// archive this reader exists for is 518 MB in one place and 3.18 GB in another,
// stored uncompressed, so the Source path is the one a product uses - which is
// exactly why it has to agree with the path the tests were written against.
func TestSourceAndMemoryReadTheSameBlocks(t *testing.T) {
	const blockSize = 8192
	data := make([]byte, 3*blockSize)
	version := buildVersionFile(1, blockSize)
	copy(data, version)
	for i := blockSize; i < 2*blockSize; i++ {
		data[i] = byte(i)
	}

	mem, err := Open(data)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	src, err := OpenSource(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenSource: %v", err)
	}
	if mem.versionBytes != src.versionBytes {
		t.Errorf("version file ends at %d in memory and %d from the source", mem.versionBytes, src.versionBytes)
	}
	if mem.Roots != src.Roots || mem.Stats != src.Stats || mem.Revision != src.Revision {
		t.Errorf("version file parsed differently:\n memory: %+v %+v\n source: %+v %+v",
			mem.Roots, mem.Stats, src.Roots, src.Stats)
	}
	for _, n := range []uint32{1, 2} {
		want, err := mem.block(n)
		if err != nil {
			t.Fatalf("block %d in memory: %v", n, err)
		}
		got, err := src.block(n)
		if err != nil {
			t.Fatalf("block %d from the source: %v", n, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("block %d differs between memory and source", n)
		}
	}
	// The view is the array itself, the copy is not: that difference is the
	// point of both paths, and it is what the caller must not depend on.
	if &data[blockSize] != &mustBlock(t, mem, 1)[0] {
		t.Error("Open should read blocks as views into the array it was given")
	}
	if _, err := src.block(3); err == nil {
		t.Error("a block past the end must be refused on the source path too")
	}
}

// mustBlock is block with the error turned into a test failure.
func mustBlock(t *testing.T, db *DB, n uint32) []byte {
	t.Helper()
	b, err := db.block(n)
	if err != nil {
		t.Fatalf("block %d: %v", n, err)
	}
	return b
}

// buildVersionFile writes the version file of a database whose only non-empty
// table is the postlist one, with its root at the given block. It is written
// with Xapian's own encodings so the parser is exercised, not bypassed.
func buildVersionFile(postlistRoot, blockSize uint32) []byte {
	var b bytes.Buffer
	b.Write(magic)
	b.Write([]byte{0, 0}) // the format version, big-endian
	b.Write(make([]byte, 16))
	b.Write(packUint(nil, 1)) // revision
	for i := 0; i < numTables; i++ {
		root, level, entries := uint32(0), uint32(0), uint32(0)
		if i == TablePostlist {
			root, entries = postlistRoot, 1
		}
		b.Write(packUint(nil, root))
		b.Write(packUint(nil, level<<2))
		b.Write(packUint(nil, entries))
		b.Write(packUint(nil, blockSize>>11))
		b.Write(packUint(nil, 8)) // compress_min
		b.Write(packUint(nil, 0)) // an empty free list
	}
	for i := 0; i < 8; i++ { // the statistics, all zero but doccount
		if i == 0 {
			b.Write(packUint(nil, 3))
			continue
		}
		b.Write(packUint(nil, 0))
	}
	return b.Bytes()
}

// packUint is Xapian's LEB128 packing, for the test's fixture only; the reader
// never writes.
func packUint(dst []byte, v uint32) []byte {
	for {
		by := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			by |= 0x80
		}
		dst = append(dst, by)
		if v == 0 {
			return dst
		}
	}
}
