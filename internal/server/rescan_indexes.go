// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"

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
	failures := s.reg.updateDictionaryIndexes(body)
	writeJSON(w, map[string]any{"failed": failures})
}

// The automatic DSL worker must finish before taking the snapshot. Rescans
// during this operation discover only; no background default can undo choices.
func (r *Registry) updateDictionaryIndexes(req rescanIndexesRequest) []string {
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
	for _, t := range targets {
		e := t.e
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
		want := clearDatabaseRequest{}
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
	if err := r.cleanupLibrary(); err != nil {
		failures = append(failures, err.Error())
	}
	if err := r.rescan(false); err != nil {
		failures = append(failures, err.Error())
	}
	return failures
}
