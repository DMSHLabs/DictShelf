// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wuweidict/wudict/internal/store"
)

const bulkIndexKey = "bulk-indexes"

type indexOperation struct {
	Dict    string `json:"dict"`
	Feature string `json:"feature"`
	Action  string `json:"action"`
}

type indexWorkStatus struct {
	ID            string   `json:"id"`
	Running       bool     `json:"running"`
	Done          int64    `json:"done"`
	Total         int64    `json:"total"`
	Current       string   `json:"current"`
	Dict          string   `json:"dict"`
	Stage         string   `json:"stage"`
	CurrentDone   int64    `json:"currentDone"`
	CurrentTotal  int64    `json:"currentTotal"`
	Action        string   `json:"action"`
	Indexes       []string `json:"indexes"`
	StopRequested bool     `json:"stopRequested"`
	Canceled      bool     `json:"canceled"`
	Error         string   `json:"error"`
	Result        any      `json:"result,omitempty"`
	Exclusive     bool     `json:"exclusive"`
	BusyDicts     []string `json:"busyDicts"`
}

func (s *Server) indexWorkStatus(requested ...string) indexWorkStatus {
	s.jobs.mu.Lock()
	snapshots := make(map[string]jobStatus)
	for key, j := range s.jobs.jobs {
		snapshots[key] = j.st.copy()
	}
	s.jobs.mu.Unlock()
	key := bulkIndexKey
	st := snapshots[key]
	if len(requested) > 0 && requested[0] != "" {
		key = requested[0]
		st = snapshots[key]
	}
	selectActive := len(requested) == 0 || requested[0] == ""
	if !st.Running && selectActive {
		for _, candidate := range []string{setupWorkKey, "rescan-indexes", reindexKey} {
			if other := snapshots[candidate]; other.Running {
				key, st = candidate, other
				break
			}
		}
	}
	if !st.Running && selectActive {
		for candidate, other := range snapshots {
			if workDictionary(candidate) != "" && other.Running {
				key, st = candidate, other
				break
			}
		}
	}
	out := indexWorkStatus{ID: key, Running: st.Running, Done: st.Done, Total: st.Total, Current: st.Current,
		CurrentDone: st.CurrentDone, CurrentTotal: st.CurrentTotal, Action: st.Action, Indexes: st.Indexes,
		StopRequested: st.StopRequested, Canceled: st.Canceled, Error: st.Err}
	s.admission.mu.Lock()
	out.Exclusive, out.BusyDicts = s.workLocks()
	s.admission.mu.Unlock()
	if key == bulkIndexKey || strings.HasPrefix(key, "single-index:") {
		out.Dict = st.Stage
		out.Result = st.Result
	}
	if key != bulkIndexKey {
		out.Stage = st.Stage
	}
	if key == setupWorkKey {
		if response, ok := st.Result.(*setupResponse); ok {
			var result map[string]any
			if json.Unmarshal(response.Bytes(), &result) == nil {
				out.Result = result
			}
		}
	}
	if strings.HasPrefix(key, "ingest:") {
		out.CurrentDone, out.CurrentTotal = st.Done, st.Total
		out.Done, out.Total = 0, 1
		if e, err := s.reg.get(strings.TrimPrefix(key, "ingest:")); err == nil {
			out.Current = e.probeName()
		}
	}
	if key == "rescan-indexes" {
		if p, ok := st.Result.(rescanIndexProgress); ok {
			out.Done, out.Total, out.Current = int64(max(0, p.At-1)), int64(p.Total), p.Name
			out.CurrentDone, out.CurrentTotal, out.Action, out.Indexes = int64(p.Done), int64(p.Entries), p.Action, p.Indexes
		}
	}
	return out
}

