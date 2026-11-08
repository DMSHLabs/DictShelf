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
	"github.com/DMSHLabs/DictShelf/internal/ftsq"
	"github.com/DMSHLabs/DictShelf/internal/htmlref"
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
	// "phrase" (the words adjacent, in order), "near" (within the configured
	// window, ten tokens by default, any order) or "words" (anywhere in the
	// article). Absent in every other mode.
	//
	// It is reported because the relaxation is silent otherwise, and a search
	// tool that quietly answers a weaker question than the one asked is lying
	// about its result. The UI says so only when the answer is NOT the phrase -
	// the precise reading needs no announcement (D102).
	Rung string `json:"rung,omitempty"`
	// OneReading says the dictionary has a single reading and never tried
	// another, so Rung is not a relaxation and must not be announced as one
	// (an archive answering from its own index - see search.Hit.OneReading).
	OneReading bool `json:"oneReading,omitempty"`

	// NeedsIndex says this dictionary would have to read article texts to
	// answer the reading, and declined to (search.Hit.NeedsIndex). What the
	// reader gets instead is an offer to build our index for it: the same
	// reading, answered from positions.
	NeedsIndex bool   `json:"needsIndex,omitempty"`
	Term       string `json:"term,omitempty"`
	CacheID    string `json:"cacheID,omitempty"`
	Total      int    `json:"total,omitempty"`
	Complete   bool   `json:"complete,omitempty"`
	Start      int    `json:"start,omitempty"`
	HasNext    bool   `json:"hasNext,omitempty"`

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
	// A query wider than the token cap cannot be run as asked, and running the
	// first 128 words of it in silence is the one thing not to do: say so.
	if ftsq.Parse(q).TooMany() {
		return searchRequestError(SearchInvalidRequest, "the query has more than %d words to search", ftsq.MaxTokens)
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
	marks := marker(mode.IsFullText() && hl)
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
	for i, e := range entries {
		e := e
		begin[i] = SearchSlot{Dict: e.ID, Name: e.ID}
		openers[i] = func() (dict.Dictionary, error) {
			d, err := e.searchOpen()
			if err != nil {
				return d, err
			}
			if request.SearchDiagnostics {
				backend, database, index := searchLocationForLog(e, d, mode)
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
		// The marks follow the reading that ANSWERED, not the one that was asked
		// for: a dictionary whose archive index has no positions answers a
		// phrase with the words reading (search.Hit.NeedsIndex), and marking the
		// phrase there would mark nothing at all - which is exactly what the
		// reader saw (found 2026-11-02).
		reading := h.Rung
		if h.NeedsIndex {
			reading = "words"
		}
		hl := marks(h.Term, reading)
		var assetTokens func(int64, string) (string, bool)
		if len(h.Results) > 0 {
			if d, err := entries[i].open(); err == nil {
				if resolver, ok := d.(interface {
					AssetToken(int64, string) (string, bool)
				}); ok {
					assetTokens = resolver.AssetToken
				}
			}
		}
		for j := range h.Results {
			s.decorateFavourite(id, &h.Results[j])
			if s.isFavouriteDictionaryID(id) && len(h.Results[j].ResourceMap) > 0 {
				h.Results[j].Body = RewriteFavouriteHTMLMarked(h.Results[j].Body, id, h.Results[j].ResourceMap, hl)
			} else {
				h.Results[j].Body = rewriteEntryHTMLFor(h.Results[j].Body, id, h.Results[j].EntryID, hl, assetTokens)
			}
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
		m := SearchEvent{T: "hit", I: i, Dict: id, Name: name, Results: h.Results, Skipped: h.Skipped,
			Rung: h.Rung, OneReading: h.OneReading, NeedsIndex: h.NeedsIndex, Term: h.Term}
		if request.SearchDiagnostics {
			logx.SystemDiagnostic("search result query=%q mode=%q dictionary=%q results=%d skipped=%t", q, mode, name, len(h.Results), h.Skipped)
		}
		switch {
		case errors.Is(h.Err, context.DeadlineExceeded):
			// A deadline can only come from below now - a sub-call with its own
			// bound - since the search itself has none. Never Go's own words, and
			// never advice: this dictionary has an index (one without is not
			// searched at all), it just did not finish.
			m.Error = searchDidNotFinish
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
	// No deadline: the search runs until every dictionary has answered, and what
	// ends it early is the reader (Stop, or a new search) - never a clock. See
	// indexconst for why the fan-out clock is gone and what replaces it.
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

// searchDidNotFinish is what a dictionary whose search produced a deadline is
// told: it did not finish, and that is all the reader needs. It is deliberately
// NOT advice - a dictionary without an index is not searched at all (the opener
// refuses it with its own message), so "index this dictionary" was advice that
// could only ever be wrong here. It used to be two messages because one fan-out
// clock covered two situations: a dictionary with nothing prepared, and one that
// merely waited too long for its turn. Both are gone with the clock (indexconst).
const searchDidNotFinish = "timed out — the search of this dictionary did not finish"

// searchTermFor adds the lemma to the same dictionary query only in the two
// explicit lemma modes. The store searches both alternatives together.
func (s *Server) searchTermFor(entries []*entry, mode search.Mode, q string) func(int, dict.Meta, string) string {
	if (mode != search.WholeWordLemma && mode != search.ExactLemma) || !s.Morph.Enabled() || strings.ContainsAny(q, " \t\r\n") {
		return nil
	}
	return func(i int, meta dict.Meta, typed string) string {
		if s.isFavouriteDictionaryID(entries[i].ID) {
			return typed
		}
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
