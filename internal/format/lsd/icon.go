// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
)

// ReadIcon reads the embedded image from an LSD header without decoding the
// dictionary's article decoder tables.
func ReadIcon(path string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	h, err := readHeader(f, st.Size())
	if err != nil {
		return nil, "", err
	}
	if !supportedVersion(h.version) || h.version <= 0x120000 {
		return nil, "", dict.ErrNotFound
	}
	icon, err := readHeaderIcon(f, h, st.Size())
	if err != nil {
		return nil, "", err
	}
	if len(icon) == 0 {
		return nil, "", dict.ErrNotFound
	}
	mime := http.DetectContentType(icon)
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("lsd: embedded icon is not an image (%s)", mime)
	}
	return icon, mime, nil
}

func readHeaderIcon(f io.ReaderAt, h fileHeader, fileSize int64) ([]byte, error) {
	limit := int64(h.decoderOffset)
	if limit > fileSize || limit < fixedHeaderSize {
		return nil, fmt.Errorf("lsd: invalid variable header boundary")
	}
	r := io.NewSectionReader(f, fixedHeaderSize, limit-fixedHeaderSize)
	readByte := func() (byte, error) {
		var b [1]byte
		_, err := io.ReadFull(r, b[:])
		return b[0], err
	}
	readText := func(n uint32) error {
		if n > 1<<20 {
			return fmt.Errorf("lsd: variable header text is too large (%d UTF-16 units)", n)
		}
		_, err := io.CopyN(io.Discard, r, int64(n)*2)
		return err
	}
	readTextField := func(label string) error {
		n, err := readByte()
		if err != nil {
			return fmt.Errorf("lsd: read %s length: %w", label, err)
		}
		if err := readText(uint32(n)); err != nil {
			return fmt.Errorf("lsd: read %s: %w", label, err)
		}
		return nil
	}
	if err := readTextField("dictionary name"); err != nil {
		return nil, err
	}
	for _, label := range []string{"first heading", "last heading"} {
		if err := readTextField(label); err != nil {
			return nil, err
		}
	}
	var capitalsLen [4]byte
	if _, err := io.ReadFull(r, capitalsLen[:]); err != nil {
		return nil, fmt.Errorf("lsd: read capitals length: %w", err)
	}
	if err := readText(binary.LittleEndian.Uint32(capitalsLen[:])); err != nil {
		return nil, fmt.Errorf("lsd: read capitals: %w", err)
	}
	var iconLen [2]byte
	if _, err := io.ReadFull(r, iconLen[:]); err != nil {
		return nil, fmt.Errorf("lsd: read icon length: %w", err)
	}
	icon := make([]byte, binary.LittleEndian.Uint16(iconLen[:]))
	if _, err := io.ReadFull(r, icon); err != nil {
		return nil, fmt.Errorf("lsd: read icon: %w", err)
	}
	if h.version > 0x140000 {
		var checksum [4]byte
		if _, err := io.ReadFull(r, checksum[:]); err != nil {
			return nil, fmt.Errorf("lsd: read header checksum: %w", err)
		}
	}
	pagesEnd, overlayData := uint32(fileSize), uint32(fileSize)
	if h.version > 0x120000 {
		var offsets [8]byte
		if _, err := io.ReadFull(r, offsets[:]); err != nil {
			return nil, fmt.Errorf("lsd: read page and resource boundaries: %w", err)
		}
		pagesEnd = binary.LittleEndian.Uint32(offsets[:4])
		overlayData = binary.LittleEndian.Uint32(offsets[4:])
	}
	if h.version > 0x140000 {
		var reserved [8]byte
		if _, err := io.ReadFull(r, reserved[:]); err != nil {
			return nil, fmt.Errorf("lsd: read reserved header values: %w", err)
		}
	}
	if pagesEnd < h.pagesOffset || int64(pagesEnd) > fileSize || (pagesEnd-h.pagesOffset)%512 != 0 {
		return nil, fmt.Errorf("lsd: invalid index page boundary %#x", pagesEnd)
	}
	if overlayData < pagesEnd || int64(overlayData) > fileSize {
		return nil, fmt.Errorf("lsd: invalid resource data boundary %#x", overlayData)
	}
	return icon, nil
}
