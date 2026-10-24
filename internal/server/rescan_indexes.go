// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"sync"
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
	New      *indexOptions      `json:"new,omitempty"`
	Existing rescanIndexActions `json:"existing"`
}

type rescanIndexProgress struct {
	Stage        string   `json:"stage"`
	Active       bool     `json:"active,omitempty"`
	DictionaryID string   `json:"dictionaryId,omitempty"`
	At           int      `json:"at"`
	Total        int      `json:"total"`
	Name         string   `json:"name"`
	Action       string   `json:"action"`
	Indexes      []string `json:"indexes,omitempty"`
	Done         int      `json:"done"`
	Entries      int      `json:"entries"`
	Percent      int      `json:"percent"`
}

func baseIndexRequired(selfIndexed bool, options indexOptions) bool {
	return options.Index && (!selfIndexed || options.Contains || options.FullText)
}

func rescanTargetNeedsWork(e *entry, fresh, selfIndexed bool, options indexOptions, req rescanIndexesRequest) bool {
	if selfIndexed {
		return false // This format's native index covers the app's supported operations.
	}
	if fresh {
		return true
	}
	dir, ok := store.LookupDir(e.Path)
	if !ok {
		// An explicit recreate is also the recovery path for a deliberately
		// removed or missing base index. There is no current plan to inspect,
		// but the user has asked us to create one from the source.
		return req.Existing.Index == "recreate"
	}
	have := store.KeptPlan(store.TextDBPath(dir))
	return req.Existing.Index == "recreate" || have.Contains && req.Existing.Contains != "keep" ||
		have.FullText && req.Existing.FullText != "keep"
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

// desktopRescanIndexes runs the same rescan job as the HTTP API and adapts its
// progress to the native bridge's NDJSON event stream.
func (s *Server) desktopRescanIndexes(ctx context.Context, raw []byte, emit func([]byte)) error {
	if len(raw) > 4096 {
		return errors.New("bad request body: request too large")
	}
	var body rescanIndexesRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Errorf("bad request body: %w", err)
	}
	if body.New == nil || !body.New.Index || !validRescanAction(body.Existing.Index, true) || !validRescanAction(body.Existing.Contains, false) || !validRescanAction(body.Existing.FullText, false) {
		return errors.New("invalid index actions")
	}
	release, admitted := s.admitWork("exclusive-request")
	if !admitted {
		return errors.New("dictionary operations are running; wait for completion or stop them")
	}
	s.startRescanIndexes(body)
	release()
	encode := func(message any) error {
		if emit == nil {
			return nil
		}
		data, err := json.Marshal(message)
		if err == nil {
			emit(data)
		}
		return err
	}
	const key = "rescan-indexes"
	for {
		st, changed, ok := s.jobs.watch(key)
		if !ok {
			return nil
		}
		if !st.Running {
			return encode(map[string]any{"t": "done", "failed": st.Failed, "canceled": st.Canceled})
		}
		if progress, ok := st.Result.(rescanIndexProgress); ok {
			message := struct {
				Type string `json:"t"`
				rescanIndexProgress
			}{"progress", progress}
			if err := encode(message); err != nil {
				return err
			}
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Server) startRescanIndexes(body rescanIndexesRequest) {
	key := "rescan-indexes"
	store.IndexDiagnostic("rescan indexes requested new=%+v existing=%+v", body.New, body.Existing)
	s.jobs.start(key, 0, jobStatus{}, func(j *job) {
		failures := s.reg.updateDictionaryIndexesContext(j.ctx, body, func(progress rescanIndexProgress) {
			j.update(func(st *jobStatus) {
				id := progress.DictionaryID
				if progress.Active {
					found := false
					for _, id := range st.ActiveDicts {
						if id == progress.DictionaryID {
							found = true
							break
						}
					}
					if !found {
						st.ActiveDicts = append(st.ActiveDicts, id)
						st.ActiveProgress = append(st.ActiveProgress, dictionaryWorkProgress{ID: id, Active: true})
					}
				} else {
					for i, id := range st.ActiveDicts {
						if id == progress.DictionaryID {
							st.ActiveDicts = append(st.ActiveDicts[:i], st.ActiveDicts[i+1:]...)
							break
						}
					}
					for i, item := range st.ActiveProgress {
						if item.ID == id {
							st.ActiveProgress[i].Active = false
							break
						}
					}
				}
				if progress.Active {
					for i := range st.ActiveProgress {
						if st.ActiveProgress[i].ID == id {
							st.ActiveProgress[i].Name = progress.Name
							st.ActiveProgress[i].Action = progress.Action
							if len(progress.Indexes) > 0 {
								st.ActiveProgress[i].Feature = progress.Indexes[0]
							}
							st.ActiveProgress[i].Done = int64(progress.Done)
							st.ActiveProgress[i].Total = int64(progress.Entries)
							break
						}
					}
				}
				st.Stage, st.Current, st.Action = progress.Stage, progress.Name, progress.Action
				st.Done, st.Total = int64(progress.At), int64(progress.Total)
				st.CurrentDone, st.CurrentTotal = int64(progress.Done), int64(progress.Entries)
				st.Result = progress
			})
		}, func() bool {
			st, _ := s.jobs.status(key)
			return st.StopRequested
		}, func(id string) (context.Context, func(), bool) {
			ctx, cancel, ok := j.beginDictionary(id)
			if !ok {
				return nil, nil, false
			}
			trackJobDictionary(j, id, true)
			return ctx, func() {
				j.finishDictionary(id, cancel)
				trackJobDictionary(j, id, false)
			}, true
		})
		status, _ := s.jobs.status(key)
		store.IndexDiagnostic("rescan indexes finished failed=%d canceled=%v", len(failures), status.StopRequested)
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
	return r.updateDictionaryIndexesContext(context.Background(), req, progress, nil, nil)
}

func (r *Registry) updateDictionaryIndexesCancelable(req rescanIndexesRequest, progress func(rescanIndexProgress), stop func() bool) []string {
	return r.updateDictionaryIndexesContext(context.Background(), req, progress, stop, nil)
}

func (r *Registry) updateDictionaryIndexesContext(ctx context.Context, req rescanIndexesRequest, progress func(rescanIndexProgress), stop func() bool, beginDictionary func(string) (context.Context, func(), bool)) []string {
	report := func(p rescanIndexProgress) {
		if progress != nil {
			progress(p)
		}
	}
	report(rescanIndexProgress{Stage: "scanning"})
	r.indexQueueMu.Lock()
	defer r.indexQueueMu.Unlock()
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
		e          *entry
		fresh      bool
		nativeOnly bool
		options    indexOptions
	}
	targets := []target{}
	workTargets := []target{}
	p := r.prefs
	rec, _ := p.data()
	for _, e := range r.all() {
		if e.builtin || store.IsTextDB(e.Path) {
			continue
		}
		_, prepared := store.LookupDir(e.Path)
		_, pending := rec.DSLPending[e.indexRemovalKey()]
		fresh := pending || (!prepared && (!previous[dict.CanonPath(e.Path)] || !e.indexBlocked())) ||
			isZIMPath(e.Path) && !hasBrowseIndex(e.Path)
		options := defaults
		t := target{e: e, fresh: fresh, nativeOnly: nativeIndexOnly(e), options: options}
		store.IndexDiagnostic("rescan index candidate source=%q dictionary=%q fresh=%v self_indexed=%v native_only=%v index=%v contains=%v fulltext=%v", e.Path, e.probeName(), fresh, entryHasSelfIndex(e), t.nativeOnly, options.Index, options.Contains, options.FullText)
		if t.nativeOnly {
			store.IndexDiagnostic("rescan index skipped source=%q dictionary=%q reason=native-index", e.Path, e.probeName())
		}
		targets = append(targets, t)
		if rescanTargetNeedsWork(e, fresh, t.nativeOnly, options, req) {
			store.IndexDiagnostic("rescan index selected source=%q dictionary=%q", e.Path, e.probeName())
			workTargets = append(workTargets, t)
		}
	}
	// Save desired pending plans before touching files; a retry/restart uses the
	// requested features, never the older queued plan.
	p.editMu.Lock()
	f, _ := p.data()
	f.DSLKnown = maps.Clone(f.DSLKnown)
	if f.DSLKnown == nil {
		f.DSLKnown = map[string]bool{}
	}
	f.DSLPending = map[string]indexOptions{}
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
		if t.nativeOnly {
			delete(f.DSLRemoved, key) // its native headword index cannot be removed
		} else if t.fresh {
			f.DSLRemoved[key] = true
			if baseIndexRequired(t.nativeOnly, t.options) {
				f.DSLPending[key] = t.options
			}
		}
	}
	err := p.store(f)
	p.editMu.Unlock()
	if err != nil {
		return []string{err.Error()}
	}
	positions := make(map[string]int, len(workTargets))
	for i, t := range workTargets {
		positions[t.e.ID] = i
	}
	var failuresMu sync.Mutex
	var started sync.Map
	addFailure := func(message string) {
		failuresMu.Lock()
		failures = append(failures, message)
		failuresMu.Unlock()
	}
	shouldStop := func() bool { return ctx.Err() != nil || stop != nil && stop() }
	runIndexTasks(workTargets, shouldStop, func(t target) {
		started.Store(t.e.ID, true)
		dictCtx := ctx
		finishDictionary := func() {}
		if beginDictionary != nil {
			var ok bool
			dictCtx, finishDictionary, ok = beginDictionary(t.e.ID)
			if !ok {
				return
			}
			defer finishDictionary()
		}
		i := positions[t.e.ID]
		e := t.e
		store.IndexDiagnostic("rescan index start source=%q dictionary=%q fresh=%v self_indexed=%v native_only=%v", e.Path, e.probeName(), t.fresh, entryHasSelfIndex(e), t.nativeOnly)
		if t.nativeOnly {
			if t.fresh {
				if !t.options.Contains && !t.options.FullText {
					return // the native index already answers headword searches
				}
			} else {
				dir, ok := store.LookupDir(e.Path)
				if !ok {
					return
				}
				have := store.KeptPlan(store.TextDBPath(dir))
				containsWork := have.Contains && req.Existing.Contains != "keep"
				fullTextWork := have.FullText && req.Existing.FullText != "keep"
				if !containsWork && !fullTextWork {
					return
				}
			}
		}
		state := rescanIndexProgress{Stage: "dictionary", DictionaryID: e.ID, At: i + 1, Total: len(workTargets), Name: e.probeName()}
		state.Active = true
		defer func() {
			state.Active = false
			report(state)
		}()
		if t.fresh {
			state.Action = "create"
			if !t.nativeOnly {
				state.Indexes = append(state.Indexes, "index")
			}
			if t.options.Contains {
				state.Indexes = append(state.Indexes, "contains")
			}
			if t.options.FullText {
				state.Indexes = append(state.Indexes, "fullText")
			}
		} else if t.nativeOnly {
			dir, _ := store.LookupDir(e.Path)
			have := store.KeptPlan(store.TextDBPath(dir))
			if (req.Existing.Contains == "delete" || req.Existing.FullText == "delete") &&
				!(req.Existing.Contains == "update" || req.Existing.FullText == "update") {
				state.Action = "remove"
			} else {
				state.Action = "update"
			}
			if have.Contains && req.Existing.Contains != "keep" {
				state.Indexes = append(state.Indexes, "contains")
			}
			if have.FullText && req.Existing.FullText != "keep" {
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
			if dictCtx.Err() != nil {
				return
			}
			if err := e.setIndexRemoved(true); err != nil {
				addFailure(err.Error())
				return
			}
			if dir, ok := store.LookupDir(e.Path); ok {
				e.ingestMu.Lock()
				e.rebuilding.Store(true)
				e.openMu.Lock()
				e.closeNow()
				_, removeErr := store.RemovePrepared(dir)
				e.openMu.Unlock()
				e.rebuilding.Store(false)
				e.ingestMu.Unlock()
				if removeErr != nil {
					addFailure(removeErr.Error())
				}
			}
			return
		}
		want := clearDatabaseRequest{progress: articleProgress, ctx: dictCtx}
		if t.fresh {
			plan := store.Plan{Contains: t.options.Contains, FullText: t.options.FullText}
			want.Index, want.plan = baseIndexRequired(t.nativeOnly, t.options), &plan
		} else {
			dir, ok := store.LookupDir(e.Path)
			if !ok && req.Existing.Index != "recreate" {
				return
			}
			have := store.Plan{}
			if ok {
				have = store.KeptPlan(store.TextDBPath(dir))
			}
			want.Index = (!t.nativeOnly && req.Existing.Index == "recreate") ||
				(req.Existing.Contains == "update" && have.Contains) || (req.Existing.FullText == "update" && have.FullText)
			want.Contains = req.Existing.Contains == "delete"
			want.FullText = req.Existing.FullText == "delete"
		}
		if err := e.clearDatabase(want); err != nil {
			if errors.Is(err, context.Canceled) && dictCtx.Err() != nil && !errors.Is(err, store.ErrIngestTempCleanup) {
				if ctx.Err() == nil {
					return
				}
				return
			}
			store.IndexDiagnostic("dictionary index failed source=%q dictionary=%q action=%q error=%q", e.Path, e.probeName(), state.Action, err.Error())
			addFailure(fmt.Sprintf("%s: %v", e.Path, err))
			return
		}
		p.editMu.Lock()
		mutateErr := p.mutate(func(f *prefsFile) { delete(f.DSLPending, e.indexRemovalKey()) })
		p.editMu.Unlock()
		if mutateErr != nil {
			addFailure(mutateErr.Error())
		}
	})
	if shouldStop() {
		p.editMu.Lock()
		stopErr := p.mutate(func(f *prefsFile) {
			for _, skipped := range workTargets {
				if _, began := started.Load(skipped.e.ID); !began {
					delete(f.DSLPending, skipped.e.indexRemovalKey())
				}
			}
		})
		p.editMu.Unlock()
		if stopErr != nil {
			addFailure("could not clear queued index plans after stop: " + stopErr.Error())
		}
		return failures
	}
	report(rescanIndexProgress{Stage: "cleanup", At: len(workTargets), Total: len(workTargets)})
	if err := r.cleanupLibrary(); err != nil {
		failures = append(failures, err.Error())
	}
	if err := r.rescan(false); err != nil {
		failures = append(failures, err.Error())
	}
	go r.refreshDictionaryIcons()
	return failures
}
