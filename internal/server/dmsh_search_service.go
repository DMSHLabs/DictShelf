// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/htmlref"
	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/lang"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/search"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// SearchRequest contains lookup parameters independent of either transport.
// An empty Dictionary or "all" selects the library; a comma-separated list
// retains its caller's slot order. Origin is supplied by the host for clean
// article resource links, never read from the desktop JSON payload.
type SearchRequest struct {
	Query                string `json:"q"`
	Mode                 string `json:"mode,omitempty"`
	Format               string `json:"format,omitempty"`
	Dictionary           string `json:"dict,omitempty"`
	Limit                int    `json:"n,omitempty"`
	Highlight            bool   `json:"hl,omitempty"`
	Suggestions          bool   `json:"suggest,omitempty"`
	IgnoreDiacritics     bool   `json:"ignoreDiacritics,omitempty"`
	Origin               string `json:"-"`
	CacheAction          string `json:"cacheAction,omitempty"`
	CacheID              string `json:"cacheID,omitempty"`
	ReturnCacheID        string `json:"returnCacheID,omitempty"`
	ReturnCacheStorage   string `json:"returnCacheStorage,omitempty"`
	CacheCount           int    `json:"cacheCount,omitempty"`
	CacheOrder           int    `json:"cacheOrder,omitempty"`
	Start                int    `json:"start,omitempty"`
	Size                 int    `json:"size,omitempty"`
	Rung                 string `json:"rung,omitempty"`
	PageCacheDiagnostics bool   `json:"pageCacheDiagnostics,omitempty"`
	SearchDiagnostics    bool   `json:"searchDiagnostics,omitempty"`

	skipArticlePageCache bool
}

type SearchErrorKind string

const (
	SearchInvalidRequest     SearchErrorKind = "invalid_request"
	SearchDictionaryNotFound SearchErrorKind = "dictionary_not_found"
)

// SearchError reports lookup validation without imposing HTTP status codes.
type SearchError struct {
	Kind    SearchErrorKind
	Message string
}

func (e *SearchError) Error() string { return e.Message }
func searchRequestError(kind SearchErrorKind, message string, args ...any) error {
	return &SearchError{Kind: kind, Message: fmt.Sprintf(message, args...)}
}

// SearchSlot names one dictionary in the result layout (begin message).
type SearchSlot struct {
	Dict string `json:"dict"`
	Name string `json:"name"`
}

// SearchEvent is one progressively delivered search result event:
//
//	{"t":"begin","slots":[{dict,name}…]}   ordered slot layout
//	{"t":"hit","i":N,dict,name,results…}   one dictionary's results
//	{"t":"end"}                            all dictionaries done
type SearchEvent struct {
	T       string        `json:"t"`
	Slots   []SearchSlot  `json:"slots,omitempty"`
	I       int           `json:"i"`
	Dict    string        `json:"dict,omitempty"`
	Name    string        `json:"name,omitempty"`
	Results []dict.Result `json:"results,omitempty"`
	Skipped bool          `json:"skipped,omitempty"`
	Error   string        `json:"error,omitempty"`

	// Rung says which reading of a full-text query this dictionary answered:
	// "phrase" (the words adjacent, in order), "near" (within ten tokens, any
	// order) or "words" (anywhere in the article). Absent in every other mode.
	//
	// It is reported because the relaxation is silent otherwise, and a search
	// tool that quietly answers a weaker question than the one asked is lying
	// about its result. The UI says so only when the answer is NOT the phrase -
	// the precise reading needs no announcement (D102).
	Rung     string `json:"rung,omitempty"`
	Term     string `json:"term,omitempty"`
	CacheID  string `json:"cacheID,omitempty"`
	Total    int    `json:"total,omitempty"`
	Complete bool   `json:"complete,omitempty"`
	Start    int    `json:"start,omitempty"`
	HasNext  bool   `json:"hasNext,omitempty"`

	// Deferred: this dictionary was not searched because the fan-out cap
	// declined to materialise it (see fanout). It is NOT an error and must not
	// be reported as one: the dictionary is fine, the query was wide, and
	// asking for this one dictionary by itself answers it. Bytes is what the
	// open was estimated to cost, for logs and tests - the UI does not show a
	// number, because a megabyte figure is not a decision the reader can make.
	Deferred bool  `json:"deferred,omitempty"`
	Bytes    int64 `json:"bytes,omitempty"`

	// Indexing: this deferred dictionary is already being prepared, because
	// something asked for it by name. Only ever set beside Deferred, and the
	// difference matters to the reader: "tap to search" is an offer, and
	// repeating it at a dictionary that is midway through a ten-minute ingest
	// reads as nothing having happened the first time.
	Indexing bool `json:"indexing,omitempty"`

	// Legacy morphology event fields retained for decoding older streams.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Lang string `json:"lang,omitempty"`
}

