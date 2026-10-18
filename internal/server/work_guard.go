// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

type workAdmission struct {
	mu      sync.Mutex
	pending map[string]int
}

func workDictionary(key string) string {
	for _, prefix := range []string{"ingest:", "single-index:"} {
		if strings.HasPrefix(key, prefix) {
			return strings.TrimPrefix(key, prefix)
		}
	}
	return ""
}
func exclusiveWork(key string) bool {
	return key == bulkIndexKey || key == setupWorkKey || key == "rescan-indexes" || key == reindexKey || key == "exclusive-request"
}

// Caller holds admission.mu. Reservations cover the interval before a handler
// registers its job; running jobs cover the interval after its request ends.
func (s *Server) workLocks() (bool, []string) {
	exclusive := false
	ids := map[string]bool{}
	add := func(key string) {
		if exclusiveWork(key) {
			exclusive = true
		}
		if id := workDictionary(key); id != "" {
			ids[id] = true
		}
	}
	for key, count := range s.admission.pending {
		if count > 0 {
			add(key)
		}
	}
	s.jobs.mu.Lock()
	for key, j := range s.jobs.jobs {
		if !j.st.Running {
			continue
		}
		if key == "initial-index-queue" {
			for _, id := range j.st.QueuedDicts {
				ids[id] = true
			}
			for _, id := range j.st.ActiveDicts {
				ids[id] = true
			}
			continue
		}
		add(key)
	}
	s.jobs.mu.Unlock()
	dictionaries := make([]string, 0, len(ids))
	for id := range ids {
		dictionaries = append(dictionaries, id)
	}
	return exclusive, dictionaries
}

func (s *Server) withWorkAdmission(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := ""
		preflightOnly := false
		switch r.URL.Path {
		case "/api/ingest":
			if r.Method == "GET" {
				id := r.URL.Query().Get("dict")
				// Reattaching to an existing stream does not request another operation.
				if st, ok := s.jobs.status(ingestKey(id)); ok && st.Running && len(r.URL.Query()) == 1 {
					next(w, r)
					return
				}
				key = ingestKey(id)
			}
		case "/api/setup":
			if save, _ := queryFlag(r.URL.Query(), "save"); save {
				key = "exclusive-request"
			}
		case "/api/index-defaults":
			if r.Method == "PUT" {
				key = "exclusive-request"
			}
		case "/api/rescan":
			if r.Method == "POST" || (r.Method == "GET" && r.URL.Query().Get("status") != "1") {
				key = "exclusive-request"
			}
		case "/api/library":
			if r.Method == "DELETE" {
				key = "exclusive-request"
			}
		case "/api/reindex":
			// StartReindex owns admission so the HTTP route and native bridge
			// enforce the same operation boundary.
		case "/api/index-work":
			if r.Method == http.MethodPost {
				key, preflightOnly = bulkIndexKey, true
				if r.URL.Query().Get("single") == "1" {
					body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
					if err == nil {
						r.Body = io.NopCloser(bytes.NewReader(body))
						var operations []indexOperation
						if len(body) <= 1<<20 && json.Unmarshal(body, &operations) == nil && len(operations) == 1 && operations[0].Dict != "" {
							key = "single-index:" + operations[0].Dict
						}
					}
				}
			}
		case "/api/clear-database", "/api/orphans", "/api/intake", "/api/howto":
			if r.Method == "POST" {
				key = "exclusive-request"
			}
		}
		if key == "" {
			next(w, r)
			return
		}
		release, ok := s.admitWork(key)
		if !ok {
			httpErr(w, http.StatusConflict, "dictionary operations are running; wait for completion or stop them")
			return
		}
		if preflightOnly {
			release()
			next(w, r)
			return
		}
		defer release()
		next(w, r)
	}
}

// admitWork reserves an operation before it creates a job. Running jobs and
// pending requests share one check, so HTTP and native callers have the same
// race-free admission boundary.
func (s *Server) admitWork(key string) (func(), bool) {
	s.admission.mu.Lock()
	exclusive, ids := s.workLocks()
	blocked := exclusive
	if exclusiveWork(key) {
		blocked = blocked || len(ids) > 0
	} else {
		for _, id := range ids {
			if id == workDictionary(key) {
				blocked = true
			}
		}
	}
	if blocked {
		s.admission.mu.Unlock()
		return nil, false
	}
	if s.admission.pending == nil {
		s.admission.pending = map[string]int{}
	}
	s.admission.pending[key]++
	s.admission.mu.Unlock()
	return func() {
		s.admission.mu.Lock()
		s.admission.pending[key]--
		s.admission.mu.Unlock()
	}, true
}
