// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
)

// The panel's Rebuild: bring every OUTDATED dictionary current, as one job the
// user starts, watches and can stop. Outdated is store.Stale - built by an
// older build, from a source edited since, or unreadable by this one - and is
// what /api/dicts reports per row; the page sums those rows into the one line
// that offers this job, and shows nothing when the sum is zero.
//
// It is never started on anyone's behalf. On a large library a rebuild is
// hours of work, which is why an upgrade that changes the indexing only marks
// the library outdated and waits (store/stale.go). There is deliberately no
// "rebuild everything" here: rebuilding a current dictionary gains nothing,
// and the one case where it might - suspected damage the stamps cannot see -
// is `wudict reindex -all`, from the command line.
//
// Shape: the background lane (indexLimit), one dictionary at a time, so it
// yields to everything the user does in the meantime; HoldActiveProcs for the
// whole job, because a person is watching a progress line and the Android
// shell keeps its foreground service up only while that hold is taken.
// Cancel takes effect between dictionaries: each rebuild is the usual atomic
// temp+rename, so stopping one halfway would only throw its work away.

// ReindexStatus is the job as the page - and `wudict reindex`, handing off to
// a running server - polls it. The zero value is "nothing has run": Total 0
// and not Running.
type ReindexStatus struct {
	Running bool `json:"running"`
	// Done of Total dictionaries finished (failed ones included).
	Done  int `json:"done"`
	Total int `json:"total"`
	// Current is the dictionary being rebuilt, with its own entry progress.
	Current      string `json:"current,omitempty"`
	CurrentDone  int    `json:"currentDone,omitempty"`
	CurrentTotal int    `json:"currentTotal,omitempty"`
	// Failed names each dictionary that could not be rebuilt, with why.
	Failed []string `json:"failed,omitempty"`
	// Canceled: the user stopped the job; what finished stays finished.
	Canceled bool `json:"canceled,omitempty"`
}

// reindexKey is the rebuild's job (jobs.go): one at a time.
const reindexKey = "reindex"

// reindexStatus is the rebuild job as the page and `wudict reindex` poll it.
func (s *Server) reindexStatus() ReindexStatus {
	st, _ := s.jobs.status(reindexKey)
	return ReindexStatus{
		Running: st.Running, Done: int(st.Done), Total: int(st.Total),
		Current: st.Current, CurrentDone: int(st.CurrentDone), CurrentTotal: int(st.CurrentTotal),
		Failed: st.Failed, Canceled: st.Canceled,
	}
}

// entryOutdated is the per-dictionary test the job selects by. It agrees with
// dictInfo.Outdated by construction: a folder PreparedFor accepts is judged by
// store.Stale on its meta, and a folder it refuses (schema, source) is stale
// by the same function.
func entryOutdated(path string) bool {
	if !rebuildable(path) {
		return false
	}
	dir, ok := store.LookupDir(path)
	return ok && len(store.Stale(store.TextDBPath(dir), path)) > 0
}

// handleReindex starts the job over every outdated dictionary. 202 with the
// status when it starts, 200 with the status when there was nothing to do or
// a job is already running (the page then simply follows that one).
func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	var todo []*entry
	for _, e := range s.reg.all() {
		if entryOutdated(e.Path) {
			todo = append(todo, e)
		}
	}
	if len(todo) == 0 {
		writeJSON(w, s.reindexStatus())
		return
	}
	if _, started := s.jobs.start(reindexKey, 0, jobStatus{Total: int64(len(todo))},
		func(j *job) { s.runReindex(j, todo) }); !started {
		writeJSON(w, s.reindexStatus())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, s.reindexStatus())
}

// handleReindexStatus is the poll. Never fails, as the import poll never does.
func (s *Server) handleReindexStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.reindexStatus())
}

// handleReindexCancel stops the job after the dictionary in hand.
func (s *Server) handleReindexCancel(w http.ResponseWriter, r *http.Request) {
	s.jobs.cancel(reindexKey)
	writeJSON(w, s.reindexStatus())
}

func (s *Server) runReindex(j *job, todo []*entry) {
	defer j.update(func(st *jobStatus) {
		st.Current, st.CurrentDone, st.CurrentTotal = "", 0, 0
	})
	for _, e := range todo {
		release := acquire(indexLimit)
		// the lane is FIFO and a rebuild takes minutes: the cancel may have
		// arrived while this slot was being waited for
		if j.canceled() {
			release()
			j.update(func(st *jobStatus) { st.Canceled = true })
			return
		}
		name := e.probeName()
		j.update(func(st *jobStatus) {
			st.Current, st.CurrentDone, st.CurrentTotal = name, 0, 0
		})
		last := time.Time{}
		progress := func(done, total int) {
			if time.Since(last) < 200*time.Millisecond {
				return
			}
			last = time.Now()
			j.update(func(st *jobStatus) { st.CurrentDone, st.CurrentTotal = int64(done), int64(total) })
		}
		_, err := e.refresh(progress)
		release()
		j.update(func(st *jobStatus) {
			st.Done++
			if err != nil {
				st.Failed = append(st.Failed, name+": "+err.Error())
			}
		})
		if err != nil {
			logx.Warn("could not rebuild %s: %v", filepath.Base(e.Path), err)
		}
	}
}

// refresh rebuilds this dictionary's prepared data if, and only if, it is
// outdated, and reports whether anything changed. The text keeps the plan it
// already has - a rebuild the user asked for to bring data current must not
// change what they chose to index - and a media.db that was there before is
// repacked if the rebuild (or its own age) left it unpaired. A dictionary
// never packed is not given media it did not have.
func (e *entry) refresh(progress store.Progress) (bool, error) {
	if !rebuildable(e.Path) {
		return false, nil // no source: nothing to rebuild from
	}
	if _, ok := store.LookupDir(e.Path); !ok {
		return false, nil // never prepared: nothing is outdated
	}
	out, err := e.reconcile(e.probeName(), store.Target{Rebuild: store.IfOutdated, Media: store.MediaKeep}, progress)
	return out.Changed(), err
}
