// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favs

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/favorites"
)

func TestProvisionAndOpenEmptyFavorites(t *testing.T) {
	root := t.TempDir()
	s := favorites.New(root)
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	d, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if d.Meta().EntryCount != 0 {
		t.Fatalf("entries = %d", d.Meta().EntryCount)
	}
	r, err := NewReader(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("empty reader = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions")); err != nil {
		t.Fatal(err)
	}
}

func TestReaderPreservesSavedBodyBytes(t *testing.T) {
	root := t.TempDir()
	s := favorites.New(root)
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	body := "<p>first</p>\n\n<div>second</div>\n"
	if _, err := s.Add(favorites.Provenance{DictionaryID: "origin", Headword: "word"}, body, nil); err != nil {
		t.Fatal(err)
	}
	r, err := NewReader(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	e, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != dict.BodyHTML || e.Body != body {
		t.Fatalf("body bytes changed: %q", e.Body)
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("end = %v", err)
	}
}
