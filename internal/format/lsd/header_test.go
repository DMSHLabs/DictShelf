// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReadHeaderFixture(t *testing.T) {
	path := filepath.Join("testdata", "simple_testdict1", "test.lsd")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	h, err := readHeader(f, stat.Size())
	if err != nil {
		t.Fatal(err)
	}
	if h.version != 0x142001 || h.checksum != 0x9341A792 || h.entryCount != 3 {
		t.Fatalf("unexpected header: version=%#x checksum=%#x entries=%d", h.version, h.checksum, h.entryCount)
	}
	if h.sourceLanguageID != 1033 || h.contentsLanguageID != 1033 {
		t.Fatalf("unexpected language IDs: %d -> %d", h.sourceLanguageID, h.contentsLanguageID)
	}
}

func TestReadUserDecoderFixture(t *testing.T) {
	path := filepath.Join("testdata", "simple_testdict1", "test.lsd")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	h, err := readHeader(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	section := make([]byte, int64(h.articlesOffset-h.decoderOffset))
	if _, err := f.ReadAt(section, int64(h.decoderOffset)); err != nil {
		t.Fatal(err)
	}
	d, err := readDecoder(section, h.version)
	if err != nil {
		t.Fatalf("read user dictionary decoder: %v", err)
	}
	if len(d.articleSymbols) == 0 || len(d.headingSymbols) == 0 || d.ref2 == 0 {
		t.Fatalf("incomplete decoder tables: article=%d heading=%d ref2=%d", len(d.articleSymbols), len(d.headingSymbols), d.ref2)
	}
}

func TestReadHeaderRejectsTruncatedFile(t *testing.T) {
	if _, err := readHeader(nil, fixedHeaderSize-1); err == nil {
		t.Fatal("expected truncated header error")
	}
}

func TestReadHeadersFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "simple_testdict1", "*.lsd"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("fixture set has no LSD files")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readHeader(f, info.Size()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReadIconWithoutDecodingDictionary(t *testing.T) {
	icon := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	path := filepath.Join(t.TempDir(), "icon.lsd")
	writeIconOnlyLSD(t, path, icon)
	body, mime, err := ReadIcon(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(icon) || mime != "image/png" {
		t.Fatalf("ReadIcon = %x, %q; want %x, image/png", body, mime, icon)
	}
	d := &Dict{source: &source{icon: icon}}
	rc, mime, err := d.Icon()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, err = io.ReadAll(rc)
	if err != nil || string(body) != string(icon) || mime != "image/png" {
		t.Fatalf("Dict.Icon = %x, %q, %v", body, mime, err)
	}
}

func TestEmbeddedBMPIconMIME(t *testing.T) {
	icon := make([]byte, 14)
	copy(icon, "BM")
	if got := iconMIME(icon); got != "image/bmp" {
		t.Fatalf("iconMIME(BMP) = %q, want image/bmp", got)
	}
	path := filepath.Join(t.TempDir(), "icon.lsd")
	writeIconOnlyLSD(t, path, icon)
	body, mime, err := ReadIcon(path)
	if err != nil || string(body) != string(icon) || mime != "image/bmp" {
		t.Fatalf("ReadIcon(BMP) = %x, %q, %v; want original bytes, image/bmp", body, mime, err)
	}
	if got := iconMIME([]byte("not an image")); got != "" {
		t.Fatalf("iconMIME(text) = %q, want empty", got)
	}
}

func writeIconOnlyLSD(t *testing.T, path string, icon []byte) {
	t.Helper()
	variable := []byte{0, 0, 0, 0, 0, 0, 0}
	variable = append(variable, byte(len(icon)), byte(len(icon)>>8))
	variable = append(variable, icon...)
	decoderOffset := fixedHeaderSize + len(variable) + 4 + 4 + 4 + 8
	variable = append(variable, make([]byte, 4)...)
	var offsets [8]byte
	binary.LittleEndian.PutUint32(offsets[:4], uint32(decoderOffset))
	binary.LittleEndian.PutUint32(offsets[4:], uint32(decoderOffset))
	variable = append(variable, offsets[:]...)
	variable = append(variable, make([]byte, 8)...)
	file := make([]byte, decoderOffset+pageSize)
	copy(file[:8], []byte{'L', 'i', 'n', 'g', 'V', 'o'})
	binary.LittleEndian.PutUint32(file[8:12], 0x142001)
	binary.LittleEndian.PutUint32(file[24:28], fixedHeaderSize)
	binary.LittleEndian.PutUint32(file[28:32], uint32(decoderOffset))
	binary.LittleEndian.PutUint32(file[32:36], uint32(decoderOffset))
	binary.LittleEndian.PutUint32(file[36:40], uint32(decoderOffset))
	copy(file[fixedHeaderSize:], variable)
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
}
