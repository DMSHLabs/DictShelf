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
	Stage   string   `json:"stage"`
	At      int      `json:"at"`
	Total   int      `json:"total"`
	Name    string   `json:"name"`
	Action  string   `json:"action"`
	Indexes []string `json:"indexes,omitempty"`
	Done    int      `json:"done"`
	Entries int      `json:"entries"`
	Percent int      `json:"percent"`
}

func (s *Server) handleStopRescanIndexes(w http.ResponseWriter, req *http.Request) {
	if !s.removalOffered(req) {
		httpErr(w, 403, "deleting from another machine is off")
		return
	}
	s.jobs.requestStop("rescan-indexes")
	writeJSON(w, map[string]bool{"stopping": true})
}

func (s *Server) handleRescanIndexesStatus(w http.ResponseWriter, req *http.Request) {
	st, ok := s.jobs.status("rescan-indexes")
	if !ok {
		writeJSON(w, map[string]any{"running": false})
		return
	}
	progress, _ := st.Result.(rescanIndexProgress)
	writeJSON(w, map[string]any{
		"running": st.Running, "stage": progress.Stage, "at": progress.At,
		"total": progress.Total, "name": progress.Name, "action": progress.Action,
		"indexes": progress.Indexes, "done": progress.Done, "entries": progress.Entries,
		"percent": progress.Percent, "stopRequested": st.StopRequested,
		"canceled": st.Canceled, "failed": st.Failed,
	})
}

func validRescanAction(action string, base bool) bool {
	return action == "" || action == "keep" || (base && action == "recreate") || (!base && (action == "update" || action == "delete"))
}

func (s *Server) startRescanIndexes(body rescanIndexesRequest) {
	key := "rescan-indexes"
	s.jobs.start(key, 0, jobStatus{}, func(j *job) {
		failures := s.reg.updateDictionaryIndexesCancelable(body, func(progress rescanIndexProgress) {
			j.update(func(st *jobStatus) {
				st.Stage, st.Current, st.Action = progress.Stage, progress.Name, progress.Action
				st.Done, st.Total = int64(progress.At), int64(progress.Total)
				st.CurrentDone, st.CurrentTotal = int64(progress.Done), int64(progress.Entries)
				st.Result = progress
			})
		}, func() bool {
			st, _ := s.jobs.status(key)
			return st.StopRequested
		})
		j.update(func(st *jobStatus) { st.Failed, st.Canceled = failures, st.StopRequested })
	})
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
			_ = encoder.Encode(message)
			flusher.Flush()
		}
		key := "rescan-indexes"
		s.startRescanIndexes(body)
		for {
			st, changed, ok := s.jobs.watch(key)
			if !ok {
				break
			}
			if !st.Running {
				emit(map[string]any{"t": "done", "failed": st.Failed, "canceled": st.Canceled})
				break
			}
			if progress, ok := st.Result.(rescanIndexProgress); ok {
				emit(struct {
					Type string `json:"t"`
					rescanIndexProgress
				}{"progress", progress})
			}
			select {
			case <-changed:
			case <-req.Context().Done():
				return
			}
		}
		return
	}
	s.startRescanIndexes(body)
	status := s.jobs.wait("rescan-indexes")
	writeJSON(w, map[string]any{"failed": status.Failed, "canceled": status.Canceled})
}

// The automatic DSL worker must finish before taking the snapshot. Rescans
// during this operation discover only; no background default can undo choices.
func (r *Registry) updateDictionaryIndexes(req rescanIndexesRequest) []string {
	return r.updateDictionaryIndexesProgress(req, nil)
}

func (r *Registry) updateDictionaryIndexesProgress(req rescanIndexesRequest, progress func(rescanIndexProgress)) []string {
	return r.updateDictionaryIndexesCancelable(req, progress, nil)
}

