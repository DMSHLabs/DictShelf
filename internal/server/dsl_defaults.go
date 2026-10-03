// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"maps"
	"net/http"

	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
)

type dslIndexOptions struct {
	Index    bool `json:"index"`
	Contains bool `json:"contains"`
	FullText bool `json:"fullText"`
}

type dslDefaults struct {
	Original dslIndexOptions `json:"original"`
	GD       dslIndexOptions `json:"gd"`
}

func (p *Prefs) newDSLDefaults() dslDefaults {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.dslDefaults != nil {
		return *p.dslDefaults
	}
	return dslDefaults{Original: dslIndexOptions{Index: true}}
}

func (s *Server) handleDSLDefaults(w http.ResponseWriter, r *http.Request) {
	var next dslDefaults
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&next); err != nil {
		httpErr(w, 400, "invalid DSL defaults")
		return
	}
	if !next.Original.Index && !next.GD.Index {
		httpErr(w, 400, "select at least one DSL index: Original or GD compatible")
		return
	}
	for _, v := range []dslIndexOptions{next.Original, next.GD} {
		if !v.Index && (v.Contains || v.FullText) {
			httpErr(w, 400, "DSL contains and full-text require index")
			return
		}
	}
	entries := s.reg.all()
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	old, known, existed := p.dslDefaults, p.dslKnown, p.exists
	oldParser, oldInitial := p.dslParser, p.dslInitialSetup
	if old == nil && len(entries) == 0 && len(p.dsl) == 0 && len(known) == 0 && p.dslParser == "" {
		p.dslInitialSetup = true
	}
	if p.dslInitialSetup && len(entries) == 0 {
		p.dslParser = "both"
		if !next.GD.Index {
			p.dslParser = "original"
		} else if !next.Original.Index {
			p.dslParser = "gd"
		}
	}
	p.dslKnown = maps.Clone(known)
	if p.dslKnown == nil {
		p.dslKnown = map[string]bool{}
	}
	// Activating the policy never changes dictionaries already in the registry.
	for _, e := range entries {
		if e.dslSource != "" {
			p.dslKnown[e.dslSource] = true
		}
	}
	p.dslDefaults, p.exists = &next, true
	if err := p.saveLocked(); err != nil {
		p.dslDefaults, p.dslKnown, p.exists = old, known, existed
		p.dslParser, p.dslInitialSetup = oldParser, oldInitial
		httpErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, next)
}

func (r *Registry) queueNewDSL() error {
	entries := r.all()
	families := make(map[string][]*entry)
	for _, e := range entries {
		if e.dslSource != "" {
			families[e.dslSource] = append(families[e.dslSource], e)
		}
	}
	p := r.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	// Existing installations retain their choices until defaults are saved.
	if p.dslDefaults == nil {
		return nil
	}
	known, pending, removed, modes, existed := p.dslKnown, p.dslPending, p.dslRemoved, p.dsl, p.exists
	initial := p.dslInitialSetup
	p.dslKnown = maps.Clone(known)
	if p.dslKnown == nil {
		p.dslKnown = map[string]bool{}
	}
	p.dslPending = maps.Clone(pending)
	if p.dslPending == nil {
		p.dslPending = map[string]dslIndexOptions{}
	}
	p.dslRemoved = maps.Clone(removed)
	if p.dslRemoved == nil {
		p.dslRemoved = map[string]bool{}
	}
	p.dsl = maps.Clone(modes)
	if p.dsl == nil {
		p.dsl = map[string]string{}
	}
	changed := initial && len(entries) > 0
	if changed {
		p.dslInitialSetup = false
	}
	for source, family := range families {
		if p.dslKnown[source] {
			continue
		}
		p.dslKnown[source] = true
		changed = true
		// A previously prepared family is not a newly added dictionary.
		existing := false
		for _, other := range family {
			if _, ok := validPrepared(other.Path); ok {
				existing = true
			}
		}
		if existing {
			continue
		}
		mode := "both"
		if !p.dslDefaults.GD.Index {
			mode = "original"
		} else if !p.dslDefaults.Original.Index {
			mode = "gd"
		}
		p.dsl[source] = mode
		for _, other := range family {
			v := p.dslDefaults.Original
			if other.dslVariant == "gd" {
				v = p.dslDefaults.GD
			}
			key := other.indexRemovalKey()
			// Block implicit preparation until the selected plan finishes.
			p.dslRemoved[key] = true
			if v.Index {
				p.dslPending[key] = v
			}
		}
	}
	if !changed {
		return nil
	}
	p.exists = true
	if err := p.saveLocked(); err != nil {
		p.dslKnown, p.dslPending, p.dslRemoved, p.dsl, p.exists = known, pending, removed, modes, existed
		p.dslInitialSetup = initial
		return err
	}
	return nil
}

func (r *Registry) prepareNewDSL() {
	r.dslAutoMu.Lock()
	defer r.dslAutoMu.Unlock()
	for _, e := range r.all() {
		key := e.indexRemovalKey()
		p := r.prefs
		p.mu.RLock()
		v, ok := p.dslPending[key]
		p.mu.RUnlock()
		if !ok {
			continue
		}
		if err := e.restoreDSLIndex(store.Plan{Contains: v.Contains, FullText: v.FullText}, nil); err != nil {
			logx.Warn("preparing new DSL %s: %v", e.Path, err)
			continue
		}
		p.editMu.Lock()
		p.mu.Lock()
		delete(p.dslPending, key)
		if err := p.saveLocked(); err != nil {
			p.dslPending[key] = v
			logx.Warn("saving new DSL preparation: %v", err)
		}
		p.mu.Unlock()
		p.editMu.Unlock()
	}
}
