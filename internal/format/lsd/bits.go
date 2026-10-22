// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"encoding/binary"
	"fmt"
)

// bitReader consumes the MSB-first bit stream used by the decoder and page
// records. It never reads past the supplied section.
type bitReader struct {
	data []byte
	bit  uint64
}

func (r *bitReader) read(n uint8) (uint32, error) {
	if n > 32 {
		return 0, fmt.Errorf("lsd: invalid bit count %d", n)
	}
	limit := uint64(len(r.data)) * 8
	if r.bit > limit || uint64(n) > limit-r.bit {
		return 0, fmt.Errorf("lsd: truncated bit stream at bit %d", r.bit)
	}
	var v uint32
	for i := uint8(0); i < n; i++ {
		byteAt := r.data[r.bit>>3]
		v = (v << 1) | uint32((byteAt>>(7-(r.bit&7)))&1)
		r.bit++
	}
	return v, nil
}

func (r *bitReader) align() {
	if r.bit&7 != 0 {
		r.bit = (r.bit + 7) &^ 7
	}
}

func (r *bitReader) readU32BE() (uint32, error) {
	r.align()
	if r.bit > uint64(len(r.data))*8 || 32 > uint64(len(r.data))*8-r.bit {
		return 0, fmt.Errorf("lsd: truncated 32-bit value")
	}
	v := binary.BigEndian.Uint32(r.data[r.bit/8 : r.bit/8+4])
	r.bit += 32
	return v, nil
}

type huffmanNode struct {
	left, right int32 // zero = absent; positive = child index + 1; negative = symbol index - 1
}

type lengthTable struct {
	nodes []huffmanNode
	root  int
}

func readLengthTable(r *bitReader) (*lengthTable, error) {
	count, err := r.read(32)
	if err != nil {
		return nil, err
	}
	width, err := r.read(8)
	if err != nil {
		return nil, err
	}
	if count == 0 || count > 1<<20 || width == 0 || width > 32 {
		return nil, fmt.Errorf("lsd: invalid Huffman table dimensions (%d symbols, %d length bits)", count, width)
	}
	t := &lengthTable{nodes: make([]huffmanNode, count-1), root: int(count) - 2}
	next := 0
	for i := uint32(0); i < count; i++ {
		idx, err := r.read(bitWidth(count))
		if err != nil {
			return nil, err
		}
		length, err := r.read(uint8(width))
		if err != nil {
			return nil, err
		}
		if idx >= count || length == 0 || !t.place(idx, t.root, int(length), &next) {
			return nil, fmt.Errorf("lsd: invalid Huffman code for symbol %d", idx)
		}
	}
	if count == 1 {
		return nil, fmt.Errorf("lsd: single-symbol Huffman tables are unsupported")
	}
	return t, nil
}

func bitWidth(v uint32) uint8 {
	var n uint8
	for v != 0 {
		n++
		v >>= 1
	}
	if n == 0 {
		return 1
	}
	return n
}

func (t *lengthTable) place(sym uint32, at, remaining int, next *int) bool {
	if at < 0 || at >= len(t.nodes) || remaining <= 0 {
		return false
	}
	for side := 0; side < 2; side++ {
		child := &t.nodes[at].left
		if side == 1 {
			child = &t.nodes[at].right
		}
		if remaining == 1 {
			if *child == 0 {
				*child = -int32(sym) - 1
				return true
			}
			continue
		}
		if *child == 0 {
			if *next >= len(t.nodes) {
				continue
			}
			*child = int32(*next + 1)
			*next++
		}
		if *child > 0 && t.place(sym, int(*child-1), remaining-1, next) {
			return true
		}
	}
	return false
}

func (t *lengthTable) decode(r *bitReader) (uint32, error) {
	if t == nil || t.root < 0 || t.root >= len(t.nodes) {
		return 0, fmt.Errorf("lsd: invalid Huffman tree")
	}
	at := t.root
	for steps := 0; steps <= len(t.nodes); steps++ {
		b, err := r.read(1)
		if err != nil {
			return 0, err
		}
		child := t.nodes[at].left
		if b != 0 {
			child = t.nodes[at].right
		}
		if child == 0 {
			return 0, fmt.Errorf("lsd: missing Huffman branch")
		}
		if child < 0 {
			return uint32(-child - 1), nil
		}
		at = int(child - 1)
	}
	return 0, fmt.Errorf("lsd: cyclic Huffman tree")
}