// Search runs one lookup and serializes progressive events to emit. Both the
// HTTP adapter and the native bridge use this method without a listener.
// An emitter error stops delivery and cancels outstanding dictionary queries.
// Validation/emitter errors are returned; dictionary failures and cancellation
// are reported in hit events, retaining the progressive protocol's end event.
func (s *Server) Search(parent context.Context, request SearchRequest, emit func(SearchEvent) error) error {
	if emit == nil {
		return searchRequestError(SearchInvalidRequest, "search event callback is required")
	}
	if !request.skipArticlePageCache && request.ReturnCacheID != "" && request.CacheAction == "" {
		s.articlePages.resetFor(request.ReturnCacheID, request.PageCacheDiagnostics)
	}
	if !request.skipArticlePageCache && s.articlePages.eligible(request) {
		return s.articlePages.serve(parent, request, emit, func(capture func(SearchEvent) error) error {
			request.skipArticlePageCache = true
			return s.Search(parent, request, capture)
		})
	}
	if request.CacheAction == "returnReplay" {
		if request.SearchDiagnostics {
			logx.SystemDiagnostic("search_return_cache stage=replay_start id=%q query=%q mode=%q scope=%q", request.CacheID, request.Query, request.Mode, request.Dictionary)
		}
		if err := s.replaySearchReturnCache(parent, request.CacheID, emit, request.SearchDiagnostics); err == nil {
			if request.SearchDiagnostics {
				logx.SystemDiagnostic("search_return_cache stage=replay_result id=%q result=hit", request.CacheID)
			}
			return nil
		} else if parent.Err() != nil {
			return parent.Err()
		} else if !errors.Is(err, errSearchReturnCacheUnavailable) {
			logx.System("search_return_cache stage=replay_result id=%q result=failed error=%q", request.CacheID, err)
			return err
		} else {
			if request.SearchDiagnostics {
				logx.SystemDiagnostic("search_return_cache stage=fallback id=%q reason=%q query=%q mode=%q scope=%q", request.CacheID, err, request.Query, request.Mode, request.Dictionary)
			}
			request.CacheAction = ""
			request.ReturnCacheID = request.CacheID
		}
	}
	if request.CacheAction != "" {
		return s.searchArticleResultCache(parent, request, emit)
	}
	q := strings.TrimSpace(request.Query)
	if q == "" {
		return searchRequestError(SearchInvalidRequest, "missing q parameter")
	}
	mode, err := search.ParseMode(request.Mode)
	if err != nil {
		return searchRequestError(SearchInvalidRequest, "%v", err)
	}
	format, err := parseFormat(request.Format)
	if err != nil {
		return searchRequestError(SearchInvalidRequest, "%v", err)
	}
	origin := request.Origin
	// Full-text match marking, compiled ONCE for the whole fan-out rather than
	// per result: the terms are the query, and the query does not change while
	// twenty dictionaries answer it.
	//
	// Opt-in, and only here. An API client that did not ask for marks gets the
	// article's own bytes, unchanged - which is what makes "on by default in
	// the UI" (the SPA sends hl=1 from its remembered preference) and "off by
	// default for everyone else" the same rule rather than two. And only in
	// full-text mode: in exact, prefix or contains the match IS the headword
	// the reader clicked, and marking it inside the article would mark the
	// word they are already looking at, on every line it appears (D102).
	hl := request.Highlight
	marks := marker(mode == search.FullText && hl)
	// Capped like the store's own maxLimit: the store clamps at its boundary,
	// but a direct backend treats the limit as a stop sign only, and would
	// materialize every match - article bodies included - before stopping.
	n := request.Limit
	if n <= 0 {
		n = 20
	}
	if n > 501 {
		n = 501
	}

	dictParam := request.Dictionary
	var entries []*entry
	if dictParam == "" || dictParam == "all" {
		entries = s.reg.all()
	} else {
		for _, id := range strings.Split(dictParam, ",") {
			if id = strings.TrimSpace(id); id == "" {
				continue
			}
			if e, err := s.reg.get(id); err == nil {
				entries = append(entries, e)
			}
		}
		if len(entries) == 0 { // none of the requested ids resolved
			return searchRequestError(SearchDictionaryNotFound, "unknown dictionary id %q", dictParam)
		}
	}

	entries = slices.DeleteFunc(entries, func(e *entry) bool { return !s.reg.dslAvailable(e) })
	if len(entries) == 0 && dictParam != "" && dictParam != "all" {
		return searchRequestError(SearchDictionaryNotFound, "dictionary variant unavailable")
	}
	if request.SearchDiagnostics {
		logx.SystemDiagnostic("search start query=%q mode=%q scope=%q dictionaries=%d", q, mode, dictParam, len(entries))
	}

	ctx, stop := context.WithCancel(parent)
	defer stop()
	if request.IgnoreDiacritics {
		ctx = store.WithIgnoreDiacritics(ctx)
	}
	var returnCache *searchReturnCacheEntry
	if request.ReturnCacheID != "" {
		returnCache, err = s.beginSearchReturnCache(request.ReturnCacheID, request.ReturnCacheStorage, request.SearchDiagnostics)
		if err != nil {
			logx.System("search_return_cache stage=create_failed id=%q requested_storage=%q query=%q mode=%q scope=%q dictionaries=%d error=%q",
				request.ReturnCacheID, request.ReturnCacheStorage, q, mode, dictParam, len(entries), err)
		} else if request.SearchDiagnostics {
			logx.SystemDiagnostic("search_return_cache stage=search_attached id=%q query=%q mode=%q scope=%q dictionaries=%d storage=%s",
				request.ReturnCacheID, q, mode, dictParam, len(entries), returnCache.storage)
		}
	}
	defer func() {
		if returnCache != nil && !returnCache.complete {
			returnCache.close("stream_incomplete")
		}
	}()
	var emitErr error
	writeLine := func(m SearchEvent) {
		if emitErr != nil {
			// A disconnected page is still a valid return snapshot: finish the
			// event sequence with what it received before navigation interrupted
			// the live search.
			if returnCache != nil && m.T == "end" {
				if cacheErr := returnCache.append(m, true); cacheErr != nil {
					logx.System("search_return_cache stage=write_failed id=%q error=%q", request.ReturnCacheID, cacheErr)
					returnCache.close("write_failed")
					returnCache = nil
				}
			}
			return
		}
		if returnCache != nil {
			if cacheErr := returnCache.append(m, m.T == "end" && parent.Err() != nil); cacheErr != nil {
				logx.System("search_return_cache stage=write_failed id=%q error=%q", request.ReturnCacheID, cacheErr)
				returnCache.close("write_failed")
				returnCache = nil
			}
		}
		if err := emit(m); err != nil {
			emitErr = err
			stop()
		}
	}

	// Emit the slot layout FIRST, from cheap entry ids only - no opens on the
	// request path. The client paints the empty accordion immediately; each
	// dictionary's real name arrives with its "hit" as it completes. Opening
	// (cold MDX ~180ms, cold SLOB ~1s) is deferred into the workers below, so
	// time-to-first-byte is one open at most, never the sum of all of them.
	begin := make([]SearchSlot, len(entries))
	openers := make([]search.Opener, len(entries))
	// One budget for this query: what the fan-out may newly materialise before
	// it starts declining dictionaries instead of opening them (see fanout).
	// Nil when uncapped, which is the desktop default.
	//
	// A query naming ONE dictionary has no fan-out to cap, and is never capped.
	// The budget bounds the multiplication - N dictionaries materialised at
	// once by a single `all` query - and a search of one is bounded by that one
	// dictionary. It is also, always, a direct demand: the user picked it in
	// the selector, asked for `more…`, or opened a deferred section. Declining
	// it would leave the dictionary unreachable through EVERY path the app has,
	// which is worse than the memory by a wide margin - and the steady state is
	// still held by PREVIEW_MEMORY, the janitor and the relax valve, none of
	// which this bypasses.
	fan := s.reg.fanout()
	demand := len(entries) == 1
	if demand {
		fan = nil
	}
	for i, e := range entries {
		e := e
		begin[i] = SearchSlot{Dict: e.ID, Name: e.ID}
		openers[i] = func() (dict.Dictionary, error) {
			d, err := e.openWithin(fan)
			if err != nil {
				return d, err
			}
			backend, database, index := searchLocationForLog(e, d, mode)
			if request.SearchDiagnostics {
				logx.SystemDiagnostic("search database query=%q mode=%q dictionary=%q backend=%q database=%q index=%q", q, mode, e.probeName(), backend, database, index)
			}
			return d, nil
		}
	}
	writeLine(SearchEvent{T: "begin", Slots: begin})
	if emitErr != nil {
		return emitErr
	}

	// renderHit turns one dictionary's answer into its result event.
	renderHit := func(i int, h search.Hit) SearchEvent {
		id := entries[i].ID
		name := h.Meta.Name
		if name == "" {
			// open failed or cancelled: no Meta. Show the filename, not the
			// opaque id hash, so a broken dictionary is identifiable.
			name = filepath.Base(entries[i].Path)
		}
		// resolve dictionary-internal refs (sound://, relative, absolute)
		// to /res/{dict}/… here, where it is unit-tested.
		// Rewrite first, reduce second: RewriteEntryHTML is what turns a
		// dictionary's internal ref into /res/{dict}/…, and `clean` then
		// absolutises exactly those. Reducing first would throw away the
		// references before they had been named.
		hl := marks(h.Term, h.Rung)
		for j := range h.Results {
			h.Results[j].Body = RewriteEntryHTMLMarked(h.Results[j].Body, id, hl)
		}
		// `clean` and `text` need this dictionary's own CSS to know which of
		// its classes are blocks and which are hidden (D68); derived once per
		// dictionary, from the article that first asks. `raw` never asks, so
		// the desktop path costs nothing.
		var st htmlref.Styles
		if format != formatRaw && len(h.Results) > 0 {
			st = s.articleStyles(id, h.Results[0].Body)
		}
		for j := range h.Results {
			h.Results[j].Body = applyFormat(h.Results[j].Body, format, origin, st)
		}
		m := SearchEvent{T: "hit", I: i, Dict: id, Name: name, Results: h.Results, Skipped: h.Skipped, Rung: h.Rung, Term: h.Term}
		if request.SearchDiagnostics {
			logx.SystemDiagnostic("search result query=%q mode=%q dictionary=%q results=%d skipped=%t", q, mode, name, len(h.Results), h.Skipped)
		}
		var heavy tooHeavy
		switch {
		case errors.As(h.Err, &heavy):
			// Deferred, not failed. The client renders a section the user can
			// open, and opening it re-queries this dictionary alone - which is
			// uncapped above, and which auto-indexes it so the deferral does
			// not recur.
			m.Deferred, m.Bytes = true, heavy.bytes

		case errors.Is(h.Err, context.DeadlineExceeded):
			// Never Go's own words. "context deadline exceeded" is what an
			// unindexed dictionary looked like from the outside while a preview
			// search crawled through its source format, and it told the user
			// nothing they could act on - which is the whole content of the
			// message they need.
			m.Error = "timed out — index this dictionary to search it"
		case errors.Is(h.Err, context.Canceled):
			m.Error = "search cancelled"
		case h.Err != nil:
			m.Error = h.Err.Error()
		}
		if m.Error != "" && !errors.Is(h.Err, context.Canceled) {
			logx.System("search failure dictionary=%q source=%q mode=%q error=%q", name, entries[i].Path, mode, m.Error)
		}
		return m
	}

	// The ceiling exists to stop ONE slow dictionary from holding a wide
	// fan-out open. It is the wrong shape for a search of a single named
	// dictionary: there is nothing to fan out, the user chose this one, and an
	// unprepared dictionary searched through its own format on a phone can
	// legitimately take longer than that to answer. Cutting it off there
	// produced a timeout instead of a result - see indexconst for both clocks.
	budget := indexconst.SearchFanoutBudget
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if request.Suggestions && mode == search.Wildcards {
		search.StreamHeadwordSuggestions(ctx, openers, q, n, func(i int, h search.Hit) {
			writeLine(renderHit(i, h))
		})
	} else {
		search.StreamOpenWithTerms(ctx, openers, mode, q, n, s.searchTermFor(entries, mode, q), func(i int, h search.Hit) {
			writeLine(renderHit(i, h))
		})
	}
	writeLine(SearchEvent{T: "end"})
	return emitErr
}

// searchTermFor adds the lemma to the same dictionary query only in the two
// explicit lemma modes. The store searches both alternatives together.
func (s *Server) searchTermFor(entries []*entry, mode search.Mode, q string) func(int, dict.Meta, string) string {
	if (mode != search.WholeWordLemma && mode != search.ExactLemma) || !s.Morph.Enabled() || strings.ContainsAny(q, " \t\r\n") {
		return nil
	}
	return func(i int, meta dict.Meta, typed string) string {
		code := lang.Resolve(meta.IndexLang, langPath(entries[i].Path), s.reg.Dirs(), meta.Name)
		if code == "" {
			code = "en"
		}
		if !s.Morph.Supports(code) {
			return typed
		}
		lemma, ok := s.Morph.Lemma(code, typed)
		if !ok || lemma == typed {
			return typed
		}
		return typed + "\x1f" + lemma
	}
}
