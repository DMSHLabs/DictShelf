// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/url"
	"strconv"
)

// Query strings are an HTTP/legacy-call concern. Search receives the same
// typed parameters from either adapter and applies its own defaults/limits.
func searchRequestFromQuery(q url.Values) SearchRequest {
	n, err := strconv.Atoi(q.Get("n"))
	if err != nil {
		n = 0
	}
	start, _ := strconv.Atoi(q.Get("start"))
	size, _ := strconv.Atoi(q.Get("size"))
	cacheCount, _ := strconv.Atoi(q.Get("cacheCount"))
	cacheOrder, _ := strconv.Atoi(q.Get("cacheOrder"))
	highlight, _ := queryFlag(q, "hl")
	suggestions, _ := queryFlag(q, "suggest")
	ignoreDiacritics, _ := queryFlag(q, "ignoreDiacritics")
	pageCacheDiagnostics, _ := queryFlag(q, "pageCacheDiagnostics")
	searchDiagnostics, _ := queryFlag(q, "searchDiagnostics")
	bodyOnly, _ := queryFlag(q, "bodyOnly")
	bodyIndex := 0
	switch body := q.Get("body"); body {
	case "none":
		bodyIndex = -1
	case "":
	default:
		if index, err := strconv.Atoi(body); err == nil && index >= 0 {
			bodyIndex = index + 1
		}
	}
	return SearchRequest{
		Query: q.Get("q"), Mode: q.Get("mode"), Format: q.Get("format"),
		Dictionary: q.Get("dict"), Limit: n, Highlight: highlight, Suggestions: suggestions,
		IgnoreDiacritics:     ignoreDiacritics,
		BodyIndex:            bodyIndex,
		BodyOnly:             bodyOnly,
		PageCacheDiagnostics: pageCacheDiagnostics,
		SearchDiagnostics:    searchDiagnostics,
		CacheAction:          q.Get("cacheAction"), CacheID: q.Get("cacheID"), CacheCount: cacheCount,
		ReturnCacheID:      q.Get("returnCacheID"),
		ReturnCacheStorage: q.Get("returnCacheStorage"),
		CacheOrder:         cacheOrder, Start: start, Size: size, Rung: q.Get("rung"),
	}
}
