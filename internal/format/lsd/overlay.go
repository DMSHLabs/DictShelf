// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/resource"
)

const maxResourceBytes = 256 << 20

type overlayEntry struct {
	name         string
	offset       uint32
	inflatedSize uint32
	streamSize   uint32
}

func (s *source) readOverlay() ([]overlayEntry, error) {
	if s.overlayEnd == uint32(s.fileSize) || s.overlayEnd == s.pagesEnd {
		return nil, nil
	}
	sectionSize := int64(s.overlayEnd - s.pagesEnd)
	if sectionSize < 4 || sectionSize > 64<<20 {
		return nil, fmt.Errorf("lsd: invalid resource index size %d", sectionSize)
	}
	r := io.NewSectionReader(s.f, int64(s.pagesEnd), sectionSize)
	readU32 := func() (uint32, error) {
		var b [4]byte
		_, err := io.ReadFull(r, b[:])
		return binary.LittleEndian.Uint32(b[:]), err
	}
	count, err := readU32()
	if err != nil {
		return nil, fmt.Errorf("lsd: read resource count: %w", err)
	}
	if count > 1<<20 {
		return nil, fmt.Errorf("lsd: resource count is too large (%d)", count)
	}
	entries := make([]overlayEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return nil, fmt.Errorf("lsd: read resource %d name length: %w", i, err)
		}
		nameBytes := make([]byte, int(b[0])*2)
		if _, err := io.ReadFull(r, nameBytes); err != nil {
			return nil, fmt.Errorf("lsd: read resource %d name: %w", i, err)
		}
		words := make([]uint16, int(b[0]))
		for j := range words {
			words[j] = binary.LittleEndian.Uint16(nameBytes[j*2:])
		}
		values := [4]uint32{}
		for j := range values {
			values[j], err = readU32()
			if err != nil {
				return nil, fmt.Errorf("lsd: read resource %d metadata: %w", i, err)
			}
		}
		entry := overlayEntry{name: string(utf16.Decode(words)), offset: values[0], inflatedSize: values[2], streamSize: values[3]}
		if entry.inflatedSize == 0 {
			continue
		}
		if entry.inflatedSize > maxResourceBytes || uint64(entry.offset)+uint64(entry.streamSize) > uint64(s.fileSize)-uint64(s.overlayData) {
			return nil, fmt.Errorf("lsd: invalid resource bounds for %q", entry.name)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *source) overlayEntries() ([]overlayEntry, error) {
	s.overlayOnce.Do(func() { s.overlay, s.overlayErr = s.readOverlay() })
	return s.overlay, s.overlayErr
}

func (s *source) resource(name string) (io.ReadCloser, string, error) {
	key := resource.Key(name)
	if key == "" {
		return nil, "", dict.ErrNotFound
	}
	entries, err := s.overlayEntries()
	if err != nil {
		return nil, "", err
	}
	for _, e := range entries {
		if resource.Key(e.name) != key {
			continue
		}
		compressed := make([]byte, int(e.streamSize))
		offset := int64(s.overlayData) + int64(e.offset)
		if _, err := s.f.ReadAt(compressed, offset); err != nil && err != io.EOF {
			return nil, "", fmt.Errorf("lsd: read resource %q: %w", e.name, err)
		}
		zr, err := zlib.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, "", fmt.Errorf("lsd: open compressed resource %q: %w", e.name, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(zr, int64(e.inflatedSize)+1))
		closeErr := zr.Close()
		if readErr != nil {
			return nil, "", fmt.Errorf("lsd: inflate resource %q: %w", e.name, readErr)
		}
		if closeErr != nil {
			return nil, "", fmt.Errorf("lsd: close resource %q: %w", e.name, closeErr)
		}
		if len(data) != int(e.inflatedSize) {
			return nil, "", fmt.Errorf("lsd: resource %q expanded to %d bytes; expected %d", e.name, len(data), e.inflatedSize)
		}
		return resource.Bytes(data), resource.MIME(e.name), nil
	}
	return nil, "", dict.ErrNotFound
}

func (d *Dict) Resources() []string {
	entries, err := d.source.overlayEntries()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	seen := map[string]bool{}
	for _, e := range entries {
		key := resource.Key(e.name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, e.name)
	}
	return names
}
