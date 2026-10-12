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

type indexOptions struct {
	Index    bool `json:"index"`
	Contains bool `json:"contains"`
	FullText bool `json:"fullText"`
}

func (p *Prefs) newIndexDefaults() indexOptions {
	f, _ := p.data()
	if f.IndexDefaults != nil {
		v := *f.IndexDefaults
		v.Index = true
		return v
	}
	return indexOptions{Index: true}
}

func (s *Server) handleIndexDefaults(w http.ResponseWriter, req *http.Request) {
	var next indexOptions
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

func (r *Registry) queueNewDictionaries() error {
	entries := r.all()
	families := make(map[string][]*entry)
	for _, e := range entries {
		if e.dslSource != "" {
			families[e.dslSource] = append(families[e.dslSource], e)
		} else if !e.builtin && !store.IsTextDB(e.Path) && !entryHasSelfIndex(e) {
			families[cleanAbs(e.Path)] = append(families[cleanAbs(e.Path)], e)
		}
	}
	p := r.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	f.DSLKnown = maps.Clone(f.DSLKnown)
	if f.DSLKnown == nil {
		f.DSLKnown = map[string]bool{}
	}
	f.DSLPending = maps.Clone(f.DSLPending)
	if f.DSLPending == nil {
		f.DSLPending = map[string]indexOptions{}
	}
	f.DSLRemoved = maps.Clone(f.DSLRemoved)
	if f.DSLRemoved == nil {
		f.DSLRemoved = map[string]bool{}
	}
	changed := false
	for _, e := range entries {
		if !entryHasSelfIndex(e) {
			continue
		}
		key := e.indexRemovalKey()
		_, pending := f.DSLPending[key]
		if pending || f.DSLRemoved[key] || !f.DSLKnown[cleanAbs(e.Path)] {
			store.IndexDiagnostic("discard native-index preparation source=%q pending=%v removed=%v", e.Path, pending, f.DSLRemoved[key])
			delete(f.DSLPending, key)
			delete(f.DSLRemoved, key)
			f.DSLKnown[cleanAbs(e.Path)] = true
			e.gen.Add(1)
			changed = true
		}
	}
	// New dictionaries default to a headword index. Saved defaults only add optional indexes.
	defaults := indexOptions{Index: true}
	if f.IndexDefaults != nil {
		defaults = *f.IndexDefaults
		defaults.Index = true
	}
	for source, family := range families {
		if f.DSLKnown[source] {
			continue
		}
		f.DSLKnown[source] = true
		changed = true
		for _, other := range family {
			if _, ok := validPrepared(other.Path); ok {
				continue
			}
			key := other.indexRemovalKey()
			if f.DSLRemoved[key] {
				continue
			}
			f.DSLRemoved[key] = true
			f.DSLPending[key] = defaults
		}
	}
	if !changed {
		return nil
	}
	return p.store(f)
}

func (r *Registry) prepareNewDictionaries() {
	r.indexQueueMu.Lock()
	defer r.indexQueueMu.Unlock()
	entries := r.all()
	for _, e := range entries {
		key := e.indexRemovalKey()
		p := r.prefs
		f, _ := p.data()
		v, ok := f.DSLPending[key]
		if !ok {
			continue
		}
		if entryHasSelfIndex(e) {
			store.IndexDiagnostic("skip pending index preparation source=%q reason=native-index", e.Path)
			p.editMu.Lock()
			err := p.mutate(func(f *prefsFile) {
				delete(f.DSLPending, key)
				delete(f.DSLRemoved, key)
			})
			p.editMu.Unlock()
			if err != nil {
				logx.Warn("could not discard native-index preparation %s: %v", e.Path, err)
			}
			e.gen.Add(1)
			continue
		}
		jobKey := ingestKey(e.ID)
		indexes := []string{"index"}
		if v.Contains {
			indexes = append(indexes, "contains")
		}
		if v.FullText {
			indexes = append(indexes, "fullText")
		}
		_, started := r.jobs.start(jobKey, 0, jobStatus{Action: "create", Indexes: indexes}, func(j *job) {
			progress := func(done, total int) {
				j.update(func(st *jobStatus) { st.Done, st.Total = int64(done), int64(total) })
			}
			if err := e.restoreIndex(store.Plan{Contains: v.Contains, FullText: v.FullText}, progress, indexLimit); err != nil {
				j.update(func(st *jobStatus) { st.Err = err.Error() })
				logx.Warn("indexing new dictionary %s: %v", e.Path, err)
				return
			}
			if st, _ := r.jobs.status(jobKey); st.StopRequested {
				j.update(func(st *jobStatus) { st.Canceled = true })
				return
			}
			p.editMu.Lock()
			err := p.mutate(func(f *prefsFile) { delete(f.DSLPending, key) })
			p.editMu.Unlock()
			if err != nil {
				j.update(func(st *jobStatus) { st.Err = err.Error() })
				logx.Warn("saving index preparation state: %v", err)
			}
		})
		st := r.jobs.wait(jobKey)
		if st.Canceled || st.StopRequested {
			// Keep this and later entries pending. The next process start or
			// rescan can resume the unfinished work.
			return
		}
		if !started && st.Err != "" {
			logx.Warn("new dictionary index job did not complete: %s: %s", e.Path, st.Err)
		}
	}
}
