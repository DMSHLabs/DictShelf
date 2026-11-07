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
	Collection string `json:"collectionId"`
}

func (s *Server) handleFavouriteCollections(w http.ResponseWriter, r *http.Request) {
	if s.favSets == nil {
		httpErr(w, http.StatusServiceUnavailable, "favourites are unavailable")
		return
	}
	if r.Method == http.MethodGet {
		collections, err := s.listFavouriteCollections()
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "could not list favourites: %v", err)
			return
		}
		writeJSON(w, map[string]any{"collections": collections})
		return
	}
	if r.Method == http.MethodDelete {
		var request struct {
			ID string `json:"id"`
		}
		if !decodeJSON(w, r, &request, 4096) {
			return
		}
		if err := s.deleteFavouriteCollection(request.ID); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, favorites.ErrInvalidCollection) {
				status = http.StatusBadRequest
			}
			httpErr(w, status, "%v", err)
			return
		}
		writeJSON(w, map[string]bool{"deleted": true})
		return
	}
	if r.Method != http.MethodPost {
		httpErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request, 4096) {
		return
	}
	collection, err := s.createFavouriteCollection(request.Name)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, favorites.ErrInvalidCollection) {
			status = http.StatusBadRequest
		}
		httpErr(w, status, "%v", err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, collection)
}

func (s *Server) deleteFavouriteCollection(id string) error {
	if s.favSets == nil || s.reg == nil {
		return errors.New("favourites are unavailable")
	}
	if id == favorites.ID {
		return fmt.Errorf("%w: the default collection cannot be deleted", favorites.ErrInvalidCollection)
	}
	e, err := s.reg.get(id)
	if err != nil {
		return fmt.Errorf("%w: collection not found", favorites.ErrInvalidCollection)
	}
	e.ingestMu.Lock()
	e.rebuilding.Store(true)
	e.openMu.Lock()
	e.closeNow()
	e.openMu.Unlock()
	deleteErr := s.favSets.Delete(id)
	e.rebuilding.Store(false)
	e.ingestMu.Unlock()
	if deleteErr != nil {
		_ = s.reg.rescan(false)
		return deleteErr
	}
	s.reg.RemoveBuiltin(id)
	if err := s.reg.rescan(false); err != nil {
		return err
	}
	s.dictRows.flush()
	return nil
}

func (s *Server) listFavouriteCollections() ([]favorites.CollectionInfo, error) {
	if s.favSets == nil {
		return nil, errors.New("favourites are unavailable")
	}
	collections, err := s.favSets.List()
	if err != nil {
		return nil, err
	}
	if s.reg != nil && s.reg.prefs != nil {
		for i := range collections {
			if title := s.reg.prefs.DisplayTitle(collections[i].ID); title != "" {
				collections[i].Name = title
			}
		}
	}
	return collections, nil
}

func (s *Server) createFavouriteCollection(name string) (favorites.CollectionInfo, error) {
	if s.favSets == nil {
		return favorites.CollectionInfo{}, errors.New("favourites are unavailable")
	}
	name = strings.TrimSpace(name)
	current, err := s.listFavouriteCollections()
	if err != nil {
		return favorites.CollectionInfo{}, err
	}
	for _, collection := range current {
		if strings.EqualFold(collection.Name, name) {
			return favorites.CollectionInfo{}, fmt.Errorf("%w: a collection with that name already exists", favorites.ErrInvalidCollection)
		}
	}
	collection, err := s.favSets.Create(name)
	if err != nil {
		return favorites.CollectionInfo{}, err
	}
	collectionStore, _ := s.favSets.Store(collection.ID)
	fullText := true
	if _, err := store.Reconcile(collectionStore.Path(), store.Target{FullText: &fullText, Media: store.MediaOn}, store.Hooks{}); err != nil {
		return favorites.CollectionInfo{}, fmt.Errorf("could not prepare favourites: %w", err)
	}
	s.reg.AddBuiltin(Builtin{ID: collection.ID, Path: collectionStore.Path()})
	if err := s.reg.rescan(false); err != nil {
		return favorites.CollectionInfo{}, fmt.Errorf("could not register favourites: %w", err)
	}
	s.dictRows.flush()
	return collection, nil
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
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unknown dictionary") || strings.Contains(err.Error(), "unknown favourites collection") {
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
	if s.isFavouriteDictionaryID(id) {
		return fmt.Errorf("an article in favourites cannot be saved again")
	}
	collectionID := strings.TrimSpace(req.Collection)
	if collectionID == "" {
		collectionID = favorites.ID
	}
	_, ok := s.favouriteStore(collectionID)
	if !ok {
		return fmt.Errorf("unknown favourites collection")
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
		probe, mediaType, err := d.Resource(name)
		if err != nil {
			// Keep the snapshot even when a source article contains a stale
			// reference. There are no bytes to preserve for a missing resource.
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(probe, favorites.MaxAsset+1))
		closeErr := probe.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(data) > favorites.MaxAsset {
			return fmt.Errorf("resource %q exceeds the snapshot limit", ref.URL)
		}
		assets = append(assets, favorites.AssetInput{URL: ref.URL, Name: name, MIME: mediaType, Data: data})
	}
	name := d.Meta().Name
	if display := s.reg.prefs.DisplayTitle(id); display != "" {
		name = display
	}
	counts := s.aboutSourceFor(e)
	provenance := favorites.Provenance{DictionaryID: id, DictionaryName: name, SourcePath: e.Path,
		Headword: row.Headword, Headwords: counts.headwords, Articles: counts.entries, EntryID: fmt.Sprint(row.EntryID)}
	return s.rebuildFavourites(collectionID, func(target *favorites.Store) error {
		_, err := target.Add(provenance, row.Body, assets)
		return err
	})
}

