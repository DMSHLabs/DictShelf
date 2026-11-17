// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package glass

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
)

// A branched table with split tags puts seek targets on both sides of leaf
// boundaries, in gaps between chunks and beyond the final document.
func skipFixture(t *testing.T, splitLater bool) *DB {
	const bs = 8192
	term := "term"
	prefix := chunkTermKey(term)
	key := func(d uint32) []byte { return appendPreservingSort(append([]byte(nil), prefix...), d) }
	tag := func(first, last uint32, final bool, head bool) []byte {
		var b []byte
		if head {
			b = packUint(b, 6)
			b = packUint(b, 21)
			b = packUint(b, first-1)
		}
		end := byte('0')
		if final {
			end = '1'
		}
		b = append(b, end)
		b = packUint(b, last-first)
		b = packUint(b, first)
		if first != last {
			b = packUint(b, last-first-1)
			b = packUint(b, last)
		}
		return b
	}
	data := make([]byte, 5*bs)
	copy(data, buildVersionFile(1, bs))
	leaf := func(n int, k, tag []byte, split bool) {
		b := data[n*bs : (n+1)*bs]
		binary.BigEndian.PutUint32(b, 1)
		at := bs
		pieces := [][]byte{tag}
		if split {
			pieces = [][]byte{tag[:len(tag)/2], tag[len(tag)/2:]}
		}
		for i, piece := range pieces {
			flags := byte(0)
			if i == 0 {
				flags |= flagFirst
			}
			if i == len(pieces)-1 {
				flags |= flagLast
			}
			size := len(k) + len(piece)
			if i > 0 {
				size += 2
			}
			item := []byte{flags | byte(size>>8), byte(size), byte(len(k))}
			item = append(item, k...)
			if i > 0 {
				item = append(item, 0, byte(i+1))
			}
			item = append(item, piece...)
			at -= len(item)
			copy(b[at:], item)
			binary.BigEndian.PutUint16(b[11+2*i:], uint16(at))
		}
		binary.BigEndian.PutUint16(b[9:], uint16(11+2*len(pieces)))
	}
	leaf(2, []byte(term), tag(2, 7, false, true), true)
	leaf(3, key(40000), tag(40000, 40010, false, false), splitLater)
	leaf(4, key(5000000), tag(5000000, 5000010, true, false), false)
	root := data[bs : 2*bs]
	root[4] = 1
	at := bs
	for i, k := range [][]byte{nil, key(40000), key(5000000)} {
		item := make([]byte, 5)
		binary.BigEndian.PutUint32(item, uint32(i+2))
		item[4] = byte(len(k))
		item = append(item, k...)
		item = append(item, 0, 1)
		at -= len(item)
		copy(root[at:], item)
		binary.BigEndian.PutUint16(root[11+2*i:], uint16(at))
	}
	binary.BigEndian.PutUint16(root[9:], 17)
	db, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	db.Roots[TablePostlist].Level = 1
	return db
}
func TestSkipToAcrossChunks(t *testing.T) {
	db := skipFixture(t, false)
	full, err := db.Lookup("term")
	if err != nil || !full.Complete || len(full.Postings) != 6 {
		t.Fatalf("fixture: %+v %v", full, err)
	}
	targets := []uint32{1, 2, 3, 7, 8, 39999, 40000, 40001, 40010, 40011, 4999999, 5000000, 5000001, 5000010, 5000011}
	for _, target := range targets {
		c, _, err := db.postingCursor(context.Background(), "term", chunkTermKey("term"))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.skipTo(target); err != nil {
			t.Fatal(err)
		}
		want := uint32(0)
		for _, p := range full.Postings {
			if p.DocID >= target {
				want = p.DocID
				break
			}
		}
		if c.did != want {
			t.Fatalf("skipTo(%d)=%d want %d", target, c.did, want)
		}
	}
	var candidates []Hit
	for _, target := range targets {
		candidates = append(candidates, Hit{DocID: target})
	}
	// The optimized cursor also assembles multi-component later chunks.
	db = skipFixture(t, true)
	got, err := db.selectedPostings(context.Background(), "term", chunkTermKey("term"), candidates)
	if err != nil {
		t.Fatal(err)
	}
	var want []Hit
	for _, p := range full.Postings {
		want = append(want, Hit{DocID: p.DocID, WDF: p.WDF})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected=%v want=%v", got, want)
	}
	// Before the first key and at the exact keys of split tags.
	for _, target := range [][]byte{[]byte("a"), []byte("term"), append(appendPreservingSort(chunkTermKey("term"), 40000), 1), []byte("z")} {
		k, err := db.floorKey(TablePostlist, target)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(k, target) > 0 {
			t.Fatalf("floor(%x)=%x", target, k)
		}
	}
}

func TestSelectedPostingsCancellation(t *testing.T) {
	db := skipFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	visited := 0
	db.Progress = func(uint32) { visited++; cancel() }
	hits, err := db.selectedPostings(ctx, "term", chunkTermKey("term"), []Hit{{DocID: 40000}})
	if !errors.Is(err, context.Canceled) || len(hits) != 0 || visited != 1 {
		t.Fatalf("hits=%v err=%v visited=%d", hits, err, visited)
	}
}
