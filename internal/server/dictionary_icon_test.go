// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestDictionaryIconPrefersSiblingThenUsesEmbeddedLSDIcon(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "sample.lsd")
	embedded := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	writeIconOnlyLSDForServerTest(t, source, embedded)
	s := &Server{}
	e := &entry{Path: source}

	body, mime := s.dictionaryIcon(e)
	if string(body) != string(embedded) || mime != "image/png" {
		t.Fatalf("embedded icon = %x, %q; want %x, image/png", body, mime, embedded)
	}

	sibling := []byte("GIF89a")
	if err := os.WriteFile(filepath.Join(dir, "sample.gif"), sibling, 0o600); err != nil {
		t.Fatal(err)
	}
	body, mime = s.dictionaryIcon(e)
	if string(body) != string(sibling) || mime != "image/gif" {
		t.Fatalf("sibling icon = %x, %q; want %x, image/gif", body, mime, sibling)
	}
}

func writeIconOnlyLSDForServerTest(t *testing.T, path string, icon []byte) {
	t.Helper()
	const fixedHeaderSize = 52
	variable := []byte{0, 0, 0, 0, 0, 0, 0, byte(len(icon)), byte(len(icon) >> 8)}
	variable = append(variable, icon...)
	decoderOffset := fixedHeaderSize + len(variable) + 4 + 4 + 4 + 8
	variable = append(variable, make([]byte, 4)...)
	var offsets [8]byte
	binary.LittleEndian.PutUint32(offsets[:4], uint32(decoderOffset))
	binary.LittleEndian.PutUint32(offsets[4:], uint32(decoderOffset))
	variable = append(variable, offsets[:]...)
	variable = append(variable, make([]byte, 8)...)
	file := make([]byte, decoderOffset+512)
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
