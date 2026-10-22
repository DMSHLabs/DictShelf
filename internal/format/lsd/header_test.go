// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
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
