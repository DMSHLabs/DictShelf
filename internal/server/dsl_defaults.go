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
	f, _ := p.data()
	if f.DSLDefaults != nil {
		return *f.DSLDefaults
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
	// One read-decide-write under editMu: the record is a local copy, so a
	// failed save leaves the file as it was - no rollback to spell out.
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	if f.DSLDefaults == nil && len(entries) == 0 && len(f.DSL) == 0 && len(f.DSLKnown) == 0 && f.DSLParser == "" {
		f.DSLInitialSetup = true
	}
	if f.DSLInitialSetup && len(entries) == 0 {
		f.DSLParser = "both"
		if !next.GD.Index {
			f.DSLParser = "original"
		} else if !next.Original.Index {
			f.DSLParser = "gd"
		}
	}
	f.DSLKnown = maps.Clone(f.DSLKnown)
	if f.DSLKnown == nil {
		f.DSLKnown = map[string]bool{}
	}
	// Activating the policy never changes dictionaries already in the registry.
	for _, e := range entries {
		if e.dslSource != "" {
			f.DSLKnown[e.dslSource] = true
		}
	}
	f.DSLDefaults = &next
	if err := p.store(f); err != nil {
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
	f, _ := p.data()
	// Existing installations retain their choices until defaults are saved.
	if f.DSLDefaults == nil {
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
	f.DSL = maps.Clone(f.DSL)
	if f.DSL == nil {
		f.DSL = map[string]string{}
	}
	changed := f.DSLInitialSetup && len(entries) > 0
	if changed {
		f.DSLInitialSetup = false
	}
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
		}
		if existing {
			continue
		}
		mode := "both"
		if !f.DSLDefaults.GD.Index {
			mode = "original"
		} else if !f.DSLDefaults.Original.Index {
			mode = "gd"
		}
		f.DSL[source] = mode
		for _, other := range family {
			v := f.DSLDefaults.Original
			if other.dslVariant == "gd" {
				v = f.DSLDefaults.GD
			}
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