func (s *Server) removeFavourite(req favouriteRequest) error {
	id := strings.TrimSpace(req.FavoriteID)
	if id == "" {
		return fmt.Errorf("favoriteId is required")
	}
	collectionID := strings.TrimSpace(req.Collection)
	if collectionID == "" {
		collectionID = favorites.ID
	}
	_, ok := s.favouriteStore(collectionID)
	if !ok {
		return fmt.Errorf("unknown favourites collection")
	}
	return s.rebuildFavourites(collectionID, func(target *favorites.Store) error {
		return target.Remove(id)
	})
}

func (s *Server) favouriteStore(id string) (*favorites.Store, bool) {
	if s.favSets != nil {
		return s.favSets.Store(id)
	}
	if id == favorites.ID && s.favourites != nil {
		return s.favourites, true
	}
	return nil, false
}

func (s *Server) favouriteIDs() []string {
	if s.favSets != nil {
		return s.favSets.IDs()
	}
	if s.favourites != nil {
		return []string{favorites.ID}
	}
	return nil
}

func (s *Server) isFavouriteDictionaryID(id string) bool {
	_, ok := s.favouriteStore(id)
	return ok
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
	// url.Parse has already unescaped Path once. Keep any remaining escape
	// sequences literal: a ZIM can store a filename containing "%2C", whose
	// article URL spells that percent sign as "%25". Unescaping Path again
	// turns the stored "%2C" into a comma and makes Resource miss the asset.
	name := u.Path
	// MDict resource names are read without the URL pseudo-scheme. The
	// rewriter applies the same sound:// and file:// normalization when it
	// serves the source article, so snapshots must use that normalized name
	// when asking the source backend for the bytes.
	name = soundOrFile.ReplaceAllString(name, "")
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." || strings.HasPrefix(name, "../") {
		return "", false
	}
	return name, true
}

func (s *Server) rebuildFavourites(collectionID string, update func(*favorites.Store) error) error {
	collectionStore, ok := s.favouriteStore(collectionID)
	if !ok {
		return fmt.Errorf("unknown favourites collection")
	}
	e, err := s.reg.get(collectionID)
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
	if err := update(collectionStore); err != nil {
		return err
	}
	yes := true
	if _, err := e.reconcileLocked("Favourites", store.Target{FullText: &yes, Media: store.MediaOn}, nil); err != nil {
		return err
	}
	collectionStore.Collect()
	if err := s.reg.rescan(false); err != nil {
		return err
	}
	return nil
}

func (s *Server) decorateFavourite(dictionaryID string, result *dict.Result) {
	if s.favourites == nil || result == nil {
		return
	}
	if s.isFavouriteDictionaryID(dictionaryID) {
		if result.FavoriteID == "" || result.FavoriteSource == nil {
			if e, err := s.reg.get(dictionaryID); err == nil {
				if d, err := e.open(); err == nil {
					if result.FavoriteID == "" {
						if resolver, ok := d.(interface{ FavoriteID(int64) string }); ok {
							result.FavoriteID = resolver.FavoriteID(result.EntryID)
						}
					}
					if result.FavoriteSource == nil {
						if resolver, ok := d.(interface {
							FavoriteSource(int64) *dict.FavoriteSource
						}); ok {
							result.FavoriteSource = resolver.FavoriteSource(result.EntryID)
						}
					}
				}
			}
		}
		s.decorateFavouriteSource(result.FavoriteSource)
		return
	}
	for _, collectionID := range s.favouriteIDs() {
		collectionStore, ok := s.favouriteStore(collectionID)
		if ok && collectionStore.Has(dictionaryID, result.Headword) {
			result.FavoriteIn = append(result.FavoriteIn, collectionID)
		}
	}
	result.Favorite = len(result.FavoriteIn) > 0
}

func (s *Server) decorateFavouriteSource(source *dict.FavoriteSource) {
	if source == nil {
		return
	}
	if source.SourcePath == "" {
		return
	}
	for _, e := range s.reg.all() {
		if dict.CanonPath(e.Path) == dict.CanonPath(source.SourcePath) {
			source.DictionaryID = e.ID
			source.SourcePath = e.Path
			source.Available = true
			if source.Headwords == 0 || source.Articles == 0 {
				counts := s.aboutSourceFor(e)
				if source.Headwords == 0 {
					source.Headwords = counts.headwords
				}
				if source.Articles == 0 {
					source.Articles = counts.entries
				}
			}
			return
		}
	}
}
