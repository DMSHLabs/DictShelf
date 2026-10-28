// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"testing"
)

func TestArticlePageCacheLRUAndSearchReset(t *testing.T) {
	id := "article-page-test-0001"
	event := SearchEvent{T: "hit", Complete: true}
	payload, err := json.Marshal([]SearchEvent{event})
	if err != nil {
		t.Fatal(err)
	}
	c := &articlePageCache{maxBytes: int64(len(payload)*2 + 1)}
	c.resetFor(id, false)
	t.Cleanup(func() {
		if c.db != nil {
			_ = c.db.Close()
		}
	})
	loads := 0
	read := func(start int) {
		t.Helper()
		r := SearchRequest{CacheID: id, CacheAction: "page", Start: start, Size: 1}
		if err := c.serve(context.Background(), r, func(got SearchEvent) error {
			if got.T != "hit" || !got.Complete {
				t.Fatalf("unexpected cached event: %+v", got)
			}
			return nil
		}, func(emit func(SearchEvent) error) error {
			loads++
			return emit(event)
		}); err != nil {
			t.Fatal(err)
		}
	}
	read(0)
	read(1)
	read(0) // recently used; the next insert must evict page 1
	read(2)
	if loads != 3 {
		t.Fatalf("loads after LRU insert = %d, want 3", loads)
	}
	read(0)
	if loads != 3 {
		t.Fatalf("recent page was evicted: loads = %d", loads)
	}
	read(1)
	if loads != 4 {
		t.Fatalf("oldest page was not evicted: loads = %d", loads)
	}
	c.resetFor("article-page-test-0002", false)
	id = "article-page-test-0002"
	read(0)
	if loads != 5 {
		t.Fatalf("new search reused old page: loads = %d", loads)
	}
}

func TestArticlePageCacheDoesNotFreezeGrowingResults(t *testing.T) {
	c := &articlePageCache{}
	c.resetFor("article-page-test-growing", false)
	defer func() {
		if c.db != nil {
			_ = c.db.Close()
		}
	}()
	r := SearchRequest{CacheID: "article-page-test-growing", CacheAction: "page", Size: 20}
	loads := 0
	for i := 0; i < 2; i++ {
		if err := c.serve(context.Background(), r, func(SearchEvent) error { return nil }, func(emit func(SearchEvent) error) error {
			loads++
			return emit(SearchEvent{T: "hit", Total: loads, Complete: false, HasNext: true})
		}); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 2 {
		t.Fatalf("incomplete result was cached: loads = %d", loads)
	}
}
