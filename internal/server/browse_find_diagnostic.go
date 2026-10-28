// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"net/url"
	"time"

	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/search"
)

type browseFindDiagnosticKey struct{}

func browseFindDiagnosticEnabled(ctx context.Context) bool {
	return ctx.Value(browseFindDiagnosticKey{}) == true
}

func traceBrowseFind(ctx context.Context, q url.Values, send func(browseFindStreamEvent) error,
	run func(context.Context, func(browseFindStreamEvent) error) error) error {
	if q.Get("diagnostics") != "1" {
		return run(ctx, send)
	}
	ctx = context.WithValue(ctx, browseFindDiagnosticKey{}, true)
	mode, source := "browse", "dictionary_entry_fts"
	if q.Get("wildcache") != "" {
		mode, source = "wildcards_dictionary", "dictionary_headword_index"
		if q.Get("within") != "" && q.Get("wildcards") == "1" {
			source = "wildcard_sql_result_cache"
		} else if q.Get("wildcards") == "1" && search.IsSimpleMask(q.Get("q")) {
			source = "wildcard_sql_cache_build"
		}
	}
	if q.Get("merged") == "1" {
		mode, source = "wildcards_merged", "merged_sql_cache"
	}
	matchMode := browseMatchMode(q.Get("matchmode"), q.Get("wildcards") == "1" || q.Get("mask") == "1")
	withinMode := browseMatchMode(q.Get("withinMode"), q.Get("withinWildcards") == "1" || q.Get("withinMask") == "1")
	strategy := browseFindStrategy(matchMode, q.Get("wildcache") != "", q.Get("within") != "")
	candidateLiteral := ""
	if q.Get("merged") == "1" {
		strategy = "merged_sql_candidate_index"
	}
	started := time.Now()
	firstEventMs, firstWordMs, found, scanned, events := int64(-1), int64(-1), 0, 0, 0
	err := run(ctx, func(event browseFindStreamEvent) error {
		events++
		if firstEventMs < 0 {
			firstEventMs = time.Since(started).Milliseconds()
		}
		if event.Total > found {
			found = event.Total
		}
		if event.Strategy != "" {
			strategy = event.Strategy
			candidateLiteral = event.CandidateLiteral
		}
		if event.Scanned > scanned {
			scanned = event.Scanned
		}
		if firstWordMs < 0 && len(event.Words) > 0 {
			firstWordMs = time.Since(started).Milliseconds()
		}
		return send(event)
	})
	logx.SystemDiagnostic("headword_find mode=%s source=%s matchmode=%s strategy=%s candidate_literal=%q dict=%q query=%q within_mode=%s within=%q mask=%v within_mask=%v first_event_ms=%d first_word_ms=%d total_ms=%d found=%d scanned=%d events=%d error=%v",
		mode, source, matchMode, strategy, candidateLiteral, q.Get("dict"), q.Get("q"), withinMode, q.Get("within"),
		matchMode == "wildcards", withinMode == "wildcards", firstEventMs, firstWordMs,
		time.Since(started).Milliseconds(), found, scanned, events, err)
	return err
}
