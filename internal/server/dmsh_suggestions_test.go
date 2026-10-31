// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "testing"

func TestWildcardSuggestionsStreamFirstHeadwordWithoutArticle(t *testing.T) {
	s := newTestServer(t)
	events := collectLookup(t, s, SearchRequest{Query: "*", Mode: "wildcards", Suggestions: true, Limit: 8})
	first := -1
	for i, event := range events {
		if event.T != "hit" || len(event.Results) == 0 {
			continue
		}
		if first < 0 {
			first = i
			if len(event.Results) != 1 {
				t.Fatalf("first suggestion event has %d headwords, want 1", len(event.Results))
			}
		}
		for _, result := range event.Results {
			if result.Body != "" {
				t.Fatalf("suggestion %q loaded an article body", result.Headword)
			}
		}
	}
	if first < 0 || events[0].T != "begin" || events[len(events)-1].T != "end" {
		t.Fatalf("suggestion stream lacks begin, first headword, or end: %+v", events)
	}
}
