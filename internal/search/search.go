// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package search fans one query out over many dictionaries concurrently
// (draego's "all" mode, minus its sequential per-request DB opens -
// FTS-audit #6: dictionaries stay open, queries run in parallel).
package search

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/ftsq"
	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// Mode selects the query type; dictionaries lacking the capability are
// skipped (Hit.Skipped).
type Mode int

const (
	Exact Mode = iota
	Prefix
	Contains
	FullText
	Phrase
	Near
	Wildcards
	Suffix
	WholeWord
	WholeWordLemma
	ExactLemma
)

func (m Mode) String() string {
	return [...]string{"exact", "prefix", "contains", "fts", "phrase", "near", "wildcards", "suffix", "word", "wordLemma", "exactLemma"}[m]
}

// ParseMode reads a mode by its API name. "" is prefix, the default; "fuzzy"
// is the retired name of what prefix now does (D16) and is still accepted.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "", "prefix", "fuzzy":
		return Prefix, nil
	case "exact":
		return Exact, nil
	case "contains":
		return Contains, nil
	case "fts":
		return FullText, nil
	case "phrase":
		return Phrase, nil
	case "near":
		return Near, nil
	case "wildcards", "mask":
		return Wildcards, nil
	case "suffix":
		return Suffix, nil
	case "word":
		return WholeWord, nil
	case "wordLemma":
		return WholeWordLemma, nil
	case "exactLemma":
		return ExactLemma, nil
	}
	return 0, fmt.Errorf("unknown mode %q", s)
}

// Query searches one dictionary the way the fan-out searches each of its
// members - the same reading for the mode, the same capability rules - so a
// command-line lookup and the app cannot answer the same question differently.
func Query(ctx context.Context, d dict.Dictionary, mode Mode, term string, limit int) Hit {
	rung, hasRung := rungFor(mode, term)
	return query(ctx, d, mode, term, rung, hasRung, limit)
}

// HasMask reports whether term contains either supported wildcard.
func HasMask(term string) bool { return strings.ContainsAny(term, "?*") }

func maskHeadwords(ctx context.Context, d dict.Dictionary, term string, limit int) ([]dict.Result, error) {
	limit = min(max(limit, 1), 500)
	var out []dict.Result
	for offset := 0; ; offset += 256 {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		words := d.Keywords(offset, 256)
		if len(words) == 0 {
			break
		}
		for _, word := range words {
			matched, err := MatchMask(term, word)
			if err != nil {
				return nil, err
			}
			if !matched {
				continue
			}
			rows, err := d.Exact(word, 1)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
			if len(out) >= limit {
				return out, nil
			}
		}
		if len(words) < 256 {
			break
		}
	}
	return out, nil
}

