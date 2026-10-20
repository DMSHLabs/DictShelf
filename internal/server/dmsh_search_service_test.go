// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func collectLookup(t *testing.T, s *Server, request SearchRequest) []SearchEvent {
	t.Helper()
	var events []SearchEvent
	if err := s.Search(context.Background(), request, func(event SearchEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestSharedSearchMatchesHTTPAndNativeWithoutRouter(t *testing.T) {
	s := newTestServer(t)
	e := s.reg.all()[0]
	if err := e.setFeatures(features{Contains: true, FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ mode, term, format, limit, highlight string }{
		{"exact", "corazón", "raw", "", ""},
		{"prefix", "ca", "raw", "1", "1"},
		{"fuzzy", "ca", "raw", "2", "0"},
		{"contains", "ora", "raw", "5000", ""},
		{"fts", "vivienda", "raw", "20", "1"},
		{"fts", "órgano muscular", "raw", "20", "1"},
		{"exact", "corazón", "clean", "-1", ""},
		{"exact", "corazón", "text", "overflow", ""},
	} {
		t.Run(tc.mode+"/"+tc.format+"/"+tc.term, func(t *testing.T) {
			q := url.Values{"q": {tc.term}, "mode": {tc.mode}, "format": {tc.format},
				"dict": {e.ID}, "n": {tc.limit}, "hl": {tc.highlight}}
			wireEvents := stream(t, s, "/api/search?"+q.Encode())
			httpEvents := make([]SearchEvent, len(wireEvents))
			for i, event := range wireEvents {
				httpEvents[i] = searchEventFromStream(event)
			}
			request := searchRequestFromQuery(q)
			request.Origin = "http://127.0.0.1:6888"
			mux := s.mux
			s.mux = http.NewServeMux() // Both Go paths must work without /api/search registered.
			defer func() { s.mux = mux }()
			direct := collectLookup(t, s, request)
			payload, _ := json.Marshal(request)
			var native []SearchEvent
			if err := s.DesktopStream(context.Background(), request.Origin, "search", string(payload), func(raw []byte) {
				var event SearchEvent
				if err := json.Unmarshal(raw, &event); err != nil {
					t.Fatal(err)
				}
				native = append(native, event)
			}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(httpEvents, direct) || !reflect.DeepEqual(httpEvents, native) {
				t.Fatalf("lookup outputs differ:\nHTTP: %+v\nGo: %+v\nnative: %+v", httpEvents, direct, native)
			}
			if len(direct) != 3 || direct[0].T != "begin" || direct[1].T != "hit" || len(direct[1].Results) == 0 || direct[2].T != "end" {
				t.Fatalf("missing results or broken progressive protocol: %+v", direct)
			}
			if tc.mode == "fts" && tc.highlight == "1" && !strings.Contains(direct[1].Results[0].Body, "wu-hl") {
				t.Fatal("shared lookup lost full-text highlighting")
			}
		})
	}
}

func TestSharedSearchPreservesRequestedSlotOrder(t *testing.T) {
	s, _ := newPrefsServer(t)
	entries := s.reg.all()
	ids := []string{entries[1].ID, entries[0].ID}
	events := collectLookup(t, s, SearchRequest{Query: "casa", Mode: "exact", Dictionary: strings.Join(ids, ",")})
	if len(events) != 4 || events[0].T != "begin" || events[3].T != "end" {
		t.Fatalf("unexpected protocol: %+v", events)
	}
	for i, slot := range events[0].Slots {
		if slot.Dict != ids[i] {
			t.Fatalf("requested order lost: %+v", events[0].Slots)
		}
	}
	for _, event := range events[1:3] {
		if event.T != "hit" || event.Dict != ids[event.I] {
			t.Fatalf("result did not retain its requested slot: %+v", event)
		}
	}
}

func TestSharedSearchValidationMatchesHTTP(t *testing.T) {
	s := newTestServer(t)
	for _, tc := range []struct {
		query  string
		status int
		kind   SearchErrorKind
	}{
		{"q=", 400, SearchInvalidRequest},
		{"q=casa&mode=invalid", 400, SearchInvalidRequest},
		{"q=casa&format=invalid", 400, SearchInvalidRequest},
		{"q=casa&dict=unknown", 404, SearchDictionaryNotFound},
	} {
		t.Run(tc.query, func(t *testing.T) {
			parameters, _ := url.ParseQuery(tc.query)
			err := s.Search(context.Background(), searchRequestFromQuery(parameters), func(SearchEvent) error {
				t.Fatal("invalid request emitted a result")
				return nil
			})
			var validation *SearchError
			if !errors.As(err, &validation) || validation.Kind != tc.kind {
				t.Fatalf("invalid lookup error: %v", err)
			}
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, newRequest("GET", "/api/search?"+tc.query, nil))
			var response struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.status || response.Error != validation.Error() {
				t.Fatalf("HTTP validation changed: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSharedSearchCancellationAndEmitterFailure(t *testing.T) {
	s, _ := newPrefsServer(t)
	for _, e := range s.reg.all() {
		e.closeNow()
	}
	for _, e := range s.reg.all() {
		if e.d != nil {
			t.Fatal("fixture already opened a dictionary")
		}
	}
	sentinel := errors.New("consumer closed")
	calls := 0
	if err := s.Search(context.Background(), SearchRequest{Query: "casa"}, func(event SearchEvent) error {
		calls++
		if event.T != "begin" {
			t.Fatal("opened dictionaries before begin delivery")
		}
		return sentinel
	}); !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("emitter error not propagated: calls=%d error=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []SearchEvent
	if err := s.Search(ctx, SearchRequest{Query: "casa"}, func(event SearchEvent) error {
		events = append(events, event)
		if event.T == "begin" {
			cancel()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[3].T != "end" {
		t.Fatalf("cancelled lookup did not finish: %+v", events)
	}
	for _, event := range events[1:3] {
		if event.Error != "search cancelled" {
			t.Fatalf("cancellation lost: %+v", event)
		}
	}
	for _, e := range s.reg.all() {
		if e.d != nil {
			t.Fatal("cancelled lookup opened a dictionary")
		}
	}
}

func TestSharedSearchMorphology(t *testing.T) {
	s := morphServer(t, map[string]string{"words.dsl": dslFile("Unlabelled", "know")})
	for _, e := range s.reg.all() {
		e.closeNow()
	}
	events := collectLookup(t, s, SearchRequest{Query: "knew", Mode: "exact"})
	var morphs []SearchEvent
	for _, event := range events {
		if event.T == "morph" {
			morphs = append(morphs, event)
		}
	}
	if len(morphs) != 1 || morphs[0].To != "know" || events[len(events)-1].T != "end" {
		t.Fatalf("shared lookup lost morphology: %+v", events)
	}
}
