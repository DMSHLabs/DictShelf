// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/favorites"
	"github.com/DMSHLabs/DictShelf/internal/format/zim"
	"github.com/DMSHLabs/DictShelf/internal/htmlref"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

func TestFavouriteSnapshotsRoundTripThroughServerAndPreparedMedia(t *testing.T) {
	s := newTestServer(t)
	sources := getDicts(t, s, "/api/dicts")
	if len(sources) != 1 {
		t.Fatalf("source dictionaries = %d", len(sources))
	}
	root := t.TempDir()
	s.favourites = favorites.New(filepath.Join(root, "favourites"))
	if err := s.favourites.EnsureNamed("Избранное"); err != nil {
		t.Fatal(err)
	}
	s.reg.AddBuiltin(Builtin{ID: favorites.ID, Path: s.favourites.Path()})
	if err := s.reg.rescan(false); err != nil {
		t.Fatal(err)
	}

	requestBody, _ := json.Marshal(favouriteRequest{Dictionary: sources[0].ID, Headword: "corazón"})
	added := httptestNew(s, http.MethodPost, "/api/favourites", requestBody)
	if added.Code != http.StatusOK {
		t.Fatalf("add favourite status %d: %s", added.Code, added.Body.String())
	}
	articles, err := s.favourites.Manifest()
	if err != nil || len(articles) != 1 {
		t.Fatalf("saved snapshots = %d, %v", len(articles), err)
	}
	article := articles[0]
	if article.Provenance.DictionaryID != sources[0].ID || article.Provenance.DictionaryName != "Server Test Dict" || article.Headword != "corazón" {
		t.Fatalf("provenance = %+v", article.Provenance)
	}
	asset, ok := article.Assets["beat.mp3"]
	if !ok {
		t.Fatalf("saved assets = %+v; source body refs = %+v", article.Assets, (htmlref.Rewriter{}).Refs(articleBody(t, s, sources[0].ID)))
	}
	dir, ok := store.LookupDir(s.favourites.Path())
	if !ok {
		t.Fatal("favourites was not prepared")
	}
	prepared, err := store.Open(store.TextDBPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := prepared.Exact("corazón", 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("prepared lookup = %+v, %v", rows, err)
	}
	if rows[0].Body != articleBody(t, s, sources[0].ID) {
		t.Fatal("snapshot body differs from its source")
	}
	resource, _, err := prepared.Resource(asset.Token)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resource)
	resource.Close()
	prepared.Close()
	if err != nil || !bytes.Equal(data, []byte("MP3DATA")) {
		t.Fatalf("saved media = %q, %v", data, err)
	}

	search := collectLookup(t, s, SearchRequest{Query: "corazón", Mode: "exact", Dictionary: sources[0].ID})
	if len(search) < 2 || len(search[1].Results) != 1 || !search[1].Results[0].Favorite {
		t.Fatalf("source favourite state = %+v", search)
	}
	copySearch := collectLookup(t, s, SearchRequest{Query: "corazón", Mode: "exact", Dictionary: favorites.ID})
	if len(copySearch) < 2 || len(copySearch[1].Results) != 1 || copySearch[1].Results[0].FavoriteID != article.ID {
		t.Fatalf("saved snapshot result = %+v", copySearch)
	}

	removeBody, _ := json.Marshal(favouriteRequest{FavoriteID: article.ID})
	removed := httptestNew(s, http.MethodDelete, "/api/favourites", removeBody)
	if removed.Code != http.StatusOK {
		t.Fatalf("remove status %d: %s", removed.Code, removed.Body.String())
	}
	articles, err = s.favourites.Manifest()
	if err != nil || len(articles) != 0 {
		t.Fatalf("snapshots after removal = %d, %v", len(articles), err)
	}
	if _, err := os.Stat(store.TextDBPath(dir)); err != nil {
		t.Fatalf("prepared empty dictionary missing: %v", err)
	}
}

func TestFavouriteResourceNameUnescapesURLPathOnlyOnce(t *testing.T) {
	ref := htmlref.Ref{Tag: "img", Attr: "src", URL: "_assets_/hash/East_Side_School%E2%80%93Old_Hammond_High_School%252C_Louisiana.jpg"}
	name, ok := favouriteResourceName(ref)
	if !ok {
		t.Fatal("encoded ZIM resource was rejected")
	}
	want := "_assets_/hash/East_Side_School–Old_Hammond_High_School%2C_Louisiana.jpg"
	if name != want {
		t.Fatalf("favouriteResourceName = %q, want %q", name, want)
	}
}

