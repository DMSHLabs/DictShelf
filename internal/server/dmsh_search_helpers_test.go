// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "testing"

// collectLookupHits runs one search and returns its per-dictionary rows. It was
// written for the fan-out budget's tests, which are gone with the budget; it
// stays because the power and janitor tests ask their questions through it.
func collectLookupHits(t *testing.T, s *Server, request SearchRequest) []SearchEvent {
	t.Helper()
	var hits []SearchEvent
	for _, event := range collectLookup(t, s, request) {
		if event.T == "hit" {
			hits = append(hits, event)
		}
	}
	return hits
}
