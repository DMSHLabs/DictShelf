// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"time"

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
	next, err := s.saveIndexDefaults(next)
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, next)
}

func (s *Server) saveIndexDefaults(next indexOptions) (indexOptions, error) {
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
		return indexOptions{}, err
	}
	return next, nil
}

func (r *Registry) queueNewDictionaries() error {
	entries := r.all()
	families := make(map[string][]*entry)
	for _, e := range entries {
		if e.dslSource != "" {
			families[e.dslSource] = append(families[e.dslSource], e)
		} else if !e.builtin && !store.IsTextDB(e.Path) && !nativeIndexOnly(e) {
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
		if !nativeIndexOnly(e) {
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
	// A headword index is mandatory for a newly added dictionary. Saved defaults
	// choose only the optional contains and full-text indexes. In particular, a
	// first scan must still queue the base index before the setup page has saved
	// those preferences.
	defaults := indexOptions{Index: true}
	if f.IndexDefaults != nil {
		defaults = *f.IndexDefaults
		defaults.Index = true
	}
	for source, family := range families {
		if !f.DSLKnown[source] {
			f.DSLKnown[source] = true
			changed = true
		}
		for _, other := range family {
			key := other.indexRemovalKey()
			if wanted, pending := f.DSLPending[key]; pending {
				// A pending receipt may have been written before the user enabled
				// optional indexes. Keep existing features and add the current
				// defaults so a rescan can finish the requested setup.
				if hasBrowseIndex(other.Path) {
					if prepared, ok := preparedInfo(other.Path); ok {
						wanted.Contains = wanted.Contains || prepared.Plan.Contains || defaults.Contains
						wanted.FullText = wanted.FullText || prepared.Plan.FullText || defaults.FullText
						if f.DSLPending[key] != wanted {
							f.DSLPending[key] = wanted
							changed = true
						}
					}
				}
				if preparedHasIndexes(other.Path, wanted) {
					delete(f.DSLPending, key)
					delete(f.DSLRemoved, key)
					changed = true
				}
				continue
			}
			if f.DSLRemoved[key] {
				continue
			}
			if hasBrowseIndex(other.Path) {
				// The base headword index already exists. Queue only the missing
				// optional indexes, and leave the working base index available
				// while they are built incrementally.
				prepared, ok := preparedInfo(other.Path)
				if !ok {
					continue
				}
				wanted := defaults
				wanted.Contains = wanted.Contains || prepared.Plan.Contains
				wanted.FullText = wanted.FullText || prepared.Plan.FullText
				if preparedHasIndexes(other.Path, wanted) {
					continue
				}
				f.DSLPending[key] = wanted
				changed = true
				continue
			}
			f.DSLRemoved[key] = true
			f.DSLPending[key] = defaults
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return p.store(f)
}

// A finished database can outlive a queued receipt when the process stops
// between index completion and saving preferences. Reuse it when it already
// contains every requested index; an incomplete plan remains queued.
func preparedHasIndexes(path string, wanted indexOptions) bool {
	prepared, ok := preparedInfo(path)
	if !ok || prepared.Meta == nil || isZIMPath(path) && prepared.Meta["index_kind"] != "headwords" {
		return false
	}
	return (!wanted.Contains || prepared.Plan.Contains) &&
		(!wanted.FullText || prepared.Plan.FullText)
}

func (r *Registry) prepareNewDictionaries(parent ...*job) {
	r.indexQueueMu.Lock()
	defer r.indexQueueMu.Unlock()
	type pendingIndex struct {
		entry    *entry
		key      string
		plan     indexOptions
		prepared bool
	}
	var queueJob *job
	if len(parent) > 0 {
		queueJob = parent[0]
	}
	entries := r.all()
	var pending []pendingIndex
	for _, e := range entries {
		key := e.indexRemovalKey()
		p := r.prefs
		f, _ := p.data()
		v, ok := f.DSLPending[key]
		if !ok {
			continue
		}
		if nativeIndexOnly(e) {
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
		pending = append(pending, pendingIndex{entry: e, key: key, plan: v, prepared: hasBrowseIndex(e.Path)})
	}
	if queueJob != nil {
		queueJob.update(func(st *jobStatus) {
			st.Total = int64(len(pending))
			st.QueuedDicts = make([]string, 0, len(pending))
			for _, item := range pending {
				st.QueuedDicts = append(st.QueuedDicts, item.entry.ID)
			}
		})
	}
	if len(pending) == 0 {
		return
	}
	process := func(item pendingIndex) {
		e := item.entry
		ctx := context.Background()
		cancel := func() {}
		if queueJob != nil {
			var ok bool
			ctx, cancel, ok = queueJob.beginDictionary(e.ID)
			if !ok {
				return
			}
			queueJob.update(func(st *jobStatus) {
				for i, id := range st.QueuedDicts {
					if id == e.ID {
						st.QueuedDicts = append(st.QueuedDicts[:i], st.QueuedDicts[i+1:]...)
						break
					}
				}
			})
			trackJobDictionary(queueJob, e.ID, true)
			defer func() {
				queueJob.finishDictionary(e.ID, cancel)
				trackJobDictionary(queueJob, e.ID, false)
			}()
			indexes := []string{"index"}
			if item.prepared {
				indexes = indexes[:0]
			}
			if item.plan.Contains {
				indexes = append(indexes, "contains")
			}
			if item.plan.FullText {
				indexes = append(indexes, "fullText")
			}
			queueJob.update(func(st *jobStatus) {
				action := "create"
				if item.prepared {
					action = "update"
				}
				st.Stage, st.Current, st.Action, st.Indexes = e.ID, e.probeName(), action, indexes
				st.CurrentDone, st.CurrentTotal = 0, 0
			})
			feature := "index"
			action := "create"
			if item.prepared {
				feature, action = "", "update"
			}
			updateJobDictionaryProgress(queueJob, e.ID, e.probeName(), feature, action, 0, 0)
		}
		lastProgress := time.Time{}
		progress := func(done, total int) {
			if queueJob == nil || done != total && time.Since(lastProgress) < 200*time.Millisecond {
				return
			}
			lastProgress = time.Now()
			feature := "index"
			action := "create"
			if item.prepared {
				feature, action = "", "update"
			}
			updateJobDictionaryProgress(queueJob, e.ID, e.probeName(), feature, action, int64(done), int64(total))
		}
		var err error
		if item.prepared {
			media := false
			if prepared, ok := preparedInfo(e.Path); ok {
				media = prepared.MediaPaired()
			}
			err = e.setFeaturesContext(ctx, features{Contains: item.plan.Contains, FullText: item.plan.FullText, Media: media}, progress)
		} else {
			err = e.restoreIndexContext(ctx, store.Plan{Contains: item.plan.Contains, FullText: item.plan.FullText}, progress, currentIndexLimit())
		}
		if err != nil {
			canceled := errors.Is(err, context.Canceled) && ctx.Err() != nil
			if queueJob != nil {
				queueJob.update(func(st *jobStatus) {
					if !canceled || errors.Is(err, store.ErrIngestTempCleanup) {
						st.Failed = append(st.Failed, e.probeName()+": "+err.Error())
					}
				})
			}
			// A failed automatic index is not a deliberate removal. Keep its
			// pending plan for retry, but let direct search use the source.
			if !canceled {
				if unblockErr := e.setIndexRemoved(false); unblockErr != nil {
					logx.Warn("could not unblock dictionary after failed index %s: %v", e.Path, unblockErr)
				}
			}
			store.IndexDiagnostic("new dictionary index failed source=%q dictionary=%q error=%q", e.Path, e.probeName(), err.Error())
			logx.Warn("indexing new dictionary %s: %v", e.Path, err)
		} else {
			p := r.prefs
			p.editMu.Lock()
			clearErr := p.mutate(func(f *prefsFile) { delete(f.DSLPending, item.key) })
			p.editMu.Unlock()
			if clearErr != nil {
				if queueJob != nil {
					queueJob.update(func(st *jobStatus) { st.Failed = append(st.Failed, e.probeName()+": "+clearErr.Error()) })
				}
				logx.Warn("saving index preparation state: %v", clearErr)
			}
		}
		if queueJob != nil {
			queueJob.update(func(st *jobStatus) { st.Done++ })
		}
	}
	runIndexTasks(pending, func() bool { return queueJob != nil && queueJob.canceled() }, process)
	if queueJob != nil && queueJob.canceled() {
		queueJob.update(func(st *jobStatus) { st.Canceled = true })
	}
}