func TestFavouriteCollectionsCreateSaveSearchAndRemove(t *testing.T) {
	s := newTestServer(t)
	sources := getDicts(t, s, "/api/dicts")
	if len(sources) != 1 {
		t.Fatalf("source dictionaries = %d", len(sources))
	}
	sets, err := favorites.OpenCollections(filepath.Join(t.TempDir(), "favourites"), "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	s.favSets = sets
	s.favourites, _ = sets.Store(favorites.ID)
	for _, collection := range mustCollectionList(t, sets) {
		store, _ := sets.Store(collection.ID)
		s.reg.AddBuiltin(Builtin{ID: collection.ID, Path: store.Path()})
	}
	if err := s.reg.rescan(false); err != nil {
		t.Fatal(err)
	}

	created := httptestNew(s, http.MethodPost, "/api/favourites/collections", []byte(`{"name":"Deutsch"}`))
	if created.Code != http.StatusCreated {
		t.Fatalf("create collection status %d: %s", created.Code, created.Body.String())
	}
	var collection favorites.CollectionInfo
	if err := json.Unmarshal(created.Body.Bytes(), &collection); err != nil || collection.ID == "" || collection.Name != "Deutsch" {
		t.Fatalf("created collection = %+v, %v", collection, err)
	}
	listed := httptestNew(s, http.MethodGet, "/api/favourites/collections", nil)
	var list struct {
		Collections []favorites.CollectionInfo `json:"collections"`
	}
	if listed.Code != http.StatusOK || json.Unmarshal(listed.Body.Bytes(), &list) != nil || len(list.Collections) != 2 || list.Collections[1].ID != collection.ID {
		t.Fatalf("collection list status %d: %s", listed.Code, listed.Body.String())
	}
	duplicate := httptestNew(s, http.MethodPost, "/api/favourites/collections", []byte(`{"name":"deutsch"}`))
	if duplicate.Code != http.StatusBadRequest {
		t.Fatalf("duplicate collection status %d: %s", duplicate.Code, duplicate.Body.String())
	}

	requestBody, _ := json.Marshal(favouriteRequest{Dictionary: sources[0].ID, Headword: "corazón", Collection: collection.ID})
	added := httptestNew(s, http.MethodPost, "/api/favourites", requestBody)
	if added.Code != http.StatusOK {
		t.Fatalf("add collection article status %d: %s", added.Code, added.Body.String())
	}
	copySearch := collectLookup(t, s, SearchRequest{Query: "corazón", Mode: "exact", Dictionary: collection.ID})
	if len(copySearch) < 2 || len(copySearch[1].Results) != 1 || copySearch[1].Results[0].FavoriteID == "" {
		t.Fatalf("saved article in custom collection = %+v", copySearch)
	}
	sourceSearch := collectLookup(t, s, SearchRequest{Query: "corazón", Mode: "exact", Dictionary: sources[0].ID})
	if len(sourceSearch) < 2 || len(sourceSearch[1].Results) != 1 || len(sourceSearch[1].Results[0].FavoriteIn) != 1 || sourceSearch[1].Results[0].FavoriteIn[0] != collection.ID {
		t.Fatalf("source collection membership = %+v", sourceSearch)
	}

	removeBody, _ := json.Marshal(favouriteRequest{FavoriteID: copySearch[1].Results[0].FavoriteID, Collection: collection.ID})
	removed := httptestNew(s, http.MethodDelete, "/api/favourites", removeBody)
	if removed.Code != http.StatusOK {
		t.Fatalf("remove collection article status %d: %s", removed.Code, removed.Body.String())
	}
	store, ok := sets.Store(collection.ID)
	if !ok {
		t.Fatal("custom collection disappeared")
	}
	articles, err := store.Manifest()
	if err != nil || len(articles) != 0 {
		t.Fatalf("custom collection after removal = %d, %v", len(articles), err)
	}
	defaultDelete := httptestNew(s, http.MethodDelete, "/api/favourites/collections", []byte(`{"id":"favourites"}`))
	if defaultDelete.Code != http.StatusBadRequest {
		t.Fatalf("default collection delete status %d: %s", defaultDelete.Code, defaultDelete.Body.String())
	}
	collectionPath := store.Path()
	deleted := httptestNew(s, http.MethodDelete, "/api/favourites/collections", []byte(`{"id":"`+collection.ID+`"}`))
	if deleted.Code != http.StatusOK {
		t.Fatalf("custom collection delete status %d: %s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(collectionPath); !os.IsNotExist(err) {
		t.Fatalf("deleted collection path status = %v", err)
	}
	if _, err := s.reg.get(collection.ID); err == nil {
		t.Fatal("deleted collection remains registered")
	}
}

func mustCollectionList(t *testing.T, sets *favorites.Collections) []favorites.CollectionInfo {
	t.Helper()
	collections, err := sets.List()
	if err != nil {
		t.Fatal(err)
	}
	return collections
}

func TestFavouriteSnapshotFindsEncodedImageInRealZIM(t *testing.T) {
	p := os.Getenv("WUDICT_TEST_ZIM")
	if p == "" {
		t.Skip("WUDICT_TEST_ZIM not set")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("WUDICT_TEST_ZIM unreadable: %v", err)
	}
	d, err := zim.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	// The article is the fixture, and only an archive that has it can test
	// what this test is about: an image whose name carries a percent escape.
	// A ZIM without it is not a failure - sibling real-file tests skip the
	// same way when the environment does not supply their fixture
	// (zim_test.go, redirect_test.go, mdx/mdd_test.go).
	rows, err := d.Exact("Хаммонд_(Луизиана)", 1)
	if err != nil {
		t.Fatalf("Hammond article lookup: %v", err)
	}
	if len(rows) != 1 {
		t.Skipf("this archive has no %q article to snapshot", "Хаммонд_(Луизиана)")
	}
	for _, ref := range (htmlref.Rewriter{}).Refs(rows[0].Body) {
		if ref.Tag != "img" || !strings.Contains(ref.URL, "East_Side_School") {
			continue
		}
		name, ok := favouriteResourceName(ref)
		if !ok {
			t.Fatalf("Hammond image reference was rejected: %q", ref.URL)
		}
		resource, mime, err := d.Resource(name)
		if err != nil {
			t.Fatalf("Resource(%q): %v", name, err)
		}
		data, readErr := io.ReadAll(resource)
		closeErr := resource.Close()
		if readErr != nil || closeErr != nil || len(data) == 0 {
			t.Fatalf("Hammond image bytes = %d, read %v, close %v", len(data), readErr, closeErr)
		}
		snapshots := favorites.New(filepath.Join(t.TempDir(), "favourites"))
		if err := snapshots.Ensure(); err != nil {
			t.Fatal(err)
		}
		saved, err := snapshots.Add(favorites.Provenance{DictionaryID: "zim", Headword: rows[0].Headword}, rows[0].Body,
			[]favorites.AssetInput{{URL: ref.URL, Name: name, MIME: mime, Data: data}})
		if err != nil {
			t.Fatal(err)
		}
		token, ok := snapshots.AssetToken(1, ref.URL)
		if !ok || token != saved.Assets[name].Token {
			t.Fatalf("saved image token = %q, %t; want %q", token, ok, saved.Assets[name].Token)
		}
		rendered := rewriteEntryHTMLFor(rows[0].Body, favorites.ID, 1, nil, snapshots.AssetToken)
		imageMapped := false
		for _, got := range (htmlref.Rewriter{}).Refs(rendered) {
			if got.Tag == "img" && got.URL == "/res/favourites/"+token {
				imageMapped = true
				break
			}
		}
		if !imageMapped {
			t.Fatal("saved Hammond image was not rewritten to the favourites resource")
		}
		reg, err := NewRegistry(nil, false, WithBuiltin(Builtin{ID: favorites.ID, Path: snapshots.Path()}))
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if entry, err := reg.get(favorites.ID); err == nil {
				entry.closeNow()
			}
		}()
		served := httptest.NewRecorder()
		New(reg).ServeHTTP(served, newRequest(http.MethodGet, "/res/favourites/"+token, nil))
		if served.Code != http.StatusOK || !bytes.Equal(served.Body.Bytes(), data) {
			t.Fatalf("saved Hammond image response = %d, %d bytes; want %d bytes", served.Code, served.Body.Len(), len(data))
		}
		return
	}
	t.Fatal("Hammond building image was not referenced")
}

func httptestNew(s *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest(method, path, bytes.NewReader(body)))
	return rec
}

func articleBody(t *testing.T, s *Server, id string) string {
	t.Helper()
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.open()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := d.Exact("corazón", 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("source lookup = %+v, %v", rows, err)
	}
	return rows[0].Body
}
