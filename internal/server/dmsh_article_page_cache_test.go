// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// The walk's page load and the reader's prefetch ask for the same block within
// a few hundred milliseconds. The second caller waits for the first render
// instead of starting a second one over the same 4.5-7.8 MB.
func TestArticlePageCacheRendersOnePageForConcurrentCallers(t *testing.T) {
	isolatedDBDir(t)
	cache := &articlePageCache{}
	cache.resetFor("0123456789abcdef", false)
	request := SearchRequest{Query: "q", Mode: "phrase", Format: "raw", Dictionary: "d",
		CacheAction: "page", CacheID: "0123456789abcdef", CacheCount: 1, Start: 5, Size: 20,
		Origin: "http://127.0.0.1:6888", Highlight: true}
	var renders int32
	started := make(chan struct{})
	var once sync.Once
	load := func(emit func(SearchEvent) error) error {
		once.Do(func() { close(started) })
		atomic.AddInt32(&renders, 1)
		time.Sleep(80 * time.Millisecond)
		return emit(SearchEvent{T: "hit", Dict: "d", Complete: true,
			Results: []dict.Result{{Headword: "h", Body: "b"}}})
	}
	serve := func() error {
		return cache.serve(context.Background(), request, func(SearchEvent) error { return nil }, load)
	}
	first := make(chan error, 1)
	go func() { first <- serve() }()
	<-started
	second := make(chan error, 1)
	go func() { second <- serve() }()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&renders); got != 1 {
		t.Fatalf("renders = %d; want one page for both callers", got)
	}
}

// The page cache key is what the stored page depends on. Keying on the whole
// request made one block live under several keys - the navigation sets the
// result LIMIT, the client does not; a diagnostic run sets its switches - and
// every duplicate re-rendered 4.5-7.8 MB of article HTML into a 128 MiB cache.
func TestArticlePageCacheKeyReflectsOnlyThePayload(t *testing.T) {
	page := SearchRequest{Query: "джазовый кларнетист", Mode: "phrase", Format: "raw",
		Dictionary: "1757caa7b754", CacheAction: "page", CacheID: "0123456789abcdef",
		CacheCount: 3, CacheOrder: 1, Rung: "phrase", Highlight: true,
		IgnoreDiacritics: true, Start: 5, Size: 20}
	noisy := page
	noisy.Limit = 21
	noisy.PageCacheDiagnostics = true
	noisy.SearchDiagnostics = true
	// Which body a response carries is a window into the same page, not a
	// different page: the window must not split the cache.
	noisy.BodyIndex = 2
	noisy.BodyOnly = true
	noisy.ReturnCacheID = "ffffffffffffffff"
	noisy.ReturnCacheStorage = "disk-temp"
	if articlePageCacheKey(page) != articlePageCacheKey(noisy) {
		t.Fatal("a field that does not shape the page changed the key")
	}
	for name, change := range map[string]func(*SearchRequest){
		"query":       func(r *SearchRequest) { r.Query = "other" },
		"mode":        func(r *SearchRequest) { r.Mode = "near" },
		"format":      func(r *SearchRequest) { r.Format = "clean" },
		"dictionary":  func(r *SearchRequest) { r.Dictionary = "other" },
		"highlight":   func(r *SearchRequest) { r.Highlight = false },
		"diacritics":  func(r *SearchRequest) { r.IgnoreDiacritics = false },
		"rung":        func(r *SearchRequest) { r.Rung = "words" },
		"cache id":    func(r *SearchRequest) { r.CacheID = "fedcba9876543210" },
		"cache count": func(r *SearchRequest) { r.CacheCount = 2 },
		"cache order": func(r *SearchRequest) { r.CacheOrder = 2 },
		"start":       func(r *SearchRequest) { r.Start = 25 },
		"size":        func(r *SearchRequest) { r.Size = 5 },
		"origin":      func(r *SearchRequest) { r.Origin = "http://127.0.0.1:6888" },
	} {
		changed := page
		change(&changed)
		if articlePageCacheKey(page) == articlePageCacheKey(changed) {
			t.Errorf("%s changed the page but not the key", name)
		}
	}
	// The FIRST answer has no window but its limit - that one is part of it.
	first := SearchRequest{Query: "q", Mode: "phrase", Dictionary: "d", CacheID: "0123456789abcdef", Limit: 5}
	wider := first
	wider.Limit = 20
	if articlePageCacheKey(first) == articlePageCacheKey(wider) {
		t.Error("the first answer's limit does not change the key")
	}
}

// A page is stored once as metadata plus per-result body rows. The reader gets
// the metadata and the one body it is about to show; a deferred row asks for
// its own body later, and the walk's page load still assembles the whole page.
func TestArticlePageCacheServesOneBodyFromAStoredPage(t *testing.T) {
	isolatedDBDir(t)
	cache := &articlePageCache{}
	cache.resetFor("0123456789abcdef", false)
	loads := 0
	load := func(emit func(SearchEvent) error) error {
		loads++
		return emit(SearchEvent{T: "hit", Dict: "d", Complete: true, Results: []dict.Result{
			{Headword: "one", Body: "<p>one</p>"},
			{Headword: "two", Body: "<p>two</p>"},
		}})
	}
	page := func(bodyIndex int, bodyOnly bool) []dict.Result {
		request := SearchRequest{Query: "q", Mode: "phrase", Format: "raw", Dictionary: "d",
			CacheAction: "page", CacheID: "0123456789abcdef", CacheCount: 1, Start: 5, Size: 20,
			BodyIndex: bodyIndex, BodyOnly: bodyOnly}
		var results []dict.Result
		if err := cache.serve(context.Background(), request, func(event SearchEvent) error {
			if event.T == "hit" {
				results = event.Results
			}
			return nil
		}, load); err != nil {
			t.Fatal(err)
		}
		return results
	}
	all := page(0, false)
	if len(all) != 2 || all[0].Body != "<p>one</p>" || all[1].Body != "<p>two</p>" {
		t.Fatalf("all bodies = %+v", all)
	}
	metadata := page(-1, false)
	if len(metadata) != 2 || metadata[0].Body != "" || metadata[1].Body != "" || metadata[0].Headword != "one" {
		t.Fatalf("metadata = %+v", metadata)
	}
	second := page(2, true)
	if len(second) != 1 || second[0].Headword != "two" || second[0].Body != "<p>two</p>" {
		t.Fatalf("one body = %+v", second)
	}
	withSecond := page(2, false)
	if len(withSecond) != 2 || withSecond[0].Body != "" || withSecond[1].Body != "<p>two</p>" {
		t.Fatalf("windowed page = %+v", withSecond)
	}
	if loads != 1 {
		t.Fatalf("renders = %d; the page must be rendered once", loads)
	}
}
