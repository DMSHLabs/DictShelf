// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDictionaryIconUsesFallbackWhenDictionaryHasNoIcon(t *testing.T) {
	source := filepath.Join(t.TempDir(), "plain.dsl")
	if err := os.WriteFile(source, []byte("plain text dictionary"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &entry{ID: "plain", Path: source}
	s := &Server{reg: &Registry{byID: map[string]*entry{"plain": e}}}
	var w *httptest.ResponseRecorder
	deadline := time.Now().Add(5 * time.Second)
	for {
		w = httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/dict-icon/plain", nil)
		s.handleDictionaryIcon(w, r)
		if w.Code != http.StatusAccepted || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("Content-Type = %q, want image/svg+xml", got)
	}
	if w.Body.String() != defaultDictionaryIcon {
		t.Fatal("fallback dictionary icon was not returned")
	}
}

func TestDictionaryIconPrefersSiblingThenUsesEmbeddedLSDIcon(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "sample.lsd")
	embedded := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	writeIconOnlyLSDForServerTest(t, source, embedded)
	body, mime := sourceDictionaryIcon(source, "")
	if string(body) != string(embedded) || mime != "image/png" {
		t.Fatalf("embedded icon = %x, %q; want %x, image/png", body, mime, embedded)
	}

	sibling := []byte("GIF89a")
	if err := os.WriteFile(filepath.Join(dir, "sample.gif"), sibling, 0o600); err != nil {
		t.Fatal(err)
	}
	body, mime = sourceDictionaryIcon(source, filepath.Join(dir, "sample.gif"))
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
