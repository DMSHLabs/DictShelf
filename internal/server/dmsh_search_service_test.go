// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

func TestArticleResultCacheBuildAndReadPortion(t *testing.T) {
	s := newTestServer(t)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		defer s.wildcardPages.mu.Unlock()
		for _, entry := range s.wildcardPages.entries {
			_ = entry.db.Close()
		}
	})
	id := getDicts(t, s, "/api/dicts")[0].ID
	request := SearchRequest{Query: "cor", Mode: "prefix", Dictionary: id, CacheAction: "build", CacheID: "article-result-test-0001"}
	build := collectLookup(t, s, request)
	if len(build) == 0 || !build[len(build)-1].Complete || build[len(build)-1].Total != 1 {
		t.Fatalf("cache build events = %+v", build)
	}
	var cachedIDs string
	if err := s.wildcardPages.entries[request.CacheID].db.QueryRow(
		`SELECT words_json FROM wildcard_block WHERE dict_id=? AND block_no=1`, id).Scan(&cachedIDs); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cachedIDs, "corazón") || strings.Contains(cachedIDs, "<p>") {
		t.Fatalf("article result cache contains article text: %q", cachedIDs)
	}
	request.CacheAction = "page"
	request.Start = 0
	request.Size = 100
	page := collectLookup(t, s, request)
	if len(page) != 1 || page[0].T != "hit" || !page[0].Complete || page[0].Total != 1 ||
		len(page[0].Results) != 1 || page[0].Results[0].Headword != "corazón" {
		t.Fatalf("cached portion = %+v", page)
	}
	query := url.Values{"q": {"cor"}, "mode": {"prefix"}, "dict": {id}, "cacheAction": {"page"},
		"cacheID": {request.CacheID}, "start": {"0"}, "size": {"100"}}
	wire := stream(t, s, "/api/search?"+query.Encode())
	if len(wire) != 1 || wire[0].Total != 1 || len(wire[0].Results) != 1 {
		t.Fatalf("HTTP cached portion = %+v", wire)
	}
	request.Origin = "http://127.0.0.1:6888"
	encoded, _ := json.Marshal(request)
	var native []SearchEvent
	if err := s.DesktopStream(context.Background(), request.Origin, "search", string(encoded), func(raw []byte) {
		var event SearchEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		native = append(native, event)
	}); err != nil {
		t.Fatal(err)
	}
	if len(native) != 1 || native[0].Total != 1 || len(native[0].Results) != 1 {
		t.Fatalf("native cached portion = %+v", native)
	}
	request.Start = 100
	pastEnd := collectLookup(t, s, request)
	if len(pastEnd) != 1 || !pastEnd[0].Complete || pastEnd[0].Total != 1 || len(pastEnd[0].Results) != 0 {
		t.Fatalf("past-end cached portion = %+v", pastEnd)
	}
}

func TestArticleResultCacheSharesOneFileAcrossDictionaries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"first.dsl", "second.dsl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sampleDSL), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	isolatedDBDir(t)
	reg, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, reg)
	closeBackends(t, reg)
	s := New(reg)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		defer s.wildcardPages.mu.Unlock()
		for _, entry := range s.wildcardPages.entries {
			_ = entry.db.Close()
		}
	})
	dicts := getDicts(t, s, "/api/dicts")
	if len(dicts) != 2 {
		t.Fatalf("dictionaries = %d", len(dicts))
	}
	request := SearchRequest{Query: "cor", Mode: "prefix", CacheAction: "build",
		CacheID: "article-result-group-0001", CacheCount: 2}
	for order, dictionary := range dicts {
		request.Dictionary, request.CacheOrder = dictionary.ID, order
		events := collectLookup(t, s, request)
		if len(events) == 0 || !events[len(events)-1].Complete || events[len(events)-1].Total != 1 {
			t.Fatalf("dictionary %d cache build = %+v", order, events)
		}
	}
	files, err := filepath.Glob(filepath.Join(s.wildcardCacheDir(), "wildcard_*.db"))
	if err != nil || len(files) != 1 {
		t.Fatalf("group cache files = %v, %v", files, err)
	}
	request.CacheAction, request.Size = "page", 100
	for order, dictionary := range dicts {
		request.Dictionary, request.CacheOrder = dictionary.ID, order
		page := collectLookup(t, s, request)
		if len(page) != 1 || page[0].Total != 1 || !page[0].Complete ||
			len(page[0].Results) != 1 || page[0].Results[0].Headword != "corazón" {
			t.Fatalf("dictionary %d cached page = %+v", order, page)
		}
	}
}

func TestArticleResultCachePortionCrossesStoredBlock(t *testing.T) {
	dir := t.TempDir()
	var source strings.Builder
	for i := range 625 {
		fmt.Fprintf(&source, "work%04d\n\tdefinition %d\n", i, i)
	}
	if err := os.WriteFile(filepath.Join(dir, "many.dsl"), []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedDBDir(t)
	reg, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, reg)
	closeBackends(t, reg)
	s := New(reg)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		defer s.wildcardPages.mu.Unlock()
		for _, entry := range s.wildcardPages.entries {
			_ = entry.db.Close()
		}
	})
	dicts := getDicts(t, s, "/api/dicts")
	if len(dicts) != 1 {
		t.Fatalf("dictionaries = %d", len(dicts))
	}
	request := SearchRequest{Query: "work", Mode: "prefix", Dictionary: dicts[0].ID,
		CacheAction: "build", CacheID: "article-result-boundary-0001"}
	build := collectLookup(t, s, request)
	if len(build) == 0 || build[len(build)-1].Total != 625 || !build[len(build)-1].Complete {
		t.Fatalf("cache count = %+v", build)
	}
	request.CacheAction, request.Start, request.Size = "page", 275, 100
	page := collectLookup(t, s, request)
	if len(page) != 1 || page[0].Total != 625 || len(page[0].Results) != 100 ||
		page[0].Results[0].Headword != "work0275" || page[0].Results[99].Headword != "work0374" {
		t.Fatalf("cross-block portion = %+v", page)
	}
	request.Start = 600
	tail := collectLookup(t, s, request)
	if len(tail) != 1 || len(tail[0].Results) != 25 || tail[0].HasNext ||
		tail[0].Results[24].Headword != "work0624" {
		t.Fatalf("tail portion = %+v", tail)
	}
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
