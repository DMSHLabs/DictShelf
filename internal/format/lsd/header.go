// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const fixedHeaderSize = 52

type fileHeader struct {
	version            uint32
	checksum           uint32
	entryCount         uint32
	annotationOffset   uint32
	decoderOffset      uint32
	articlesOffset     uint32
	pagesOffset        uint32
	lastPage           uint16
	sourceLanguageID   uint16
	contentsLanguageID uint16
}

// readHeader validates the fixed LSD header without reading the rest of the
// file. LSD's fixed-width fields are little-endian; the later bitstream is
// handled by the decoder layer.
func readHeader(r io.ReaderAt, fileSize int64) (fileHeader, error) {
	if fileSize < fixedHeaderSize {
		return fileHeader{}, fmt.Errorf("lsd: file is shorter than its fixed header")
	}
	buf := make([]byte, fixedHeaderSize)
	if _, err := r.ReadAt(buf, 0); err != nil {
		return fileHeader{}, fmt.Errorf("lsd: read fixed header: %w", err)
	}
	magic := strings.ReplaceAll(string(buf[:8]), "\x00", "")
	if magic != "LingVo" {
		return fileHeader{}, fmt.Errorf("lsd: invalid signature %q", magic)
	}
	h := fileHeader{
		version:            binary.LittleEndian.Uint32(buf[8:12]),
		checksum:           binary.LittleEndian.Uint32(buf[16:20]),
		entryCount:         binary.LittleEndian.Uint32(buf[20:24]),
		annotationOffset:   binary.LittleEndian.Uint32(buf[24:28]),
		decoderOffset:      binary.LittleEndian.Uint32(buf[28:32]),
		articlesOffset:     binary.LittleEndian.Uint32(buf[32:36]),
		pagesOffset:        binary.LittleEndian.Uint32(buf[36:40]),
		lastPage:           binary.LittleEndian.Uint16(buf[44:46]),
		sourceLanguageID:   binary.LittleEndian.Uint16(buf[48:50]),
		contentsLanguageID: binary.LittleEndian.Uint16(buf[50:52]),
	}
	for name, off := range map[string]uint32{
		"annotation": h.annotationOffset,
		"decoder":    h.decoderOffset,
		"articles":   h.articlesOffset,
		"pages":      h.pagesOffset,
	} {
		if uint64(off) >= uint64(fileSize) {
			return fileHeader{}, fmt.Errorf("lsd: %s offset %d is outside file (%d bytes)", name, off, fileSize)
		}
	}
	if h.annotationOffset > h.decoderOffset || h.decoderOffset > h.articlesOffset || h.articlesOffset > h.pagesOffset {
		return fileHeader{}, fmt.Errorf("lsd: invalid section offsets")
	}
	return h, nil
}

func supportedVersion(version uint32) bool {
	switch version {
	case 0x110001, 0x120001, 0x131001, 0x132001, 0x141004, 0x142001, 0x145001, 0x151005, 0x152001, 0x155001:
		return true
	default:
		return false
	}
}
