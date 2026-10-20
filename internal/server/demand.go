// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "net/http"

// DemandDictionary is used by the desktop bridge when a user chooses a
// dictionary that needs its prepared headword index.
func (s *Server) DemandDictionary(id string, explicit bool) (bool, error) {
	return s.demandDictionary(id, explicit)
}

// handleDemand supports explicit features that require a prepared headword
// index, such as Browse. Ordinary search and article rendering never request
// indexing; missing indexes are prepared when dictionaries are discovered.
//
// Same-origin only, deliberately absent from corsAllowed (D69): it starts work
// and writes to the user's library folder, which is not what the read-only
// extension grant is for.
func (s *Server) handleDemand(w http.ResponseWriter, r *http.Request) {
	explicit, _ := queryFlag(r.URL.Query(), "explicit")
	indexing, err := s.demandDictionary(r.URL.Query().Get("dict"), explicit)
	if err != nil {
		httpErr(w, 404, "%v", err)
		return
	}
	writeJSON(w, map[string]bool{"indexing": indexing})
}

func (s *Server) demandDictionary(id string, explicit bool) (bool, error) {
	e, err := s.reg.get(id)
	if err != nil {
		return false, err
	}
	// Only an explicit index-dependent action may request preparation here.
	// Discovery queues missing indexes independently of this endpoint.
	if explicit {
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
