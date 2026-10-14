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
	highlight, _ := queryFlag(q, "hl")
	return SearchRequest{
		Query: q.Get("q"), Mode: q.Get("mode"), Format: q.Get("format"),
		Dictionary: q.Get("dict"), Limit: n, Highlight: highlight,
	}
}
