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
