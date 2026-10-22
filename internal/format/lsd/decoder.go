// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

type decoder struct {
	prefix         []uint16
	articleSymbols []uint32
	headingSymbols []uint32
	articles       *lengthTable
	headings       *lengthTable
	prefixLengths  *lengthTable
	postfixLengths *lengthTable
	ref1, ref2     uint32
	systemArticles bool
}

type headingChar struct {
	value   uint16
	sorted  bool
	escaped bool
}

type extChar struct {
	index uint8
	value uint16
}

func readDecoder(data []byte, version uint32) (*decoder, error) {
	// User, system and abbreviation dictionaries share the bit/Huffman
	// machinery but differ in their symbol alphabets and table ordering.
	switch version {
	case 0x110001, 0x120001, 0x131001, 0x132001, 0x141004, 0x142001, 0x145001, 0x152001, 0x155001:
	default:
		return nil, fmt.Errorf("lsd: decoder version %#x is not supported yet", version)
	}
	r := &bitReader{data: data}
	prefixLen, err := r.readU32BE()
	if err != nil {
		return nil, fmt.Errorf("lsd: read dictionary prefix length: %w", err)
	}
	if prefixLen > 1<<20 {
		return nil, fmt.Errorf("lsd: dictionary prefix is too large (%d UTF-16 units)", prefixLen)
	}
	prefix := make([]uint16, prefixLen)
	for i := range prefix {
		v, err := r.read(16)
		if err != nil {
			return nil, fmt.Errorf("lsd: read dictionary prefix: %w", err)
		}
		if version == 0x145001 || version == 0x155001 {
			v ^= 0x879a
		}
		prefix[i] = uint16(v)
	}
	xoredSymbols := version == 0x145001 || version == 0x155001
	a, err := readSymbols(r, xoredSymbols)
	if err != nil {
		return nil, fmt.Errorf("lsd: read article symbols: %w", err)
	}
	h, err := readSymbols(r, xoredSymbols)
	if err != nil {
		return nil, fmt.Errorf("lsd: read heading symbols: %w", err)
	}
	at, err := readLengthTable(r)
	if err != nil {
		return nil, fmt.Errorf("lsd: read article Huffman table: %w", err)
	}
	ht, err := readLengthTable(r)
	if err != nil {
		return nil, fmt.Errorf("lsd: read heading Huffman table: %w", err)
	}
	var pt, qt *lengthTable
	if version == 0x141004 {
		qt, err = readLengthTable(r)
		if err == nil {
			_, err = r.read(32) // system decoder's reserved word
		}
		if err == nil {
			pt, err = readLengthTable(r)
		}
	} else {
		pt, err = readLengthTable(r)
		if err == nil {
			qt, err = readLengthTable(r)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("lsd: read heading length tables: %w", err)
	}
	r1, err := r.read(32)
	if err != nil {
		return nil, fmt.Errorf("lsd: read first reference number: %w", err)
	}
	r2, err := r.read(32)
	if err != nil {
		return nil, fmt.Errorf("lsd: read second reference number: %w", err)
	}
	return &decoder{
		prefix: prefix, articleSymbols: a, headingSymbols: h,
		articles: at, headings: ht, prefixLengths: pt, postfixLengths: qt,
		ref1: r1, ref2: r2, systemArticles: version == 0x131001 || version == 0x141004,
	}, nil
}

func readSymbols(r *bitReader, xor bool) ([]uint32, error) {
	n, err := r.read(32)
	if err != nil {
		return nil, err
	}
	w, err := r.read(8)
	if err != nil {
		return nil, err
	}
	if n == 0 || n > 1<<20 || w == 0 || w > 32 {
		return nil, fmt.Errorf("invalid symbol table dimensions (%d symbols, %d bits)", n, w)
	}
	symbols := make([]uint32, n)
	for i := range symbols {
		v, err := r.read(uint8(w))
		if err != nil {
			return nil, err
		}
		if xor {
			v ^= 0x1325
		}
		symbols[i] = v
	}
	return symbols, nil
}

func (d *decoder) readRef(r *bitReader) (uint32, error) {
	code, err := r.read(2)
	if err != nil {
		return 0, err
	}
	if code == 3 {
		return r.read(32)
	}
	n := bitWidth(d.ref2)
	if n < 2 {
		return 0, fmt.Errorf("lsd: invalid reference width %d", n)
	}
	low, err := r.read(n - 2)
	if err != nil {
		return 0, err
	}
	return code<<(n-2) | low, nil
}

func (d *decoder) readHeading(r *bitReader, knownPrefix []uint16) (string, []uint16, uint32, error) {
	pl, err := d.prefixLengths.decode(r)
	if err != nil {
		return "", nil, 0, fmt.Errorf("prefix length: %w", err)
	}
	ql, err := d.postfixLengths.decode(r)
	if err != nil {
		return "", nil, 0, fmt.Errorf("postfix length: %w", err)
	}
	if pl > uint32(len(knownPrefix)) || ql > 1<<16 {
		return "", nil, 0, fmt.Errorf("lsd: invalid heading lengths %d + %d", pl, ql)
	}
	out := append([]uint16(nil), knownPrefix[:pl]...)
	for i := uint32(0); i < ql; i++ {
		idx, err := d.headings.decode(r)
		if err != nil {
			return "", nil, 0, fmt.Errorf("heading symbol: %w", err)
		}
		if idx >= uint32(len(d.headingSymbols)) || d.headingSymbols[idx] > 0xffff {
			return "", nil, 0, fmt.Errorf("lsd: invalid heading symbol index/value %d", idx)
		}
		out = append(out, uint16(d.headingSymbols[idx]))
	}
	ref, err := d.readRef(r)
	if err != nil {
		return "", nil, 0, fmt.Errorf("article reference: %w", err)
	}
	unsorted, err := r.read(1)
	if err != nil {
		return "", nil, 0, err
	}
	var extensions []extChar
	if unsorted != 0 {
		n, err := r.read(8)
		if err != nil {
			return "", nil, 0, err
		}
		extensions = make([]extChar, 0, n)
		for i := uint32(0); i < n; i++ {
			index, err := r.read(8)
			if err != nil {
				return "", nil, 0, err
			}
			value, err := r.read(16)
			if err != nil {
				return "", nil, 0, err
			}
			extensions = append(extensions, extChar{index: uint8(index), value: uint16(value)})
		}
	}
	return dslHeading(out, extensions), out, ref, nil
}

func dslHeading(sorted []uint16, extensions []extChar) string {
	var chars []headingChar
	textPos, extPos, index := 0, 0, uint16(0)
	nextChar := func() (headingChar, bool) {
		if extPos < len(extensions) && uint16(extensions[extPos].index) == index {
			c := headingChar{value: extensions[extPos].value, sorted: false}
			extPos++
			return c, true
		}
		if textPos >= len(sorted) {
			return headingChar{}, false
		}
		c := headingChar{value: sorted[textPos], sorted: true}
		textPos++
		return c, true
	}
	for textPos < len(sorted) || extPos < len(extensions) {
		c, ok := nextChar()
		if !ok {
			break
		}
		if c.value == '\\' {
			index++
			next, ok := nextChar()
			if !ok {
				chars = append(chars, c)
				break
			}
			next.escaped = true
			chars = append(chars, next)
		} else {
			chars = append(chars, c)
		}
		index++
	}
	var b strings.Builder
	unsorted := false
	for _, c := range chars {
		if !c.sorted && !unsorted {
			b.WriteByte('{')
			unsorted = true
		} else if c.sorted && unsorted {
			b.WriteByte('}')
			unsorted = false
		}
		if c.escaped {
			b.WriteByte('\\')
		}
		b.WriteString(string(utf16.Decode([]uint16{c.value})))
	}
	if unsorted {
		b.WriteByte('}')
	}
	return b.String()
}

func (d *decoder) decodeArticle(data []byte, size uint32) (string, error) {
	if size > 64<<20 {
		return "", fmt.Errorf("lsd: article is too large (%d UTF-16 units)", size)
	}
	r := &bitReader{data: data}
	out := make([]uint16, 0, size)
	for uint32(len(out)) < size {
		idx, err := d.articles.decode(r)
		if err != nil {
			return "", fmt.Errorf("article symbol: %w", err)
		}
		if idx >= uint32(len(d.articleSymbols)) {
			return "", fmt.Errorf("lsd: invalid article symbol index %d", idx)
		}
		sym := d.articleSymbols[idx]
		if d.systemArticles {
			switch {
			case sym <= 0x3f:
				length := sym + 3
				start, err := r.read(bitWidth(uint32(len(d.prefix))))
				if err != nil {
					return "", err
				}
				if uint64(start)+uint64(length) > uint64(len(d.prefix)) {
					return "", fmt.Errorf("lsd: system article prefix copy is out of range")
				}
				out = append(out, d.prefix[start:start+length]...)
			case sym <= 0x80:
				length := sym - 0x3d
				start, err := r.read(bitWidth(size))
				if err != nil {
					return "", err
				}
				if uint64(start)+uint64(length) > uint64(len(out)) {
					return "", fmt.Errorf("lsd: system article back-reference is out of range")
				}
				for i := uint32(0); i < length; i++ {
					out = append(out, out[start+i])
				}
			default:
				out = append(out, uint16(sym-0x80))
			}
			if uint32(len(out)) > size {
				return "", fmt.Errorf("lsd: decoded article exceeds declared length")
			}
			continue
		}
		switch {
		case sym < 0x10000:
			out = append(out, uint16(sym))
		case sym < 0x10040:
			length := sym - 0xfffd
			start, err := r.read(bitWidth(uint32(len(d.prefix))))
			if err != nil {
				return "", err
			}
			if uint64(start)+uint64(length) > uint64(len(d.prefix)) {
				return "", fmt.Errorf("lsd: article prefix copy is out of range")
			}
			out = append(out, d.prefix[start:start+length]...)
		default:
			length := sym - 0x1003d
			start, err := r.read(bitWidth(size))
			if err != nil {
				return "", err
			}
			if uint64(start)+uint64(length) > uint64(len(out)) {
				return "", fmt.Errorf("lsd: article back-reference is out of range")
			}
			for i := uint32(0); i < length; i++ {
				out = append(out, out[start+i])
			}
		}
		if uint32(len(out)) > size {
			return "", fmt.Errorf("lsd: decoded article exceeds declared length")
		}
	}
	return string(utf16.Decode(out)), nil
}

func articleSize(data []byte) (uint32, []byte, error) {
	if len(data) < 2 {
		return 0, nil, fmt.Errorf("lsd: truncated article length")
	}
	n := uint32(binary.BigEndian.Uint16(data[:2]))
	if n == 0xffff {
		if len(data) < 6 {
			return 0, nil, fmt.Errorf("lsd: truncated extended article length")
		}
		n = binary.BigEndian.Uint32(data[2:6])
		return n, data[6:], nil
	}
	return n, data[2:], nil
}
