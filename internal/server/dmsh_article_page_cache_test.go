// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "testing"

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
