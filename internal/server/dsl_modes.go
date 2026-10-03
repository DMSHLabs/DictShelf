// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
)

type dslView struct {
	Parser          string `json:"parser"`
	GlobalParser    bool   `json:"globalParser"`
	Source          string `json:"source"`
	Variant         string `json:"variant"`
	Mode            string `json:"mode"`
	Original        bool   `json:"original"`
	GD              bool   `json:"gd"`
	SourceAvailable bool   `json:"sourceAvailable"`
	IndexRemoved    bool   `json:"indexRemoved"`
}

// Resolve once at discovery; cached-only dictionaries use their source receipt.
func dslIdentity(path string) (source, variant string) {
	if strings.EqualFold(filepath.Ext(path), ".db") {
		meta, err := store.ReadMeta(path)
		if err != nil {
			return "", ""
		}
		path = meta["source_path"]
	}
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".dsl"), strings.HasSuffix(lower, ".dsl.dz"):
		return cleanAbs(path), "original"
	case strings.HasSuffix(lower, ".dslgd"):
		input := dict.SourceInput(path)
		if input != path {
			return cleanAbs(input), "gd"
		}
	}
	return "", ""
}

func (p *Prefs) dslMode(source string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	mode := p.dsl[source]
	if p.dslParser == "original" || p.dslParser == "gd" || p.dslParser == "both" {
		return p.dslParser
	}
	if mode == "original" || mode == "gd" {
		return mode
	}
	return "both"
}

func (p *Prefs) parserSelection() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.dslParser == "original" || p.dslParser == "gd" {
		return p.dslParser
	}
	return "both"
}

func (r *Registry) dslView(e *entry) *dslView {
	if e.dslSource == "" {
		return nil
	}
	v := &dslView{Source: e.dslSource, Variant: e.dslVariant, Mode: r.prefs.dslMode(e.dslSource)}
	r.prefs.mu.RLock()
	v.Parser = r.prefs.dslParser
	v.GlobalParser = v.Parser != ""
	r.prefs.mu.RUnlock()
	if v.Parser == "" {
		v.Parser = "both"
	}
	v.SourceAvailable = fileExists(e.dslSource)
	v.IndexRemoved = e.indexBlocked()
	for _, other := range r.all() {
		if other.dslSource != e.dslSource {
			continue
		}
		v.Original = v.Original || other.dslVariant == "original"
		v.GD = v.GD || other.dslVariant == "gd"
	}
	if !v.GD {
		v.Mode = "original"
	} else if !v.Original {
		v.Mode = "gd"
	}
	return v
}

func (r *Registry) dslAvailable(e *entry) bool {
	if e.indexBlocked() {
		return false
	}
	if e.dslSource == "" {
		return true
	}
	if parser := r.prefs.parserSelection(); parser != "both" {
		return parser == e.dslVariant
	}
	mode := r.prefs.dslMode(e.dslSource)
	if mode == "both" || mode == e.dslVariant {
		return true
	}
	// Losing the other source/descriptor must not hide the family's last copy.
	for _, other := range r.all() {
		if other.dslSource == e.dslSource && other.dslVariant == mode {
			return false
		}
	}
	return true
}

func (e *entry) indexBlocked() bool {
	if e.reg == nil {
		return false
	}
	p := e.reg.prefs
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.dslRemoved[e.indexRemovalKey()]
}

func (e *entry) indexRemovalKey() string {
	if e.dslSource != "" {
		return e.dslSource + "\n" + e.dslVariant
	}
	return cleanAbs(e.Path) + "\noriginal"
}

func (e *entry) setIndexRemoved(removed bool) error {
	if e.reg == nil {
		return nil
	}
	p := e.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	key := e.indexRemovalKey()
	old, exists := p.dslRemoved[key], p.exists
	if p.dslRemoved == nil {
		p.dslRemoved = make(map[string]bool)
	}
	if removed {
		p.dslRemoved[key] = true
	} else {
		delete(p.dslRemoved, key)
	}
	p.exists = true
	if err := p.saveLocked(); err != nil {
		if old {
			p.dslRemoved[key] = true
		} else {
			delete(p.dslRemoved, key)
		}
		p.exists = exists
		return err
	}
	return nil
}

// Only an explicit ingest request may restore a deliberately removed index.
func (e *entry) restoreDSLIndex(plan store.Plan, progress store.Progress) error {
	acquire(frontLimit)
	defer release(frontLimit)
	defer HoldActiveProcs()()
	e.ingestMu.Lock()
	defer e.ingestMu.Unlock()
	defer e.rebuilding.Store(false)
	if !e.indexBlocked() {
		return nil
	}
	dir, err := store.ClaimDir(e.Path)
	if err != nil {
		return err
	}
	textDB := store.TextDBPath(dir)
	prepared, ready := store.PreparedFor(e.Path)
	if !ready || prepared != textDB || len(store.Inspect(textDB).TextStale(e.Path)) != 0 || store.KeptPlan(textDB) != plan {
		if _, err := e.reconcileLocked(e.probeName(), store.Target{
			FullText: &plan.FullText, Contains: &plan.Contains, Rebuild: store.Always,
		}, progress); err != nil {
			return err
		}
	}
	// Only discard old copies after the replacement is complete. Explicit
	// database sources are user dictionaries, even if they share a receipt.
	dirs, err := store.SourceFolders(e.Path)
	if err != nil {
		return err
	}
	protected := make(map[string]bool)
	for _, other := range e.reg.all() {
		protected[dict.CanonPath(other.Path)] = true
	}
	for _, old := range dirs {
		if old == dir || protected[dict.CanonPath(store.TextDBPath(old))] {
			continue
		}
		if _, err := store.RemovePrepared(old); err != nil {
			return err
		}
	}
	if err := e.setIndexRemoved(false); err != nil {
		return err
	}
	_ = store.WriteInfo(dir)
	return e.reopen()
}

// Selection never changes descriptors, prepared files, order or group membership.
func (s *Server) handleDSLMode(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Global bool   `json:"global"`
		Dict   string `json:"dict"`
		Mode   string `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&body); err != nil {
		httpErr(w, 400, "invalid DSL selection")
		return
	}
	if body.Mode != "original" && body.Mode != "gd" && body.Mode != "both" {
		httpErr(w, 400, "invalid DSL mode")
		return
	}
	entries := s.reg.all()
	if body.Dict != "" {
		e, err := s.reg.get(body.Dict)
		if err != nil {
			httpErr(w, 404, "%v", err)
			return
		}
		v := s.reg.dslView(e)
		if v == nil {
			httpErr(w, 400, "not a DSL dictionary")
			return
		}
		if (body.Mode != "gd" && !v.Original) || (body.Mode != "original" && !v.GD) {
			httpErr(w, 409, "DSL variant missing")
			return
		}
		entries = []*entry{e}
	}
	p := s.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	p.mu.Lock()
	old, oldExists := p.dsl, p.exists
	oldParser := p.dslParser
	if body.Global {
		p.dslParser = body.Mode
	}
	next := make(map[string]string, len(old)+len(entries))
	for k, v := range old {
		next[k] = v
	}
	for _, e := range entries {
		if e.dslSource != "" {
			next[e.dslSource] = body.Mode
		}
	}
	p.dsl, p.exists = next, true
	err := p.saveLocked()
	if err != nil {
		p.dsl, p.exists = old, oldExists
		p.dslParser = oldParser
	}
	p.mu.Unlock()
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
