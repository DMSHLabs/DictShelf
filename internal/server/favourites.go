// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/favorites"
	"github.com/DMSHLabs/DictShelf/internal/htmlref"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

type favouriteRequest struct {
	Dictionary string `json:"dict"`
	Headword   string `json:"headword"`
	FavoriteID string `json:"favoriteId"`
}

func (s *Server) handleFavourites(w http.ResponseWriter, r *http.Request) {
	if s.favourites == nil {
		httpErr(w, http.StatusServiceUnavailable, "favourites are unavailable")
		return
	}
	var req favouriteRequest
	if !decodeJSON(w, r, &req, 4096) {
		return
	}
	var err error
	switch r.Method {
	case http.MethodPost:
		err = s.addFavourite(req)
	case http.MethodDelete:
		err = s.removeFavourite(req)
	default:
		httpErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, favorites.ErrDuplicate) {
			status = http.StatusConflict
		}
		if errors.Is(err, dict.ErrNotFound) || errors.Is(err, io.EOF) {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unknown dictionary") {
			status = http.StatusBadRequest
		}
		httpErr(w, status, "%v", err)
		return
	}
	writeJSON(w, map[string]bool{"favorite": r.Method == http.MethodPost})
}

func (s *Server) addFavourite(req favouriteRequest) error {
	id, word := strings.TrimSpace(req.Dictionary), strings.TrimSpace(req.Headword)
	if id == "" || word == "" {
		return fmt.Errorf("dictionary and headword are required")
	}
	if id == favorites.ID {
		return fmt.Errorf("an article in favourites cannot be saved again")
	}
	e, err := s.reg.get(id)
	if err != nil {
		return err
	}
	d, err := e.open()
	if err != nil {
		return err
	}
	rows, err := d.Exact(word, 100)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return dict.ErrNotFound
	}
	row := rows[0]
	assets := make([]favorites.AssetInput, 0)
	seen := map[string]bool{}
	for _, ref := range (htmlref.Rewriter{}).Refs(row.Body) {
		name, ok := favouriteResourceName(ref)
		if !ok || seen[ref.URL] {
			continue
		}
		seen[ref.URL] = true
		rc, mime, openErr := d.Resource(name)
		if openErr != nil {
			return fmt.Errorf("copy article resource %q: %w", ref.URL, openErr)
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, favorites.MaxAsset+1))
		closeErr := rc.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(data) > favorites.MaxAsset {
			return fmt.Errorf("resource %q exceeds the snapshot limit", ref.URL)
		}
		assets = append(assets, favorites.AssetInput{URL: ref.URL, Name: name, MIME: mime, Data: data})
	}
	name := d.Meta().Name
	if display := s.reg.prefs.DisplayTitle(id); display != "" {
		name = display
	}
	provenance := favorites.Provenance{DictionaryID: id, DictionaryName: name, SourcePath: e.Path,
		Headword: row.Headword, EntryID: fmt.Sprint(row.EntryID)}
	if _, err := s.favourites.Add(provenance, row.Body, assets); err != nil {
		return err
	}
	return s.rebuildFavourites()
}

func (s *Server) removeFavourite(req favouriteRequest) error {
	id := strings.TrimSpace(req.FavoriteID)
	if id == "" {
		return fmt.Errorf("favoriteId is required")
	}
	if err := s.favourites.Remove(id); err != nil {
		return err
	}
	return s.rebuildFavourites()
}

func favouriteResourceName(ref htmlref.Ref) (string, bool) {
	if !isResourceRef(ref) {
		return "", false
	}
	raw := ref.URL
	if strings.HasPrefix(raw, "//") {
		return "", false
	}
	if schemeRef.MatchString(raw) && !soundOrFile.MatchString(raw) {
		return "", false
	}
	raw = soundOrFile.ReplaceAllString(raw, "")
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return "", false
	}
	if strings.HasPrefix(u.Path, "/assets/") || strings.HasPrefix(u.Path, "/res/") || strings.HasPrefix(u.Path, userFileURL) {
		return "", false
	}
	name, err := url.PathUnescape(u.Path)
	if err != nil {
		return "", false
	}
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." || strings.HasPrefix(name, "../") {
		return "", false
	}
	return name, true
}

func (s *Server) rebuildFavourites() error {
	e, err := s.reg.get(favorites.ID)
	if err != nil {
		return err
	}
	e.ingestMu.Lock()
	defer e.ingestMu.Unlock()
	e.rebuilding.Store(true)
	defer e.rebuilding.Store(false)
	e.openMu.Lock()
	e.closeNow()
	e.openMu.Unlock()
	yes := true
	if _, err := e.reconcileLocked("Favourites", store.Target{FullText: &yes, Media: store.MediaOn}, nil); err != nil {
		return err
	}
	if err := s.reg.rescan(false); err != nil {
		return err
	}
	return nil
}

func (s *Server) decorateFavourite(dictionaryID string, result *dict.Result) {
	if s.favourites == nil || result == nil {
		return
	}
	if dictionaryID == favorites.ID {
		if result.FavoriteID == "" {
			if e, err := s.reg.get(dictionaryID); err == nil {
				if d, err := e.open(); err == nil {
					if resolver, ok := d.(interface{ FavoriteID(int64) string }); ok {
						result.FavoriteID = resolver.FavoriteID(result.EntryID)
					}
				}
			}
		}
		return
	}
	result.Favorite = s.favourites.Has(dictionaryID, result.Headword)
}
