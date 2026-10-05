// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/store"
)

type clearDatabaseRequest struct {
	Index    bool `json:"index"`
	Contains bool `json:"contains"`
	FullText bool `json:"fullText"`
	Obsolete bool `json:"obsolete"`
	plan     *store.Plan
	progress store.Progress
}

// cleanupLibrary retains only prepared folders belonging to discovered sources.
// Discovery must succeed before any deletion.
func (r *Registry) cleanupLibrary() error {
	// Stop preparation and first-open auto-ingestion while inspecting debris.
	// Otherwise a claimed but incomplete folder could be an active ingest.
	entries := r.all()
	for _, e := range entries {
		e.ingestMu.Lock()
		e.rebuilding.Store(true)
		e.openMu.Lock()
	}
	defer func() {
		for _, e := range entries {
			e.rebuilding.Store(false)
			e.openMu.Unlock()
			e.ingestMu.Unlock()
		}
	}()
	r.mu.RLock()
	dirs := append([]string(nil), r.dictDirs...)
	r.mu.RUnlock()
	paths, _, err := dict.DiscoverAll(dirs)
	if err != nil {
		return err
	}
	active := map[string]bool{}
	preparedDirs := map[string]bool{}
	blockedDirs := map[string]bool{}
	for _, e := range entries {
		if e.indexBlocked() {
			if dir, ok := store.LookupDir(e.Path); ok {
				blockedDirs[dict.CanonPath(dir)] = true
			}
		}
	}
	addActive := func(p string) {
		active[dict.CanonPath(p)] = true
		if dir, ok := store.LookupDir(p); ok && !blockedDirs[dict.CanonPath(dir)] {
			preparedDirs[dict.CanonPath(dir)] = true
		}
	}
	for _, p := range paths {
		addActive(p)
	}
	folders, err := store.Folders()
	if err != nil {
		return err
	}
	for _, f := range folders {
		if r.IsBuiltin(f.Source) || active[dict.CanonPath(store.TextDBPath(f.Dir))] {
			continue
		}
		if preparedDirs[dict.CanonPath(f.Dir)] {
			// Media packing is no longer offered in this fork. These copies
			// are redundant; resources continue to come from the source files.
			media := store.MediaDBPath(f.Dir)
			if fsx.FileExists(media) {
				if e := r.entryAt(store.TextDBPath(f.Dir)); e != nil {
					e.closeNow()
				}
				if err := os.Remove(media); err != nil {
					return err
				}
				if err := store.WriteInfo(f.Dir); err != nil {
					return err
				}
			}
			continue
		}
		if err := r.removeCachedFolder(f.Dir); err != nil {
			return err
		}
	}
	leftovers, err := store.FindLeftovers()
	if err != nil {
		return err
	}
	for _, item := range leftovers {
		if active[dict.CanonPath(item.Path)] || active[dict.CanonPath(store.TextDBPath(item.Path))] {
			continue
		}
		if item.IsDir {
			err = r.removeCachedFolder(item.Path)
		} else {
			err = os.Remove(item.Path)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) removeCachedFolder(dir string) error {
	e := r.entryAt(store.TextDBPath(dir))
	if e != nil {
		e.closeNow()
	}
	_, err := store.RemovePrepared(dir)
	return err
}

func (s *Server) handleClearDatabase(w http.ResponseWriter, req *http.Request) {
	if !s.removalOffered(req) {
		httpErr(w, 403, "deleting from another machine is off")
		return
	}
	var body clearDatabaseRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&body); err != nil {
		httpErr(w, 400, "bad request body: %v", err)
		return
	}
	if !body.Index && !body.Contains && !body.FullText && !body.Obsolete {
		httpErr(w, 400, "nothing selected")
		return
	}
	defer HoldActiveProcs()()
	failures := []string{}
	for _, e := range s.reg.all() {
		// A database supplied as a dictionary is a source, never a disposable index.
		if store.IsTextDB(e.Path) || e.builtin {
			continue
		}
		if err := e.clearDatabase(body); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", filepath.Base(e.Path), err))
		}
	}
	if body.Obsolete {
		if err := s.reg.cleanupLibrary(); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if err := s.reg.Rescan(); err != nil {
		failures = append(failures, err.Error())
	}
	writeJSON(w, map[string]any{"failed": failures})
}

func (e *entry) clearDatabase(want clearDatabaseRequest) error {
	acquire(frontLimit)
	defer release(frontLimit)
	e.ingestMu.Lock()
	defer e.ingestMu.Unlock()
	dir, ok := store.LookupDir(e.Path)
	if !ok {
		if !want.Index {
			return nil
		}
		var err error
		dir, err = store.ClaimDir(e.Path)
		if err != nil {
			return err
		}
	}
	text := store.TextDBPath(dir)
	plan := store.KeptPlan(text)
	if want.plan != nil {
		plan = *want.plan
	}
	if want.Contains {
		plan.Contains = false
	}
	if want.FullText {
		plan.FullText = false
	}
	e.rebuilding.Store(true)
	defer e.rebuilding.Store(false)
	e.openMu.Lock()
	defer e.openMu.Unlock()
	e.closeNow()
	if want.Obsolete {
		if err := os.Remove(store.MediaDBPath(dir)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if want.Index || (want.Obsolete && len(store.Inspect(text).TextStale(e.Path)) > 0) {
		// Rebuild atomically: a failed rebuild preserves the existing searchable data.
		if _, err := e.reconcileLocked(e.probeName(), store.Target{
			FullText: &plan.FullText, Contains: &plan.Contains, Rebuild: store.Always,
		}, want.progress); err != nil {
			return err
		}
		if err := e.setIndexRemoved(false); err != nil {
			return err
		}
	} else if want.Contains || want.FullText {
		if err := store.ClearSearchIndexes(text, want.Contains, want.FullText); err != nil {
			return err
		}
	}
	return store.WriteInfo(dir)
}
