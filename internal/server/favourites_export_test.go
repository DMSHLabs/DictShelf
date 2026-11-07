// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/favorites"
	_ "github.com/DMSHLabs/DictShelf/internal/format/mdx"
)

func TestFavouriteExportRoute(t *testing.T) {
	s := newTestServer(t)
	s.favourites = favorites.New(filepath.Join(t.TempDir(), "favourites"))
	if err := s.favourites.EnsureNamed("Saved words"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.favourites.Add(favorites.Provenance{DictionaryID: "source", Headword: "word"}, "<p>body</p>", nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	request, _ := json.Marshal(favouriteExportRequest{Action: "start", ID: favorites.ID, Path: dir, Name: "Saved words renamed"})
	r := httptestNew(s, http.MethodPost, "/api/favourites/export", request)
	if r.Code != http.StatusOK {
		t.Fatalf("start: %d %s", r.Code, r.Body.String())
	}
	s.jobs.wg.Wait()
	status := httptestNew(s, http.MethodPost, "/api/favourites/export", []byte(`{"action":"status"}`))
	if status.Code != http.StatusOK {
		t.Fatalf("status: %d %s", status.Code, status.Body.String())
	}
	var result jobStatus
	if err := json.Unmarshal(status.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Err != "" || result.Running || result.Done != result.Total || result.Done == 0 {
		t.Fatalf("export status: %+v", result)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.mdx"))
	if err != nil || len(files) != 1 {
		t.Fatalf("export files: %v %v", files, err)
	}
	// The name the request carried is the dictionary's name inside the export.
	exported, err := dict.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer exported.Close()
	if want := "Saved words renamed"; exported.Meta().Name != want {
		t.Fatalf("export title %q, want %q", exported.Meta().Name, want)
	}
}