// CompileMask builds a case-insensitive whole-headword matcher; ? consumes one
// rune and * consumes any number of runes.
func CompileMask(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, r := range pattern {
		switch r {
		case '?':
			b.WriteString(".")
		case '*':
			b.WriteString(".*")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// NormalizeSimpleMask reduces masks made only of '*' and '?' to a minimum
// headword length. Repeated stars carry no additional meaning.
func NormalizeSimpleMask(pattern string) string {
	questions := 0
	for _, r := range pattern {
		switch r {
		case '*':
		case '?':
			questions++
		default:
			return pattern
		}
	}
	if questions == 0 {
		return "*"
	}
	return "*" + strings.Repeat("?", questions) + "*"
}

// IsSimpleMask reports whether a mask contains only '*' and '?'.
func IsSimpleMask(pattern string) bool {
	if pattern == "" {
		return false
	}
	for _, r := range pattern {
		if r != '*' && r != '?' {
			return false
		}
	}
	return true
}

// MatchMask reports whether a headword matches the mask.
func MatchMask(pattern, word string) (bool, error) {
	re, err := CompileMask(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(word), nil
}

// Hit is the result of one dictionary. Order of hits mirrors the input
// dictionary order regardless of completion order.
type Hit struct {
	Meta    dict.Meta
	Results []dict.Result
	Err     error
	Skipped bool // dictionary does not support the requested mode

	// Term is what was actually searched for. It is normally the caller's
	// term, and differs only on the lemma wave, where the caller needs to know
	// which word these results answer in order to mark them.
	Term string

	// Rung names which reading of a full-text query produced these results -
	// "phrase", "near" or "words" (internal/ftsq). Empty for every other mode
	// and for a backend that has only one reading. The relaxation is PER
	// DICTIONARY: one may answer the phrase while the next needs the words, so
	// this belongs on the hit and not on the request.
	Rung string

	// OneReading says the dictionary has exactly one reading of a full-text
	// query and never tried another, so the rung above is not a fallback and
	// must not be reported as one. A source that answers full text itself - a
	// Kiwix archive with its own index - is the case: it matches words, has no
	// phrase query to fail at, and telling the reader "phrase not found" about
	// a phrase nobody looked for is simply untrue.
	OneReading bool
}

// workers bounds how many dictionaries are queried at once. Eight is right for
// a desktop: the queries are I/O-bound, so oversubscribing the cores hides seek
// latency and costs nothing that matters there.
//
// It is wrong for a phone, where eight concurrent SQLite readers means eight
// page caches, eight threads the scheduler must place, and a burst of parallel
// I/O that reads to the platform exactly like a misbehaving app - for a query
// whose answer a human then spends seconds reading. The Android startup path
// sets this to the same number as GOMAXPROCS (D64).
const defaultWorkers = 8

var workers atomic.Int32 // 0 = unset, see Workers

// SetWorkers sizes the fan-out. Values below one are ignored rather than
// clamped silently to a fixed floor, because "no parallelism" is not a
// configuration this code can honour: one worker is the floor.
func SetWorkers(n int) {
	if n < 1 {
		return
	}
	workers.Store(int32(n))
}

// diagnostics gates a per-dictionary fan-out log line. Off by default; enabled
// from Settings → System settings → Log file, so the timing of one search is
// recorded only while someone is measuring it.
var diagnostics atomic.Bool

// SetDiagnostics turns the per-dictionary fan-out log on or off.
func SetDiagnostics(enabled bool) { diagnostics.Store(enabled) }

// Workers reports the current fan-out width, so the other place that touches
// every dictionary at once (the dictionary list) can use the same number.
func Workers() int {
	if n := int(workers.Load()); n > 0 {
		return n
	}
	return defaultWorkers
}

// Opener lazily opens one dictionary. StreamOpen calls it inside the worker
// goroutine so the caller can flush its "begin" line before paying any open
// cost. On open failure the worker emits a Hit with Err set and a zero Meta -
// the caller supplies the dictionary id out of band (by slot index).
type Opener func() (dict.Dictionary, error)

// StreamOpen queries every dictionary concurrently (bounded by Workers) and
// calls emit once per dictionary as its query completes. The open is deferred
// into each worker, so an HTTP handler can emit its slot layout immediately
// (from cheap ids) instead of serializing every cold open before the first
// byte: TTFB becomes one open, not the sum of all of them. emit calls are
// serialized (safe for a shared response) but arrive in completion order; i
// is the input index, so the caller places each result in its slot.
func StreamOpen(ctx context.Context, openers []Opener, mode Mode, term string, perDict int, emit func(i int, h Hit)) {
	StreamOpenWithTerms(ctx, openers, mode, term, perDict, nil, emit)
}

// StreamOpenWithTerms allows a caller to add per-dictionary lemma alternatives
// after the dictionary's language metadata is available in the worker.
func StreamOpenWithTerms(ctx context.Context, openers []Opener, mode Mode, term string, perDict int, termFor func(int, dict.Meta, string) string, emit func(i int, h Hit)) {
	rung, hasRung := rungFor(mode, term)
	sem := make(chan struct{}, Workers())
	var wg sync.WaitGroup
	var mu sync.Mutex
	// emit does the caller's rendering - reference rewriting, highlighting,
	// encoding - and it does it HERE, on the worker's own goroutine. That is
	// the one place a panic is not the HTTP handler's to recover: net/http
	// guards the goroutine it created, this is not that goroutine, and Go's
	// rule that a panic can only be recovered where it runs means the process
	// dies and every other request with it. The same rule query() applies to
	// the parsers one layer down therefore has to hold one layer up: one bad
	// article costs one row.
	//
	// The replacement hit carries no results, so the rendering that failed is
	// not attempted again on the way out; the inner recover is there because a
	// caller whose emit panicked once may panic reporting it, and a panic in a
	// deferred report would defeat the whole guard.
	send := func(i int, h Hit) {
		mu.Lock()
		defer mu.Unlock()
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			defer func() { _ = recover() }()
			emit(i, Hit{Meta: h.Meta, Term: h.Term, Err: dict.PanicError(h.Meta.Path, r)})
		}()
		emit(i, h)
	}
	for i, open := range openers {
		wg.Add(1)
		go func(i int, open Opener) {
			defer wg.Done()
			started := time.Now()
			var dictName, errText string
			defer func() {
				logx.SystemDiagnosticIf(diagnostics.Load(), "search_fanout index=%d dict=%q mode=%s workers=%d elapsed_ms=%d error=%q",
					i, dictName, mode, Workers(), time.Since(started).Milliseconds(), errText)
			}()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errText = ctx.Err().Error()
				send(i, Hit{Err: ctx.Err()})
				return
			}
			// Re-check after the wait. Acquiring the semaphore says only that a
			// slot came free, not that the answer is still wanted: the queue is
			// long (every dictionary at once) and the slots are few, so by the
			// time a worker is admitted the request that asked for it may be
			// minutes dead. The web UI aborts the in-flight fetch on every
			// keystroke (index.html, searchAC), which makes "the caller has
			// gone" the common case rather than the exceptional one.
			//
			// What it costs to miss this is not a wasted CPU slice. open() on a
			// direct backend materialises that dictionary's whole in-memory
			// index - measured on a phone (docs.local/PERF.md §8.7), three abandoned
			// fan-outs over 24 preview dictionaries took 90s each and drove RSS
			// to 1.0 GB, all of it for output nobody would read. Stopping here
			// bounds the damage of a cancelled search to the opens already in
			// flight.
			if err := ctx.Err(); err != nil {
				errText = err.Error()
				send(i, Hit{Err: err})
				return
			}
			d, err := safeOpen(open)
			if err != nil {
				errText = err.Error()
				send(i, Hit{Err: err})
				return
			}
			if diagnostics.Load() {
				dictName = d.Meta().Name
			}
			if err := ctx.Err(); err != nil {
				// Cancelled during the open. The dictionary is now materialised
				// either way; skip only the query, and let the janitor shed it.
				errText = err.Error()
				send(i, Hit{Err: err})
				return
			}
			queryTerm := term
			if termFor != nil {
				var meta dict.Meta
				queryTerm, meta, err = safeTermFor(termFor, i, d, term)
				if err != nil {
					dictName, errText = meta.Name, err.Error()
					send(i, Hit{Meta: meta, Term: term, Err: err})
					return
				}
			}
			hit := runQuery(ctx, d, mode, queryTerm, rung, hasRung, perDict)
			if hit.Meta.Name != "" {
				dictName = hit.Meta.Name
			}
			if hit.Err != nil {
				errText = hit.Err.Error()
			}
			send(i, hit)
		}(i, open)
	}
	wg.Wait()
}

func safeTermFor(resolve func(int, dict.Meta, string) string, i int, d dict.Dictionary, term string) (value string, meta dict.Meta, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = dict.PanicError(meta.Path, r)
		}
	}()
	meta = d.Meta()
	return resolve(i, meta, term), meta, nil
}

