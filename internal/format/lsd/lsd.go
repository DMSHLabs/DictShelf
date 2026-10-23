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
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/lang"
	"github.com/wuweidict/wudict/internal/logx"
)

const pageSize = 512

// ReaderVersion tracks the behavior of the sequential ingest reader.
const ReaderVersion = 5

func init() {
	dict.RegisterFormat(".lsd", func(path string) (dict.Dictionary, error) { return Open(path) })
	dict.RegisterReader(".lsd", func(path string) (dict.Reader, error) { return NewReader(path) })
	dict.RegisterReaderVersion("lsd", ReaderVersion)
	dict.RegisterAbout("lsd", loadAnnotation)
}

func loadAnnotation(path string) (dict.About, bool) {
	s, err := openSource(path)
	if err != nil {
		return dict.About{}, false
	}
	defer s.f.Close()
	start, end := int64(s.header.annotationOffset), int64(s.header.decoderOffset)
	if end-start < 2 || end-start > 256<<10 {
		return dict.About{}, false
	}
	data := make([]byte, end-start)
	if _, err := s.f.ReadAt(data, start); err != nil {
		logx.V("lsd annotation read %s: %v", filepath.Base(path), err)
		return dict.About{}, false
	}
	if s.header.version == 0x151005 {
		x6XOR(data)
	}
	if len(data) < 2 {
		return dict.About{}, false
	}
	size := uint32(binary.BigEndian.Uint16(data[:2]))
	text, err := s.decoder.decodeArticle(data[2:], size)
	if err != nil || strings.TrimSpace(text) == "" {
		if err != nil {
			logx.V("lsd annotation decode %s: %v", filepath.Base(path), err)
		}
		return dict.About{}, false
	}
	return dict.About{Sections: []dict.Section{{Text: strings.TrimSpace(text)}}, Source: path}, true
}

type source struct {
	f           *os.File
	path        string
	fileSize    int64
	header      fileHeader
	decoder     *decoder
	meta        dict.Meta
	icon        []byte
	pagesEnd    uint32
	overlayData uint32
	overlayEnd  uint32
	overlayOnce sync.Once
	overlay     []overlayEntry
	overlayErr  error
}

