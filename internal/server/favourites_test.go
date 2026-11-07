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
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/favorites"
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
		t.Fatalf("saved assets = %+v", article.Assets)
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
	if rows[0].ResourceMap["beat.mp3"] == "" {
		t.Fatalf("snapshot resource map = %+v", rows[0].ResourceMap)
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
