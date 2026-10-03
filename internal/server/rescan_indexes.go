// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
)

type rescanIndexActions struct {
	Index    string `json:"index"`
	Contains string `json:"contains"`
	FullText string `json:"fullText"`
}

type rescanIndexesRequest struct {
	New      *dslIndexOptions   `json:"new,omitempty"`
	Existing rescanIndexActions `json:"existing"`
}

type rescanIndexProgress struct {
	Stage   string `json:"stage"`
	At      int    `json:"at"`
	Total   int    `json:"total"`
	Name    string `json:"name"`
	Done    int    `json:"done"`
	Entries int    `json:"entries"`
}

func validRescanAction(action string, base bool) bool {
	return action == "" || action == "keep" || (base && action == "recreate") || (!base && (action == "update" || action == "delete"))
}

func (s *Server) handleRescanIndexes(w http.ResponseWriter, req *http.Request) {
	if !s.removalOffered(req) {
		httpErr(w, 403, "deleting from another machine is off")
		return
	}
	var body rescanIndexesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&body); err != nil {
		httpErr(w, 400, "bad request body: %v", err)
		return
	}
	if body.New == nil || !body.New.Index || !validRescanAction(body.Existing.Index, true) || !validRescanAction(body.Existing.Contains, false) || !validRescanAction(body.Existing.FullText, false) {
		httpErr(w, 400, "invalid index actions")
		return
	}
	if req.URL.Query().Get("stream") == "1" {
		flusher, ok := w.(http.Flusher)
		if !ok {
			httpErr(w, 500, "streaming unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-cache")
		encoder := json.NewEncoder(w)
		emit := func(message any) {
			// Losing the connection must not interrupt index maintenance.
			_ = encoder.Encode(message)
			flusher.Flush()
		}
		failures := s.reg.updateDictionaryIndexesProgress(body, func(progress rescanIndexProgress) {
			emit(struct {
				Type string `json:"t"`
				rescanIndexProgress
			}{"progress", progress})
		})
		emit(map[string]any{"t": "done", "failed": failures})
		return
	}
	failures := s.reg.updateDictionaryIndexes(body)
	writeJSON(w, map[string]any{"failed": failures})
}

// The automatic DSL worker must finish before taking the snapshot. Rescans
// during this operation discover only; no background default can undo choices.
func (r *Registry) updateDictionaryIndexes(req rescanIndexesRequest) []string {
	return r.updateDictionaryIndexesProgress(req, nil)
}