func (s *Server) handleIndexWork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodDelete:
		key := r.URL.Query().Get("id")
		if key == "all" {
			s.jobs.mu.Lock()
			var keys []string
			for candidate, j := range s.jobs.jobs {
				if j.st.Running && (exclusiveWork(candidate) || workDictionary(candidate) != "") {
					keys = append(keys, candidate)
				}
			}
			s.jobs.mu.Unlock()
			for _, candidate := range keys {
				store.IndexDiagnostic("safe stop requested job=%q", candidate)
				s.jobs.requestStop(candidate)
				if candidate == reindexKey {
					s.jobs.cancel(candidate)
				}
			}
			writeJSON(w, s.indexWorkStatus())
			return
		}
		if key != bulkIndexKey && key != setupWorkKey && key != "rescan-indexes" && key != reindexKey && workDictionary(key) == "" {
			httpErr(w, 400, "invalid index job")
			return
		}
		s.jobs.requestStop(key)
		if key == reindexKey {
			s.jobs.cancel(key)
		}
		writeJSON(w, s.indexWorkStatus())
	case http.MethodPost:
		var operations []indexOperation
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&operations); err != nil || len(operations) == 0 || len(operations) > 10000 {
			httpErr(w, 400, "invalid index operation list")
			return
		}
		for _, op := range operations {
			if _, err := s.reg.get(op.Dict); err != nil {
				httpErr(w, 404, "%v", err)
				return
			}
			if op.Feature != "base" && op.Feature != "contains" && op.Feature != "fts" {
				httpErr(w, 400, "invalid index feature")
				return
			}
			if op.Action != "create" && op.Action != "update" && op.Action != "recreate" && op.Action != "delete" {
				httpErr(w, 400, "invalid index action")
				return
			}
			if op.Action == "delete" && op.Feature == "base" && !s.removalOffered(r) {
				httpErr(w, 403, "deleting from another machine is off")
				return
			}
		}
		key := bulkIndexKey
		if r.URL.Query().Get("single") == "1" && len(operations) == 1 {
			key = "single-index:" + operations[0].Dict
		}
		if _, started := s.jobs.start(key, 0, jobStatus{Total: int64(len(operations))}, func(j *job) {
			for _, op := range operations {
				st, _ := s.jobs.status(key)
				if st.StopRequested {
					j.update(func(st *jobStatus) { st.Canceled = true })
					break
				}
				e, err := s.reg.get(op.Dict)
				if err != nil {
					j.update(func(st *jobStatus) { st.Err = err.Error() })
					return
				}
				action := "create"
				if op.Action == "delete" {
					action = "remove"
				} else if op.Action == "update" || op.Action == "recreate" {
					action = "update"
				}
				feature := op.Feature
				if feature == "base" {
					feature = "index"
				} else if feature == "fts" {
					feature = "fullText"
				}
				j.update(func(st *jobStatus) {
					st.Stage = op.Dict
					st.Current = e.probeName()
					st.Action = action
					st.Indexes = []string{feature}
					st.CurrentDone = 0
					st.CurrentTotal = 0
				})
				store.IndexDiagnostic("bulk operation start dictionary=%q feature=%q action=%q", e.probeName(), op.Feature, op.Action)
				lastProgress := time.Time{}
				progress := func(done, total int) {
					if done != total && time.Since(lastProgress) < 200*time.Millisecond {
						return
					}
					lastProgress = time.Now()
					j.update(func(st *jobStatus) { st.CurrentDone = int64(done); st.CurrentTotal = int64(total) })
				}
				if op.Feature == "base" && op.Action == "delete" {
					var result removal
					result, err = s.reg.Remove(op.Dict, true, false)
					j.update(func(st *jobStatus) { st.Result = result })
				} else {
					want := s.currentFeatures(e)
					force := op.Feature == "base" && (op.Action == "update" || op.Action == "recreate")
					set := func(on bool) {
						if op.Feature == "contains" {
							want.Contains = on
						}
						if op.Feature == "fts" {
							want.FullText = on
						}
					}
					if op.Feature != "base" && action == "update" {
						set(false)
						if !e.indexBlocked() {
							err = e.setFeatures(want, progress)
						}
					}
					if err == nil {
						set(op.Action != "delete")
						if e.indexBlocked() && op.Action != "delete" {
							err = e.restoreDSLIndex(store.Plan{FullText: want.FullText, Contains: want.Contains}, progress)
							force = false
						}
						if err == nil {
							err = e.setFeatures(want, progress, force)
						}
					}
				}
				store.IndexDiagnostic("bulk operation finish dictionary=%q feature=%q action=%q error=%v", e.probeName(), op.Feature, op.Action, err)
				if err != nil {
					j.update(func(st *jobStatus) { st.Err = fmt.Sprintf("%s: %v", e.probeName(), err) })
					return
				}
				j.update(func(st *jobStatus) { st.Done++ })
			}
		}); !started {
			httpErr(w, 409, "index operations already running")
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, s.indexWorkStatus(key))
	default:
		writeJSON(w, s.indexWorkStatus(r.URL.Query().Get("id")))
	}
}