// runQuery runs one dictionary's query and waits for it, unless the context
// dies first - then the query is ABANDONED, not waited on.
//
// The context reaches a prepared database's query (dict.Searcher), which SQLite
// then interrupts; it cannot reach a direct backend, which is a plain parser.
// Waiting for a wedged one would hold the whole fan-out - wg.Wait() waits for
// every dictionary, so one crawling search would stall every other result
// behind it. Abandoning bounds the wedge to one goroutine holding one backend,
// which the janitor eventually closes underneath it; the buffered send means
// the abandoned goroutine also ends without blocking whenever its query
// finally does.
func runQuery(ctx context.Context, d dict.Dictionary, mode Mode, term string, rung ftsq.Rung, hasRung bool, perDict int) Hit {
	done := make(chan Hit, 1)
	go func() { done <- query(ctx, d, mode, term, rung, hasRung, perDict) }()
	select {
	case h := <-done:
		return h
	case <-ctx.Done():
		// A finished query outranks the cancellation that landed in the
		// same scheduler tick: the work is done and the answer exists, so
		// a slot reporting "canceled" over a result it is holding would
		// be a lie about the work.
		select {
		case h := <-done:
			return h
		default:
			return Hit{Meta: d.Meta(), Err: ctx.Err()}
		}
	}
}

