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

func TestCollectionsCreateDefaultAndPersistSiblingCollection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "favourites")
	collections, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	defaultStore, ok := collections.Store(ID)
	if !ok || defaultStore.Path() != filepath.Join(root, "fav", "text.db") {
		t.Fatalf("default store = %#v, want a store below fav", defaultStore)
	}
	if _, err := os.Stat(defaultStore.Path()); err != nil {
		t.Fatalf("default store was not initialized: %v", err)
	}
	created, err := collections.Create("Deutsch")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == ID || !strings.HasPrefix(created.ID, ID+"-") {
		t.Fatalf("new collection id = %q", created.ID)
	}
	if got := created.Path; got != filepath.Join(root, created.ID, "text.db") {
		t.Fatalf("new collection path = %q", got)
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

func TestCollectionsCreateDefaultForMissingAndEmptyRoots(t *testing.T) {
	for _, rootKind := range []string{"missing", "empty", "empty-catalogue"} {
		t.Run(rootKind, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "favourites")
			if rootKind != "missing" {
				if err := os.MkdirAll(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if rootKind == "empty-catalogue" {
				if err := os.WriteFile(filepath.Join(root, "collections.json"), []byte(`{"version":3,"ids":[]}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			collections, err := OpenCollections(root, "Favourites")
			if err != nil {
				t.Fatal(err)
			}
			if ids := collections.IDs(); len(ids) != 1 || ids[0] != ID {
				t.Fatalf("collection IDs = %v, want default %q", ids, ID)
			}
			set, ok := collections.Store(ID)
			if !ok {
				t.Fatal("default collection was not created")
			}
			if _, err := os.Stat(set.Path()); err != nil {
				t.Fatalf("default collection file was not initialized: %v", err)
			}
		})
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
	if err := os.WriteFile(filepath.Join(root, "collections.json"), []byte(`{"version":3,"ids":["favourites-../../outside"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCollections(root, "Favourites"); err == nil {
		t.Fatal("unsafe collection id was accepted")
	}
}

func TestCollectionsCanDeleteAnyButLastCollection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "favourites")
	collections, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	created, err := collections.Create("Deutsch")
	if err != nil {
		t.Fatal(err)
	}
	if err := collections.Delete(ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := collections.Store(ID); ok {
		t.Fatal("deleted default collection remains available")
	}
	if _, err := os.Stat(filepath.Join(root, "fav", "text.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted default file status = %v", err)
	}
	if err := collections.Delete(created.ID); err == nil {
		t.Fatal("last collection was deleted")
	}
	list, err := collections.List()
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("collections after delete = %+v, %v", list, err)
	}
	reloaded, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	if ids := reloaded.IDs(); len(ids) != 1 || ids[0] != created.ID {
		t.Fatalf("reloaded collections = %v", ids)
	}
}

func TestCollectionsRenameUpdatesNamesAndRejectsDuplicates(t *testing.T) {
	collections, err := OpenCollections(filepath.Join(t.TempDir(), "favourites"), "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	created, err := collections.Create("Deutsch")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collections.Rename(created.ID, "German"); err != nil {
		t.Fatal(err)
	}
	if _, err := collections.Rename(ID, "german"); err == nil {
		t.Fatal("duplicate collection name was accepted")
	}
	list, err := collections.List()
	if err != nil || len(list) != 2 || list[0].Name != "Favourites" || list[1].Name != "German" {
		t.Fatalf("renamed collection list = %+v, %v", list, err)
	}
	if _, err := collections.Rename(created.ID, " "); err == nil {
		t.Fatal("empty collection name was accepted")
	}
}

func TestCollectionDescriptionPersistsAcrossReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "favourites")
	collections, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collections.SetAbout(ID, "My articles\nEnglish and German"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenCollections(root, "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	list, err := reloaded.List()
	if err != nil || len(list) != 1 || list[0].About != "My articles\nEnglish and German" {
		t.Fatalf("reloaded description = %+v, %v", list, err)
	}
}
