// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"net/url"
	"testing"
)

func TestBrowseFindUsesPreparedHeadwordIndex(t *testing.T) {
	s := newTestServer(t)
	id := getDicts(t, s, "/api/dicts")[0].ID
	path := "/api/browse/find?dict=" + url.QueryEscape(id) + "&q=cor&p=1"
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.setFeatures(features{}, nil); err != nil {
		t.Fatal(err)
	}
	var result browseFindResp
	if rec := getJSON(t, s, path, &result); rec.Code != 200 {
		t.Fatalf("indexed dictionary: status %d: %s", rec.Code, rec.Body.String())
	}
	if result.Total != 1 || result.Page != 1 || result.Pages != 1 || len(result.Words) != 1 || result.Words[0] != "corazón" {
		t.Fatalf("search result = %+v", result)
	}
}

func TestBrowseFindDiskResultCacheServesPageWithoutRescan(t *testing.T) {
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
	const cacheID = "browse-result-cache-test-0001"
	params := url.Values{"dict": {id}, "q": {"cor"}, "matchmode": {"prefix"}, "browsecache": {cacheID}}
	request, err := s.prepareBrowseFindStream(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	var labels, total int
	err = s.streamBrowseFindCached(context.Background(), request, func(event browseFindStreamEvent) error {
		if len(event.Words) != 0 {
			t.Fatalf("cached stream sent %d words to the client", len(event.Words))
		}
		labels += len(event.Alphabet)
		total = event.Total
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || labels != 1 {
		t.Fatalf("stream total=%d, labels=%d", total, labels)
	}
	params.Set("p", "1")
	page, err := s.browseFindPage(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Words) != 1 || page.Words[0] != "corazón" {
		t.Fatalf("cached page = %+v", page)
	}
}