// safeOpen runs a caller-supplied opener under the same panic conversion as
// the query. dict.Open already recovers internally, but an Opener is a closure
// the server builds (registry lookup, store open, format dispatch) and it runs
// on the worker goroutine, where an escaping panic is fatal to the process.
func safeOpen(open Opener) (d dict.Dictionary, err error) {
	defer func() {
		if r := recover(); r != nil {
			d, err = nil, dict.PanicError("dictionary", r)
		}
	}()
	return open()
}

// planFor parses a full-text query ONCE per fan-out. Parsing it per dictionary
// would be cheap and still wrong: a hundred dictionaries must answer the same
// question, and a query is not a per-dictionary object.
// rungFor is the ONE reading a full-text mode asks for: the phrase, the words
// near one another, or the words anywhere.
//
// The mode IS the promise. A search that quietly answers a weaker question than
// the one on screen is lying about its result, so no mode walks a ladder any
// more: each asks for its own reading and reports which one answered.
//
// The operator language still wins where it is explicit. A quoted query parses
// to a single rung of its own, and a mode whose name is not in that rung set
// uses what the query itself asked for - so typing quotes in "words" still
// searches the phrase, and the header says so.
func rungFor(mode Mode, term string) (ftsq.Rung, bool) {
	var want string
	switch mode {
	case Phrase:
		want = "phrase"
	case Near:
		want = "near"
	case FullText:
		want = "words"
	default:
		return ftsq.Rung{}, false
	}
	rungs := ftsq.Parse(term).Rungs()
	for _, r := range rungs {
		if r.Name == want {
			return r, true
		}
	}
	if len(rungs) > 0 {
		return rungs[0], true
	}
	return ftsq.Rung{}, false
}

