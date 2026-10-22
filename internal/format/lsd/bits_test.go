// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"encoding/binary"
	"testing"
)

func TestBitReaderMSBAndBounds(t *testing.T) {
	r := &bitReader{data: []byte{0b10110010}}
	for i, want := range []uint32{5, 4, 2} {
		width := uint8(3)
		if i == 2 {
			width = 2
		}
		got, err := r.read(width)
		if err != nil || got != want {
			t.Fatalf("read bits = %d, %v; want %d", got, err, want)
		}
	}
	if _, err := r.read(1); err == nil {
		t.Fatal("expected truncated bit stream error")
	}
}

func TestLengthTableDecodesOneBitCodes(t *testing.T) {
	// Two symbols, one length bit each, followed by their two code bits.
	data := make([]byte, 10)
	binary.BigEndian.PutUint32(data, 2)
	data[4] = 1
	// Remaining packed bits: (idx 0, len 1), (idx 1, len 1), codes 0, 1.
	data[5] = 0b00101101
	data[6] = 0
	r := &bitReader{data: data}
	table, err := readLengthTable(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []uint32{0, 1} {
		got, err := table.decode(r)
		if err != nil || got != want {
			t.Fatalf("decoded symbol = %d, %v; want %d", got, err, want)
		}
	}
}

func TestSystemArticleSymbolModes(t *testing.T) {
	// A one-bit Huffman table selects a literal followed by a three-unit prefix
	// copy. The system alphabet stores the literal with the 0x80 bias.
	d := &decoder{
		prefix:         []uint16{'A', 'B', 'C'},
		articleSymbols: []uint32{0xe1, 0},
		articles:       &lengthTable{nodes: []huffmanNode{{left: -1, right: -2}}, root: 0},
		systemArticles: true,
	}
	got, err := d.decodeArticle([]byte{0b01000000}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got != "aABC" {
		t.Fatalf("decoded system article = %q; want %q", got, "aABC")
	}
}

func TestXoredSymbols(t *testing.T) {
	// One symbol is stored as 'x' XOR the abbreviation alphabet mask.
	data := []byte{0, 0, 0, 1, 16, 0x13, 0x5d}
	r := &bitReader{data: data}
	symbols, err := readSymbols(r, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 || symbols[0] != 'x' {
		t.Fatalf("decoded symbols = %v; want ['x']", symbols)
	}
}
