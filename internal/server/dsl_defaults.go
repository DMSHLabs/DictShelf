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

func (p *Prefs) newIndexDefaults() dslIndexOptions {
	f, _ := p.data()
	if f.IndexDefaults != nil {
		v := *f.IndexDefaults
		v.Index = true
		return v
	}
	old := p.newDSLDefaults()
	return dslIndexOptions{Index: true,
		Contains: old.Original.Index && old.Original.Contains || old.GD.Index && old.GD.Contains,
		FullText: old.Original.Index && old.Original.FullText || old.GD.Index && old.GD.FullText}
}

func (s *Server) handleIndexDefaults(w http.ResponseWriter, req *http.Request) {
	var next dslIndexOptions
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&next); err != nil {
		httpErr(w, 400, "invalid index defaults")
		return
	}
	next.Index = true
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	f.IndexDefaults = &next
	f.DSLKnown = maps.Clone(f.DSLKnown)
	if f.DSLKnown == nil {
		f.DSLKnown = map[string]bool{}
	}
	for _, e := range s.reg.all() {
		source := cleanAbs(e.Path)
		if e.dslSource != "" {
			source = e.dslSource
		}
		f.DSLKnown[source] = true
	}
	if err := p.store(f); err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, next)
}

func (p *Prefs) newDSLDefaults() dslDefaults {
	f, _ := p.data()
	if f.DSLDefaults != nil {
		return *f.DSLDefaults
	}
	return dslDefaults{Original: dslIndexOptions{Index: true}}
}

func (r *Registry) queueNewDSL() error {
	entries := r.all()
	current, _ := r.prefs.data()
	families := make(map[string][]*entry)
	for _, e := range entries {
		if e.dslSource != "" {
			families[e.dslSource] = append(families[e.dslSource], e)
		} else if current.IndexDefaults != nil && !e.builtin && !store.IsTextDB(e.Path) {
			families[cleanAbs(e.Path)] = append(families[cleanAbs(e.Path)], e)
		}
	}
	p := r.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	// Existing installations retain their choices until defaults are saved.
	if f.IndexDefaults == nil {
		return nil
	}
	f.DSLKnown = maps.Clone(f.DSLKnown)
	if f.DSLKnown == nil {
		f.DSLKnown = map[string]bool{}
	}
	f.DSLPending = maps.Clone(f.DSLPending)
	if f.DSLPending == nil {
		f.DSLPending = map[string]dslIndexOptions{}
	}
	f.DSLRemoved = maps.Clone(f.DSLRemoved)
	if f.DSLRemoved == nil {
		f.DSLRemoved = map[string]bool{}
	}
	changed := false
	for source, family := range families {
		if f.DSLKnown[source] {
			continue
		}
		f.DSLKnown[source] = true
		changed = true
		// A previously prepared family is not a newly added dictionary.
		existing := false
		for _, other := range family {
			if _, ok := validPrepared(other.Path); ok {
				existing = true
			}
			// Explicit removals and migration plans must survive import defaults.
			if f.DSLRemoved[other.indexRemovalKey()] {
				existing = true
			}
		}
		if existing {
			continue
		}
		for _, other := range family {
			v := *f.IndexDefaults
			key := other.indexRemovalKey()
			// Block implicit preparation until the selected plan finishes.
			f.DSLRemoved[key] = true
			if v.Index {
				f.DSLPending[key] = v
			}
		}
	}
	if !changed {
		return nil
	}
	return p.store(f)
}

func (r *Registry) prepareNewDSL() {
	r.dslAutoMu.Lock()
	defer r.dslAutoMu.Unlock()
	for _, e := range r.all() {
		key := e.indexRemovalKey()
		p := r.prefs
		f, _ := p.data()
		v, ok := f.DSLPending[key]
		if !ok {
			continue
		}
		if err := e.restoreDSLIndex(store.Plan{Contains: v.Contains, FullText: v.FullText}, nil); err != nil {
			logx.Warn("preparing new DSL %s: %v", e.Path, err)
			continue
		}
		p.editMu.Lock()
		err := p.mutate(func(f *prefsFile) { delete(f.DSLPending, key) })
		p.editMu.Unlock()
		if err != nil {
			logx.Warn("saving new DSL preparation: %v", err)
		}
	}
}