// query runs one dictionary's lookup, converting a parser panic into that
// dictionary's error.
//
// The recover is not defensive programming against bugs in this package: it is
// the same policy internal/dict already applies at Open, applied at the other
// end of the same parsers. A corrupt record block reaches a slice expression
// whose bounds came out of the file, and the fan-out runs it in a goroutine
// the HTTP handler did not create - so Go's rule that a panic can only be
// recovered on its own goroutine means the handler's recover cannot see it and
// the process dies. One bad file must cost one row, not the server.
func query(ctx context.Context, d dict.Dictionary, mode Mode, term string, rung ftsq.Rung, hasRung bool, perDict int) (h Hit) {
	defer func() {
		if r := recover(); r != nil {
			h.Err = dict.PanicError(h.Meta.Path, r)
			h.Results = nil
		}
	}()
	h = Hit{Meta: d.Meta(), Term: term}
	caps := d.Caps()
	// A prepared database answers every mode through its context-aware
	// surface; a direct backend has only Exact and Prefix, and no way to stop.
	s, indexed := d.(dict.Searcher)
	headwords, hasHeadwords := d.(interface {
		SearchHeadwordsMode(context.Context, string, string, int) ([]dict.Result, error)
	})
	switch mode {
	case Exact:
		if !caps.Exact {
			h.Skipped = true
			return h
		}
		if indexed {
			h.Results, h.Err = s.ExactContext(ctx, term, perDict)
		} else {
			h.Results, h.Err = d.Exact(term, perDict)
		}
	case Prefix:
		if !caps.Prefix {
			h.Skipped = true
			return h
		}
		if hasHeadwords {
			h.Results, h.Err = headwords.SearchHeadwordsMode(ctx, term, "prefix", perDict)
		} else if indexed {
			h.Results, h.Err = s.PrefixContext(ctx, term, perDict)
		} else {
			h.Results, h.Err = d.Prefix(term, perDict)
		}
	case Contains:
		if hasHeadwords {
			h.Results, h.Err = headwords.SearchHeadwordsMode(ctx, term, "contains", perDict)
			return h
		}
		if !indexed || !caps.Contains {
			h.Skipped = true
			return h
		}
		h.Results, h.Err = s.ContainsContext(ctx, term, perDict)
	case Suffix, WholeWord, WholeWordLemma, ExactLemma:
		if !hasHeadwords {
			h.Skipped = true
			return h
		}
		h.Results, h.Err = headwords.SearchHeadwordsMode(ctx, term, mode.String(), perDict)
	case FullText, Phrase, Near:
		if !indexed || !caps.FTS {
			h.Skipped = true
			return h
		}
		// A source that answers full text itself - a Kiwix archive - has no
		// positions in its index, so the strict readings cannot be ASKED of it.
		// It answers "words" natively, and the two strict ones are verified in
		// the article text instead: the same promise, a slower way to keep it,
		// with a bounded number of candidates checked and the count reported so
		// the reader can see what the reading cost (ArchiveStrict).
		if af, ok := d.(ArchiveStrict); ok && af.FullTextFromArchive() {
			switch mode {
			case Phrase:
				h.Results, h.Err = af.ArchivePhrase(ctx, term, perDict, archiveCheckCandidates(perDict))
				h.Rung = "phrase"
			case Near:
				h.Results, h.Err = af.ArchiveNear(ctx, archiveNearDistance, term, perDict, archiveCheckCandidates(perDict))
				h.Rung = "near"
			default:
				h.Results, h.Err = s.FullTextContext(ctx, term, perDict)
				h.Rung = "words"
			}
			if h.Err != nil {
				h.Rung = ""
			}
			h.OneReading = true
			return h
		}
		if !hasRung {
			// The query has no words at all: nothing to ask for, and asking the
			// database for an empty reading answers with everything.
			h.Skipped = true
			return h
		}
		h.Results, h.Err = s.FullTextMatch(ctx, rung.Match, perDict)
		if h.Err == nil {
			h.Rung = rung.Name
		}
	case Wildcards:
		if indexed {
			if ms, ok := d.(interface {
				MaskContext(context.Context, string, int) ([]dict.Result, error)
			}); ok {
				h.Results, h.Err = ms.MaskContext(ctx, term, perDict)
			} else {
				h.Err = dict.ErrUnsupported
			}
		} else {
			h.Results, h.Err = maskHeadwords(ctx, d, term, perDict)
		}
	}
	return h
}

// archiveCheckCandidates is how many of the archive's ranked candidates the
// strict readings are allowed to verify for one page.
//
// Verifying a phrase means reading articles, so a query with a million
// candidates cannot be answered this way in one page's time; the reading is a
// promise about what was checked, not about everything the index holds. Twenty
// pages' worth, with a floor, covers a phrase that exists in a handful of
// articles - which is the case the strict modes are for.
func archiveCheckCandidates(perDict int) int {
	n := perDict * 20
	if n < 200 {
		n = 200
	}
	return n
}

// archiveNearDistance is how many other words may sit between the query's words
// in the "near" reading, for both kinds of dictionary: it is the FTS5 default
// (NEAR(...) with no distance is 10) and the number the mode's own label
// states, so the two answers agree by construction rather than by luck.
const archiveNearDistance = 10
