// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/url"
	"testing"
)

func TestRemovedNonDSLIndexRequiresExplicitIngest(t *testing.T) {
	s, e := demandEntry(t)
	if e.dslSource != "" {
		t.Fatal("need a non-DSL fixture")
	}
	sse(t, s, "/api/ingest?dict="+e.ID+"&level=headwords")
	if r := deleteReq(t, s, "/api/library?dict="+e.ID+"&prepared=1&source=0"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if !e.indexBlocked() || s.reg.dslAvailable(e) {
		t.Fatal("deleted non-DSL index remained available")
	}
	if _, err := e.open(); err == nil {
		t.Fatal("implicit restore allowed")
	}
	sse(t, s, "/api/ingest?dict="+e.ID+"&level=headwords")
	if e.indexBlocked() || !prepared(e) {
		t.Fatal("explicit ingest did not restore index")
	}
}

// The panel's "🚀 index" chip: level=headwords at a dictionary that has no
// database at all prepares it. The chip exists because this path does NOT open
// the direct backend first (handleIngest), which is what setFeatures does and
// what the heavy, still-unprepared dictionaries cannot afford - but the door
// taken is not visible from outside, so what is asserted here is the outcome:
// a prepared dictionary, at the cheapest level, with nothing else built.
func TestIngestBaseIndexesAnUnpreparedDictionary(t *testing.T) {
	s, e := demandEntry(t)
	if prepared(e) {
		t.Fatal("setup: the fixture was supposed to be unprepared")
	}

	sse(t, s, "/api/ingest?dict="+e.ID)

	if !prepared(e) {
		t.Fatal("the index chip left the dictionary unprepared")
	}
	if f := s.currentFeatures(e); f.FullText || f.Contains || f.Media {
		t.Errorf("the cheap index built more than headwords: %+v", f)
	}
}

// A feature sent is a state, and one left out keeps its value: turning full
// text off leaves contains where it was.
func TestIngestFullTextOffKeepsContains(t *testing.T) {
	s := newTestServer(t)
	id := getDicts(t, s, "/api/dicts")[0].ID
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	sse(t, s, "/api/ingest?dict="+id+"&fts=1&contains=1")
	if f := s.currentFeatures(e); !f.FullText || !f.Contains {
		t.Fatalf("setup: %+v, want full text and contains", f)
	}

	sse(t, s, "/api/ingest?dict="+id+"&fts=off")

	f := s.currentFeatures(e)
	if f.FullText {
		t.Error("fts=off must remove full text")
	}
	if !f.Contains {
		t.Error("a parameter that was not sent must keep its value")
	}
}

// Every boolean parameter reads the same spellings, and "sent" is what lets a
// caller keep a state the request did not mention.
func TestQueryFlag(t *testing.T) {
	for raw, want := range map[string]struct{ on, sent bool }{
		"":           {false, false},
		"x=1":        {true, true},
		"x=true":     {true, true},
		"x=yes":      {true, true},
		"x=0":        {false, true},
		"x=false":    {false, true},
		"x=No":       {false, true},
		"x=off":      {false, true},
		"x":          {false, true},
		"x=&y=1":     {false, true},
		"y=1":        {false, false},
		"x=%20on%20": {true, true},
	} {
		on, sent := queryFlag(mustQuery(t, raw), "x")
		if on != want.on || sent != want.sent {
			t.Errorf("%q: on=%v sent=%v, want %+v", raw, on, sent, want)
		}
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return q
}