func (r *Registry) updateDictionaryIndexesProgress(req rescanIndexesRequest, progress func(rescanIndexProgress)) []string {
	report := func(p rescanIndexProgress) {
		if progress != nil {
			progress(p)
		}
	}
	report(rescanIndexProgress{Stage: "scanning"})
	r.dslAutoMu.Lock()
	defer r.dslAutoMu.Unlock()
	defer HoldActiveProcs()()
	failures := []string{}
	previous := map[string]bool{}
	for _, e := range r.all() {
		previous[dict.CanonPath(e.Path)] = true
	}
	if err := r.rescan(false); err != nil {
		return []string{err.Error()}
	}
	defaults := r.prefs.newDSLDefaults()
	if req.New != nil {
		if defaults.Original.Index {
			defaults.Original.Contains, defaults.Original.FullText = req.New.Contains, req.New.FullText
		}
		if defaults.GD.Index {
			defaults.GD.Contains, defaults.GD.FullText = req.New.Contains, req.New.FullText
		}
	}
	type target struct {
		e       *entry
		fresh   bool
		options dslIndexOptions
	}
	targets := []target{}
	p := r.prefs
	for _, e := range r.all() {
		if e.builtin || store.IsTextDB(e.Path) {
			continue
		}
		_, prepared := store.LookupDir(e.Path)
		p.mu.RLock()
		_, pending := p.dslPending[e.indexRemovalKey()]
		p.mu.RUnlock()
		fresh := pending || (!prepared && (!previous[dict.CanonPath(e.Path)] || !e.indexBlocked()))
		options := dslIndexOptions{Index: true}
		options.Contains = defaults.Original.Index && defaults.Original.Contains || defaults.GD.Index && defaults.GD.Contains
		options.FullText = defaults.Original.Index && defaults.Original.FullText || defaults.GD.Index && defaults.GD.FullText
		if req.New != nil {
			options = *req.New
		}
		if e.dslSource != "" {
			options = defaults.Original
			if e.dslVariant == "gd" {
				options = defaults.GD
			}
		}
		targets = append(targets, target{e, fresh, options})
	}
	// Save desired pending plans before touching files; a retry/restart uses the
	// requested features, never the older queued plan.
	p.editMu.Lock()
	p.mu.Lock()
	oldDefaults, oldKnown, oldPending, oldRemoved, oldExists := p.dslDefaults, p.dslKnown, p.dslPending, p.dslRemoved, p.exists
	p.dslKnown = maps.Clone(p.dslKnown)
	p.dslPending = map[string]dslIndexOptions{}
	p.dslRemoved = maps.Clone(p.dslRemoved)
	if p.dslKnown == nil {
		p.dslKnown = map[string]bool{}
	}
	if p.dslPending == nil {
		p.dslPending = map[string]dslIndexOptions{}
	}
	if p.dslRemoved == nil {
		p.dslRemoved = map[string]bool{}
	}
	p.dslDefaults = &defaults
	p.exists = true
	for _, t := range targets {
		key := t.e.indexRemovalKey()
		source := cleanAbs(t.e.Path)
		if t.e.dslSource != "" {
			source = t.e.dslSource
		}
		p.dslKnown[source] = true
		delete(p.dslPending, key)
		if t.fresh {
			p.dslRemoved[key] = true
			if t.options.Index {
				p.dslPending[key] = t.options
			}
		}
	}
	err := p.saveLocked()
	if err != nil {
		p.dslDefaults, p.dslKnown, p.dslPending, p.dslRemoved, p.exists = oldDefaults, oldKnown, oldPending, oldRemoved, oldExists
	}
	p.mu.Unlock()
	p.editMu.Unlock()
	if err != nil {
		return []string{err.Error()}
	}
	for i, t := range targets {
		e := t.e
		state := rescanIndexProgress{Stage: "dictionary", At: i + 1, Total: len(targets), Name: e.probeName()}
		report(state)
		last := time.Time{}
		articleProgress := func(done, total int) {
			state.Done, state.Entries = done, total
			if time.Since(last) >= 200*time.Millisecond || done == total {
				report(state)
				last = time.Now()
			}
		}
		if t.fresh && !t.options.Index {
			if err := e.setIndexRemoved(true); err != nil {
				failures = append(failures, err.Error())
				continue
			}
			if dir, ok := store.LookupDir(e.Path); ok {
				e.ingestMu.Lock()
				e.rebuilding.Store(true)
				e.openMu.Lock()
				e.closeNow()
				_, err = store.RemovePrepared(dir)
				e.openMu.Unlock()
				e.rebuilding.Store(false)
				e.ingestMu.Unlock()
				if err != nil {
					failures = append(failures, err.Error())
				}
			}
			continue
		}
		want := clearDatabaseRequest{progress: articleProgress}
		if t.fresh {
			plan := store.Plan{Contains: t.options.Contains, FullText: t.options.FullText}
			want.Index, want.plan = true, &plan
		} else {
			dir, ok := store.LookupDir(e.Path)
			if !ok && req.Existing.Index != "recreate" {
				continue
			}
			have := store.Plan{}
			if ok {
				have = store.KeptPlan(store.TextDBPath(dir))
			}
			want.Index = req.Existing.Index == "recreate" || (req.Existing.Contains == "update" && have.Contains) || (req.Existing.FullText == "update" && have.FullText)
			want.Contains = req.Existing.Contains == "delete"
			want.FullText = req.Existing.FullText == "delete"
			// Recreating a selected parser must not revive a disabled sibling.
			if e.indexBlocked() && e.dslSource != "" && r.prefs.parserSelection() != "both" && r.prefs.parserSelection() != e.dslVariant {
				continue
			}
		}
		if err := e.clearDatabase(want); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", e.Path, err))
			continue
		}
		p.editMu.Lock()
		p.mu.Lock()
		delete(p.dslPending, e.indexRemovalKey())
		err = p.saveLocked()
		p.mu.Unlock()
		p.editMu.Unlock()
		if err != nil {
			failures = append(failures, err.Error())
		}
	}
	report(rescanIndexProgress{Stage: "cleanup", At: len(targets), Total: len(targets)})
	if err := r.cleanupLibrary(); err != nil {
		failures = append(failures, err.Error())
	}
	if err := r.rescan(false); err != nil {
		failures = append(failures, err.Error())
	}
	return failures
}