func openSource(path string) (*source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*source, error) {
		_ = f.Close()
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	h, err := readHeader(f, st.Size())
	if err != nil {
		return fail(err)
	}
	if !supportedVersion(h.version) {
		return fail(fmt.Errorf("lsd: unsupported dictionary version %#x", h.version))
	}
	name, pagesEnd, overlayEnd, icon, err := readVariableHeader(f, h, st.Size())
	if err != nil {
		return fail(err)
	}
	decoderSize := h.articlesOffset - h.decoderOffset
	if decoderSize > 128<<20 {
		return fail(fmt.Errorf("lsd: decoder section is too large (%d bytes)", decoderSize))
	}
	decoderBytes := make([]byte, int(decoderSize))
	if _, err := f.ReadAt(decoderBytes, int64(h.decoderOffset)); err != nil {
		return fail(fmt.Errorf("lsd: read decoder section: %w", err))
	}
	if h.version == 0x151005 {
		x6XOR(decoderBytes)
	}
	d, err := readDecoder(decoderBytes, h.version)
	if err != nil {
		return fail(err)
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	indexLang := strings.SplitN(lang.FromLCID(int(h.sourceLanguageID)), "-", 2)[0]
	contentsLang := strings.SplitN(lang.FromLCID(int(h.contentsLanguageID)), "-", 2)[0]
	overlayData := overlayEnd
	if h.version < 0x140000 {
		// In Lingvo 12 and X3, resource offsets are absolute file offsets.
		overlayData = 0
	}
	return &source{
		f: f, path: path, fileSize: st.Size(), header: h, decoder: d, icon: icon, pagesEnd: pagesEnd, overlayData: overlayData, overlayEnd: overlayEnd,
		meta: dict.Meta{Name: name, Format: "lsd", Path: path, EntryCount: int(h.entryCount), IndexLang: indexLang, ContentsLang: contentsLang},
	}, nil
}

func supportedVersion(version uint32) bool {
	switch version {
	case 0x110001, 0x120001, 0x131001, 0x132001, 0x141004, 0x142001, 0x145001, 0x151005, 0x152001, 0x155001:
		return true
	default:
		return false
	}
}

func readVariableHeader(f io.ReaderAt, h fileHeader, fileSize int64) (string, uint32, uint32, []byte, error) {
	limit := int64(h.decoderOffset)
	if limit > fileSize || limit < fixedHeaderSize {
		return "", 0, 0, nil, fmt.Errorf("lsd: invalid variable header boundary")
	}
	r := io.NewSectionReader(f, fixedHeaderSize, limit-fixedHeaderSize)
	readByte := func() (byte, error) {
		var b [1]byte
		_, err := io.ReadFull(r, b[:])
		return b[0], err
	}
	readU16LE := func() (uint16, error) {
		var b [2]byte
		_, err := io.ReadFull(r, b[:])
		return binary.LittleEndian.Uint16(b[:]), err
	}
	readU32LE := func() (uint32, error) {
		var b [4]byte
		_, err := io.ReadFull(r, b[:])
		return binary.LittleEndian.Uint32(b[:]), err
	}
	readText := func(n uint32) (string, error) {
		if n > 1<<20 {
			return "", fmt.Errorf("lsd: variable header text is too large (%d UTF-16 units)", n)
		}
		words := make([]uint16, n)
		buf := make([]byte, int(n)*2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		for i := range words {
			words[i] = binary.LittleEndian.Uint16(buf[i*2:])
		}
		return string(utf16.Decode(words)), nil
	}
	nameLen, err := readByte()
	if err != nil {
		return "", 0, 0, nil, fmt.Errorf("lsd: read dictionary name length: %w", err)
	}
	name, err := readText(uint32(nameLen))
	if err != nil {
		return "", 0, 0, nil, fmt.Errorf("lsd: read dictionary name: %w", err)
	}
	for _, label := range []string{"first heading", "last heading"} {
		n, err := readByte()
		if err != nil {
			return "", 0, 0, nil, fmt.Errorf("lsd: read %s length: %w", label, err)
		}
		if _, err := readText(uint32(n)); err != nil {
			return "", 0, 0, nil, fmt.Errorf("lsd: read %s: %w", label, err)
		}
	}
	capitalsLen, err := readU32LE()
	if err != nil {
		return "", 0, 0, nil, fmt.Errorf("lsd: read capitals length: %w", err)
	}
	if _, err := readText(capitalsLen); err != nil {
		return "", 0, 0, nil, fmt.Errorf("lsd: read capitals: %w", err)
	}
	var icon []byte
	if h.version > 0x120000 {
		iconLen, err := readU16LE()
		if err != nil {
			return "", 0, 0, nil, fmt.Errorf("lsd: read icon length: %w", err)
		}
		if iconLen > 0 {
			icon = make([]byte, int(iconLen))
			if _, err := io.ReadFull(r, icon); err != nil {
				return "", 0, 0, nil, fmt.Errorf("lsd: read icon: %w", err)
			}
		}
	}
	if h.version > 0x140000 {
		var ignored [4]byte
		if _, err := io.ReadFull(r, ignored[:]); err != nil {
			return "", 0, 0, nil, fmt.Errorf("lsd: read header checksum: %w", err)
		}
	}
	pagesEnd := uint32(fileSize)
	overlayData := uint32(fileSize)
	if h.version > 0x120000 {
		pagesEnd, err = readU32LE()
		if err != nil {
			return "", 0, 0, nil, fmt.Errorf("lsd: read end of index pages: %w", err)
		}
		overlayData, err = readU32LE()
		if err != nil {
			return "", 0, 0, nil, fmt.Errorf("lsd: read resource data offset: %w", err)
		}
	}
	if h.version > 0x140000 {
		for i := 0; i < 2; i++ {
			if _, err := readU32LE(); err != nil {
				return "", 0, 0, nil, fmt.Errorf("lsd: read reserved header value: %w", err)
			}
		}
	}
	if pagesEnd < h.pagesOffset || int64(pagesEnd) > fileSize || (pagesEnd-h.pagesOffset)%pageSize != 0 {
		return "", 0, 0, nil, fmt.Errorf("lsd: invalid index page boundary %#x", pagesEnd)
	}
	if overlayData < pagesEnd || int64(overlayData) > fileSize {
		return "", 0, 0, nil, fmt.Errorf("lsd: invalid resource data boundary %#x", overlayData)
	}
	return strings.TrimSpace(name), pagesEnd, overlayData, icon, nil
}

// ReadIcon reads only the LSD headers, without decoding the dictionary's
// article decoder tables. This keeps dictionary-list rendering cheap even for
// large dictionaries.
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
	_, _, _, icon, err := readVariableHeader(f, h, st.Size())
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
