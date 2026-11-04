// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http"
)

// DemandDictionary starts or prioritizes preparation of a dictionary's
// headword index when an explicit action needs it.
func (s *Server) DemandDictionary(id string, explicit bool, priority ...bool) (bool, error) {
	wantPriority := len(priority) > 0 && priority[0]
	return s.demandDictionary(id, explicit, wantPriority, false)
}

// PromoteDictionary raises a dictionary that is already waiting in the running
// index queue to the front of it, and does nothing else. It is what Browse asks
// for when a reader arrives at a dictionary that has no index yet: the words
// they asked for are worth taking next instead of last, but opening a page must
// not begin indexing, which is why a dictionary that is not in the queue is left
// exactly as it was. `true` means the dictionary is in the queue (or already
// being indexed under it).
func (s *Server) PromoteDictionary(id string) (bool, error) {
	return s.demandDictionary(id, true, true, true)
}

// handleDemand supports explicit features that require a prepared headword
// index, such as Browse. Ordinary search and article rendering never request
// indexing; missing indexes are prepared when dictionaries are discovered.
//
// Same-origin only, deliberately absent from corsAllowed (D69): it starts work
// and writes to the user's library folder, which is not what the read-only
// extension grant is for.
func (s *Server) handleDemand(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Dict     string `json:"dict"`
		Explicit bool   `json:"explicit"`
		Priority bool   `json:"priority"`
		Promote  bool   `json:"promote"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid dictionary demand")
		return
	}
	indexing, err := s.demandDictionary(request.Dict, request.Explicit, request.Priority, request.Promote)
	if err != nil {
		httpErr(w, 404, "%v", err)
		return
	}
	writeJSON(w, map[string]bool{"indexing": indexing})
}

// promoteQueued asks the index queue that is running right now to take this
// dictionary next, and reports whether it was in that queue. A dictionary that
// is not in it is not added to anything here: this is a reorder, never a start.
func (s *Server) promoteQueued(id string) bool {
	s.reg.priorityMu.Lock()
	queue := s.reg.priorityQueue
	s.reg.priorityMu.Unlock()
	if queue == nil {
		return false
	}
	return queue.prioritize(id)
}

func (s *Server) demandDictionary(id string, explicit, priority, promote bool) (bool, error) {
	e, err := s.reg.get(id)
	if err != nil {
		return false, err
	}
	// `promote` is Browse's reorder-only request (see PromoteDictionary): the
	// dictionary already waiting in the queue is asked to be taken next, and
	// nothing at all happens for one that is not in it.
	if promote {
		return s.promoteQueued(e.ID), nil
	}
	// Only an explicit index-dependent action may request preparation here.
	// Discovery queues missing indexes independently of this endpoint.
	if explicit {
		if priority && s.promoteQueued(e.ID) {
			return true, nil
		}
		e.demandIndex()
	}
	// `indexing` means "work is in flight", and e.indexing() alone does not:
	// the demanded flag is deliberately never cleared on success (demandIndex),
	// which is harmless for the search path - a prepared dictionary is never
	// deferred again - but would make this endpoint answer "true" for a
	// dictionary prepared an hour ago, forever. One stat settles it.
	_, prepared := validPrepared(e.Path)
	return e.indexing() && !prepared, nil
}
