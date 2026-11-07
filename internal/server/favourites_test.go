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

func TestFavouriteSnapshotFindsEncodedImageInRealZIM(t *testing.T) {
	p := os.Getenv("WUDICT_TEST_ZIM")
	if p == "" {
		t.Skip("WUDICT_TEST_ZIM not set")
	}
	d, err := zim.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	rows, err := d.Exact("Хаммонд_(Луизиана)", 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Hammond article = %d rows, %v", len(rows), err)
	}
	for _, ref := range (htmlref.Rewriter{}).Refs(rows[0].Body) {
		if ref.Tag != "img" || !strings.Contains(ref.URL, "East_Side_School") {
			continue
		}
		name, ok := favouriteResourceName(ref)
		if !ok {
			t.Fatalf("Hammond image reference was rejected: %q", ref.URL)
		}
		resource, _, err := d.Resource(name)
		if err != nil {
			t.Fatalf("Resource(%q): %v", name, err)
		}
		data, readErr := io.ReadAll(resource)
		closeErr := resource.Close()
		if readErr != nil || closeErr != nil || len(data) == 0 {
			t.Fatalf("Hammond image bytes = %d, read %v, close %v", len(data), readErr, closeErr)
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
