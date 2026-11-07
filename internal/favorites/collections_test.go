// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectionsKeepLegacyStoreAndPersistNewCollection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "favourites")
	legacy := New(root)
	if err := legacy.EnsureNamed("Избранное"); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Add(Provenance{DictionaryID: "source", Headword: "слово"}, "saved article", nil); err != nil {
		t.Fatal(err)
	}

	collections, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	created, err := collections.Create("Deutsch")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == ID || !strings.HasPrefix(created.ID, ID+"-") {
		t.Fatalf("new collection id = %q", created.ID)
	}
	if got := created.Path; got != filepath.Join(root, "collections", created.ID, created.ID+".favs") {
		t.Fatalf("new collection path = %q", got)
	}

	defaultStore, ok := collections.Store(ID)
	if !ok {
		t.Fatal("legacy collection is missing")
	}
	articles, err := defaultStore.Manifest()
	if err != nil || len(articles) != 1 || articles[0].Headword != "слово" {
		t.Fatalf("legacy articles = %+v, %v", articles, err)
	}

	reloaded, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	info, ok := reloaded.Store(created.ID)
	if !ok {
		t.Fatal("created collection was not restored")
	}
	name, err := info.Name()
	if err != nil || name != "Deutsch" {
		t.Fatalf("restored collection name = %q, %v", name, err)
	}
	list, err := reloaded.List()
	if err != nil || len(list) != 2 || list[0].ID != ID || list[1].ID != created.ID {
		t.Fatalf("collection list = %+v, %v", list, err)
	}
}

func TestCollectionsRejectInvalidNamesAndDuplicates(t *testing.T) {
	collections, err := OpenCollections(filepath.Join(t.TempDir(), "favourites"), "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", " ", strings.Repeat("a", 101)} {
		if _, err := collections.Create(name); err == nil {
			t.Errorf("Create(%d characters) succeeded", len(name))
		}
	}
	if _, err := collections.Create("Deutsch"); err != nil {
		t.Fatal(err)
	}
	if _, err := collections.Create("deutsch"); err == nil {
		t.Fatal("duplicate collection name was accepted")
	}
	if _, err := os.Stat(filepath.Join(collections.root, "collections.json")); err != nil {
		t.Fatalf("catalogue was not written: %v", err)
	}
}

func TestCollectionsRejectUnsafeCatalogueIDs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "favourites")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "collections.json"), []byte(`{"version":1,"ids":["favourites-../../outside"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCollections(root, "Favourites"); err == nil {
		t.Fatal("unsafe collection id was accepted")
	}
}

func TestCollectionsDeleteKeepsDefaultAndRemovesCustom(t *testing.T) {
	root := filepath.Join(t.TempDir(), "favourites")
	collections, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	if err := collections.Delete(ID); err == nil {
		t.Fatal("default collection was deleted")
	}
	created, err := collections.Create("Deutsch")
	if err != nil {
		t.Fatal(err)
	}
	if err := collections.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := collections.Store(created.ID); ok {
		t.Fatal("deleted collection remains available")
	}
	if _, err := os.Stat(created.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted collection file status = %v", err)
	}
	list, err := collections.List()
	if err != nil || len(list) != 1 || list[0].ID != ID {
		t.Fatalf("collections after delete = %+v, %v", list, err)
	}
	reloaded, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	if ids := reloaded.IDs(); len(ids) != 1 || ids[0] != ID {
		t.Fatalf("reloaded collections = %v", ids)
	}
}
