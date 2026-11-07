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
	"github.com/DMSHLabs/DictShelf/internal/store"
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

func TestReconcileBuildsArticlesAndPacksOpaqueAssets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WUDICT_DB_DIR", filepath.Join(root, "library"))
	s := favorites.New(filepath.Join(root, "builtin"))
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	body := "<p>portable body</p><img src=\"photo.png\">"
	article, err := s.Add(favorites.Provenance{DictionaryID: "origin-id", DictionaryName: "Source", Headword: "word"}, body,
		[]favorites.AssetInput{{URL: "photo.png", Name: "photo.png", MIME: "image/png", Data: []byte("photo bytes")}})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	out, err := store.Reconcile(s.Path(), store.Target{FullText: &yes, Media: store.MediaOn}, store.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Rebuilt || out.Packed < 1 {
		t.Fatalf("reconcile = %+v", out)
	}
	prepared, err := store.Open(out.TextDB)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	rows, err := prepared.Exact("word", 2)
	if err != nil || len(rows) != 1 || rows[0].Body != body {
		t.Fatalf("prepared article = %+v, %v", rows, err)
	}
	rc, _, err := prepared.Resource(article.Assets["photo.png"].Token)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(got) != "photo bytes" {
		t.Fatalf("packed resource = %q, %v", got, err)
	}
}
