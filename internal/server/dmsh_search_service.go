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
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/htmlref"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/search"
	"github.com/wuweidict/wudict/internal/store"
)

// SearchRequest contains lookup parameters independent of either transport.
// An empty Dictionary or "all" selects the library; a comma-separated list
// retains its caller's slot order. Origin is supplied by the host for clean
// article resource links, never read from the desktop JSON payload.
type SearchRequest struct {
	Query            string `json:"q"`
	Mode             string `json:"mode,omitempty"`
	Format           string `json:"format,omitempty"`
	Dictionary       string `json:"dict,omitempty"`
	Limit            int    `json:"n,omitempty"`
	Highlight        bool   `json:"hl,omitempty"`
	IgnoreDiacritics bool   `json:"ignoreDiacritics,omitempty"`
	Origin           string `json:"-"`
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
//	{"t":"morph","from":…,"to":…,"lang":…} the hits that follow are for a
//	                                       DIFFERENT word: nothing matched
//	                                       what was typed, so its dictionary
//	                                       form was searched instead (O3)
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
	Rung string `json:"rung,omitempty"`

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

	// The "morph" line: From is what the user typed, To the dictionary form
	// actually searched, Lang the language that produced it. Every "hit" after
	// it belongs to To - which is why it is a line of its own and not a flag on
	// the hits: the client has to be able to SAY so.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Lang string `json:"lang,omitempty"`
}

const demandSearchBudget = 5 * time.Minute

// Search runs one lookup and serializes progressive events to emit. Both the
// HTTP adapter and the native bridge use this method without a listener.
// An emitter error stops delivery and cancels outstanding dictionary queries.
// Validation/emitter errors are returned; dictionary failures and cancellation
// are reported in hit events, retaining the progressive protocol's end event.
func (s *Server) Search(parent context.Context, request SearchRequest, emit func(SearchEvent) error) error {
	if emit == nil {
		return searchRequestError(SearchInvalidRequest, "search event callback is required")
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
	if n > 500 {
		n = 500
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
	logx.System("search start query=%q mode=%q scope=%q dictionaries=%d", q, mode, dictParam, len(entries))

	ctx, stop := context.WithCancel(parent)
	defer stop()
	if request.IgnoreDiacritics {
		ctx = store.WithIgnoreDiacritics(ctx)
	}
	var emitErr error
	writeLine := func(m SearchEvent) {
		if emitErr != nil {
			return
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
			logx.System("search database query=%q mode=%q dictionary=%q backend=%q database=%q index=%q", q, mode, e.probeName(), backend, database, index)
			return d, nil
		}
	}
	writeLine(SearchEvent{T: "begin", Slots: begin})
	if emitErr != nil {
		return emitErr
	}

	// renderHit turns one dictionary's answer into its result event. Extracted
	// because the morphology wave below emits the same shape for the same
	// slots - a second copy of the rewrite/format pipeline would be a second
	// place for it to drift.
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
		m := SearchEvent{T: "hit", I: i, Dict: id, Name: name, Results: h.Results, Skipped: h.Skipped, Rung: h.Rung}
		logx.System("search result query=%q mode=%q dictionary=%q results=%d skipped=%t", q, mode, name, len(h.Results), h.Skipped)
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
	// fan-out open, and 30 s is right for that. It is the wrong shape for a
	// search of a single named dictionary: there is nothing to fan out, the
	// user chose this one, and an unprepared dictionary searched through its
	// own format on a phone can legitimately take longer than that to answer.
	// Cutting it off there produced a timeout instead of a result.
	budget := 30 * time.Second
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// Metas are kept because each carries the dictionary's declared language,
	// and the morphology wave needs it. Nothing is opened to obtain them: the
	// search callback is invoked for EVERY dictionary, hit or miss.
	metas := make([]dict.Meta, len(entries))
	// missed marks the dictionaries that actually searched and came back
	// EMPTY - the only ones the second wave has anything to ask. One that hit
	// has answered the question; one that was deferred (too heavy), errored,
	// or does not support this mode has said nothing about the word, and
	// asking it the same question again with a lemma gets the same refusal.
	// If nothing missed there is no second wave at all, and no lemma pack is
	// loaded to serve a search that never happened.
	//
	// Per dictionary, not per search (D89): a gate on the WHOLE collection
	// coming back empty would make lemmatization depend on what unrelated
	// dictionaries are installed beside the one being read. One Babylon
	// glossary carrying "estuviera" as a hand-listed alias of "estar" would
	// suppress the lemma probe for every proper Spanish dictionary in the same
	// search - each of which indexes lemmas only, and each of which would
	// answer. The cost of the wider gate is that a language's pack can load on
	// a search that already partly succeeded; it is still bounded by
	// MORPH_CACHE, and the retry still goes only to dictionaries that returned
	// nothing.
	missed := make([]bool, len(entries))
	empty := 0
	search.StreamOpen(ctx, openers, mode, q, n, func(i int, h search.Hit) {
		metas[i] = h.Meta
		// Callbacks are serialised by StreamOpen, so this needs no lock.
		if h.Err == nil && !h.Skipped && len(h.Results) == 0 {
			missed[i], empty = true, empty+1
		}
		writeLine(renderHit(i, h))
	})
	if empty > 0 && !search.HasMask(q) && mode != search.WholeWordLemma && mode != search.ExactLemma {
		s.lemmaWave(ctx, func(event streamMsg) { writeLine(searchEventFromStream(event)) }, func(i int, hit search.Hit) streamMsg { return searchEventToStream(renderHit(i, hit)) }, entries, metas, missed, openers, mode, q, n)
	}
	writeLine(SearchEvent{T: "end"})
	return emitErr
}