func (r *Registry) updateDictionaryIndexesCancelable(req rescanIndexesRequest, progress func(rescanIndexProgress), stop func() bool) []string {
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
	defaults := r.prefs.newIndexDefaults()
	if req.New != nil {
		defaults = *req.New
	}
	type target struct {
		e       *entry
		fresh   bool
		options dslIndexOptions
	}
	targets := []target{}
	p := r.prefs
	rec, _ := p.data()
	for _, e := range r.all() {
		if e.builtin || store.IsTextDB(e.Path) {
			continue
		}
		_, prepared := store.LookupDir(e.Path)
		_, pending := rec.DSLPending[e.indexRemovalKey()]
		fresh := pending || (!prepared && (!previous[dict.CanonPath(e.Path)] || !e.indexBlocked()))
		options := defaults
		targets = append(targets, target{e, fresh, options})
	}
	// Save desired pending plans before touching files; a retry/restart uses the
	// requested features, never the older queued plan.
	p.editMu.Lock()
	f, _ := p.data()
	f.DSLKnown = maps.Clone(f.DSLKnown)
	if f.DSLKnown == nil {
		f.DSLKnown = map[string]bool{}
	}
	f.DSLPending = map[string]dslIndexOptions{}
	f.DSLRemoved = maps.Clone(f.DSLRemoved)
	if f.DSLRemoved == nil {
		f.DSLRemoved = map[string]bool{}
	}
	f.IndexDefaults = &defaults
	for _, t := range targets {
		key := t.e.indexRemovalKey()
		source := cleanAbs(t.e.Path)
		if t.e.dslSource != "" {
			source = t.e.dslSource
		}
		f.DSLKnown[source] = true
		delete(f.DSLPending, key)
		if t.fresh {
			f.DSLRemoved[key] = true
			if t.options.Index {
				f.DSLPending[key] = t.options
			}
		}
	}
	err := p.store(f)
	p.editMu.Unlock()
	if err != nil {
		return []string{err.Error()}
	}
	for i, t := range targets {
		if stop != nil && stop() {
			p.editMu.Lock()
			stopErr := p.mutate(func(f *prefsFile) {
				for _, skipped := range targets[i:] {
					delete(f.DSLPending, skipped.e.indexRemovalKey())
				}
			})
			p.editMu.Unlock()
			if stopErr != nil {
				failures = append(failures, "could not clear queued index plans after stop: "+stopErr.Error())
			}
			break
		}
		e := t.e
		state := rescanIndexProgress{Stage: "dictionary", At: i + 1, Total: len(targets), Name: e.probeName()}
		if t.fresh {
			state.Action = "create"
			state.Indexes = append(state.Indexes, "index")
			if t.options.Contains {
				state.Indexes = append(state.Indexes, "contains")
			}
			if t.options.FullText {
				state.Indexes = append(state.Indexes, "fullText")
			}
		} else if req.Existing.Index == "recreate" {
			state.Action = "recreate"
			state.Indexes = append(state.Indexes, "index")
		} else if req.Existing.Contains == "delete" || req.Existing.FullText == "delete" {
			state.Action = "remove"
			if req.Existing.Contains == "delete" {
				state.Indexes = append(state.Indexes, "contains")
			}
			if req.Existing.FullText == "delete" {
				state.Indexes = append(state.Indexes, "fullText")
			}
		} else {
			state.Action = "update"
			if req.Existing.Contains == "update" {
				state.Indexes = append(state.Indexes, "contains")
			}
			if req.Existing.FullText == "update" {
				state.Indexes = append(state.Indexes, "fullText")
			}
		}
		report(state)
		last := time.Time{}
		articleProgress := func(done, total int) {
			state.Done, state.Entries = done, total
			if total > 0 {
				state.Percent = min(100, done*100/total)
			}
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
		}
		if err := e.clearDatabase(want); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", e.Path, err))
			continue
		}
		p.editMu.Lock()
		err = p.mutate(func(f *prefsFile) { delete(f.DSLPending, e.indexRemovalKey()) })
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
