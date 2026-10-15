// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Failed        []string `json:"failed,omitempty"`
	Damaged       []string `json:"damaged,omitempty"`
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
	out.Failed = st.Failed
	out.Damaged = st.Damaged
	if len(st.Failed) > 0 {
		out.Error = "index operations failed"
	}
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
		status, err := s.stopIndexWork(r.URL.Query().Get("id"))
		if err != nil {
			var workErr indexWorkError
			if errors.As(err, &workErr) {
				httpErr(w, workErr.status, "%s", workErr.message)
			} else {
				httpErr(w, 500, "%v", err)
			}
			return
		}
		writeJSON(w, status)
	case http.MethodPost:
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			httpErr(w, 400, "invalid index operation list")
			return
		}
		status, err := s.startIndexWork(raw, r.URL.Query().Get("single") == "1", s.removalOffered(r))
		if err != nil {
			var workErr indexWorkError
			if errors.As(err, &workErr) {
				httpErr(w, workErr.status, "%s", workErr.message)
			} else {
				httpErr(w, 500, "%v", err)
			}
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, status)
	default:
		writeJSON(w, s.indexWorkStatus(r.URL.Query().Get("id")))
	}
}

// indexWorkError carries the HTTP status while keeping the shared operation
// usable by the native desktop bridge.

type indexWorkError struct {
	status  int
	message string
}

func (e indexWorkError) Error() string { return e.message }

func (s *Server) startIndexWork(raw []byte, single, allowBaseDelete bool) (indexWorkStatus, error) {
	if len(raw) > 1<<20 {
		return indexWorkStatus{}, indexWorkError{http.StatusRequestEntityTooLarge, "invalid index operation list"}
	}
	var operations []indexOperation
	if err := json.Unmarshal(raw, &operations); err != nil || len(operations) == 0 || len(operations) > 10000 {
		return indexWorkStatus{}, indexWorkError{http.StatusBadRequest, "invalid index operation list"}
	}
	for _, op := range operations {
		if _, err := s.reg.get(op.Dict); err != nil {
			return indexWorkStatus{}, indexWorkError{http.StatusNotFound, err.Error()}
		}
		if op.Feature != "base" && op.Feature != "contains" && op.Feature != "fts" {
			return indexWorkStatus{}, indexWorkError{http.StatusBadRequest, "invalid index feature"}
		}
		if op.Action != "create" && op.Action != "update" && op.Action != "recreate" && op.Action != "delete" {
			return indexWorkStatus{}, indexWorkError{http.StatusBadRequest, "invalid index action"}
		}
		if op.Action == "delete" && op.Feature == "base" && !allowBaseDelete {
			return indexWorkStatus{}, indexWorkError{http.StatusForbidden, "deleting from another machine is off"}
		}
	}
	key := bulkIndexKey
	if single && len(operations) == 1 {
		key = "single-index:" + operations[0].Dict
	}
	release, admitted := s.admitWork(key)
	if !admitted {
		return indexWorkStatus{}, indexWorkError{http.StatusConflict, "dictionary operations are running; wait for completion or stop them"}
	}
	defer release()
	if _, started := s.jobs.start(key, 0, jobStatus{Total: int64(len(operations))}, func(j *job) {
		for _, op := range operations {
			st, _ := s.jobs.status(key)
			if st.StopRequested {
				j.update(func(st *jobStatus) { st.Canceled = true })
				break
			}
			e, err := s.reg.get(op.Dict)
			if err != nil {
				j.update(func(st *jobStatus) { st.Failed = append(st.Failed, fmt.Sprintf("%s: %v", op.Dict, err)); st.Done++ })
				continue
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
				st.Stage, st.Current, st.Action, st.Indexes, st.CurrentDone, st.CurrentTotal = op.Dict, e.probeName(), action, []string{feature}, 0, 0
			})
			store.IndexDiagnostic("index operation start source=%q dictionary=%q feature=%q action=%q self_indexed=%v", e.Path, e.probeName(), op.Feature, op.Action, entryHasSelfIndex(e))
			if entryHasSelfIndex(e) && !isZIMPath(e.Path) {
				store.IndexDiagnostic("skip index operation source=%q dictionary=%q feature=%q action=%q reason=native-index", e.Path, e.probeName(), op.Feature, op.Action)
				j.update(func(st *jobStatus) { st.Done++ })
				continue
			}
			lastProgress := time.Time{}
			progress := func(done, total int) {
				if done == total || time.Since(lastProgress) >= 200*time.Millisecond {
					lastProgress = time.Now()
					j.update(func(st *jobStatus) { st.CurrentDone, st.CurrentTotal = int64(done), int64(total) })
				}
			}
			if op.Feature == "base" && op.Action == "delete" {
				var result removal
				result, err = s.reg.Remove(op.Dict, true, false)
				j.update(func(st *jobStatus) { st.Result = result })
			} else if op.Feature == "contains" && !e.indexBlocked() {
				err = e.setContainsIndex(op.Action, progress)
			} else {
				want := s.currentFeatures(e)
				force := op.Feature != "contains" && (op.Action == "update" || op.Action == "recreate")
				if op.Feature == "contains" {
					want.Contains = op.Action != "delete"
				}
				if op.Feature == "fts" {
					want.FullText = op.Action != "delete"
				}
				if e.indexBlocked() && op.Action != "delete" {
					err = e.restoreIndex(store.Plan{FullText: want.FullText, Contains: want.Contains}, progress, frontLimit)
					force = false
				}
				if err == nil {
					err = e.setFeatures(want, progress, force)
				}
			}
			store.IndexDiagnostic("bulk operation finish dictionary=%q feature=%q action=%q error=%v", e.probeName(), op.Feature, op.Action, err)
			if err != nil {
				name := e.probeName()
				failure := fmt.Sprintf("%s: %v", name, err)
				damaged := damagedDictionaryError(err)
				j.update(func(st *jobStatus) {
					st.Failed = append(st.Failed, failure)
					st.Done++
					if damaged {
						st.Damaged = append(st.Damaged, name)
					}
				})
			} else {
				j.update(func(st *jobStatus) { st.Done++ })
			}
		}
	}); !started {
		return indexWorkStatus{}, indexWorkError{http.StatusConflict, "index operations already running"}
	}
	return s.indexWorkStatus(key), nil
}

func (s *Server) stopIndexWork(key string) (indexWorkStatus, error) {
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
		return s.indexWorkStatus(), nil
	}
	if key != bulkIndexKey && key != setupWorkKey && key != "rescan-indexes" && key != reindexKey && workDictionary(key) == "" {
		return indexWorkStatus{}, indexWorkError{http.StatusBadRequest, "invalid index job"}
	}
	s.jobs.requestStop(key)
	if key == reindexKey {
		s.jobs.cancel(key)
	}
	return s.indexWorkStatus(), nil
}
