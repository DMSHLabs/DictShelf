// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/search"
)

const (
	articleFindNavigationMaxSessions     = 8
	articleFindNavigationFallbackResults = 500
	articleFindNavigationMaxAge          = 30 * time.Minute
)

type articleFindNavigationTarget struct {
	Dictionary      string                      `json:"dict"`
	DictionaryIndex int                         `json:"dictionaryIndex"`
	BlockStart      int                         `json:"blockStart"`
	ArticleIndex    int                         `json:"articleIndex"`
	MatchIndex      int                         `json:"matchIndex"`
	Group           int                         `json:"group,omitempty"`
	Start           int                         `json:"start,omitempty"`
	End             int                         `json:"end,omitempty"`
	Occurrences     []articleFindWireOccurrence `json:"occurrences,omitempty"`
	Headword        string                      `json:"headword,omitempty"`
	EntryID         int64                       `json:"entryId,omitempty"`
	ExpectedMatches int                         `json:"expectedMatches,omitempty"`
	Reading         string                      `json:"reading,omitempty"`
	// FromEnd is how many matches from the end this target is while the total is
	// still provisional (1 = the last match). Zero means it is known from the
	// start.
	FromEnd int `json:"fromEnd,omitempty"`
}

type articleFindNavigationResponse struct {
	Found    bool                         `json:"found"`
	Target   *articleFindNavigationTarget `json:"target,omitempty"`
	Position int                          `json:"position,omitempty"`
	Total    int                          `json:"total,omitempty"`
	Complete bool                         `json:"complete,omitempty"`
}

type articleFindNavigationPage struct {
	Start       int
	Counts      []int
	Headwords   []string
	EntryIDs    []int64
	Occurrences [][]articleFindWireOccurrence
	HasNext     bool
	Complete    bool
	// Results is how many articles this dictionary's answer holds - the number
	// that turns "the last block" into arithmetic instead of a walk through
	// every block.
	Results int
}

type articleFindNavigationSession struct {
	mu           sync.Mutex
	id           string
	query        string
	mode         string
	resultQuery  string
	resultMode   string
	ignore       bool
	matchCase    bool
	examples     bool
	nearWords    int
	cacheID      string
	cacheCount   int
	cacheOrders  []int
	rungs        []string
	dictionaries []string
	pageSize     int
	firstSize    int
	// origin is the page's own origin, as the reader's requests spell it. The
	// article page cache keys on it, and this session loads the same blocks the
	// reader pages through: without it every block was rendered twice, once
	// under the bridge's virtual origin and once under an empty one.
	origin   string
	current  *articleFindNavigationTarget
	fromEnd  int
	pages    []map[int]articleFindNavigationPage
	lemmas   map[string]string
	complete bool
	capped   bool // an article hit the mark cap: the total is a lower bound
	updated  time.Time
}

func (s *Server) handleArticleFindNavigation(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	action := q.Get("cacheAction")
	started := time.Now()
	diagnostics := logx.SystemLogSearchEventsEnabled()
	if diagnostics {
		logx.SystemDiagnostic("article_find_navigation stage=request action=%q id=%q query=%q mode=%q dictionaries=%d",
			action, q.Get("cacheID"), q.Get("q"), q.Get("mode"), len(strings.Split(q.Get("dict"), ",")))
		defer func() {
			logx.SystemDiagnostic("article_find_navigation stage=response action=%q id=%q elapsed_ms=%d",
				action, q.Get("cacheID"), time.Since(started).Milliseconds())
		}()
	}
	id := q.Get("cacheID")
	if !wildcardCacheIDPattern.MatchString(id) {
		httpErr(w, http.StatusBadRequest, "invalid article find session")
		return
	}
	var session *articleFindNavigationSession
	switch q.Get("cacheAction") {
	case "navStart":
		dictionaries := strings.Split(q.Get("dict"), ",")
		if len(dictionaries) == 0 || len(dictionaries) > 1000 {
			httpErr(w, http.StatusBadRequest, "invalid dictionary order")
			return
		}
		for i := range dictionaries {
			dictionaries[i] = strings.TrimSpace(dictionaries[i])
			if dictionaries[i] == "" {
				httpErr(w, http.StatusBadRequest, "invalid dictionary order")
				return
			}
		}
		cacheCount, err := navigationNumber(q.Get("cacheCount"), len(dictionaries), 1000)
		if err != nil || cacheCount < len(dictionaries) {
			httpErr(w, http.StatusBadRequest, "invalid dictionary cache count")
			return
		}
		cacheOrders := make([]int, len(dictionaries))
		for i := range cacheOrders {
			cacheOrders[i] = i
		}
		if raw := q.Get("cacheOrders"); raw != "" {
			parts := strings.Split(raw, ",")
			if len(parts) != len(dictionaries) {
				httpErr(w, http.StatusBadRequest, "invalid dictionary cache order")
				return
			}
			for i, part := range parts {
				order, parseErr := strconv.Atoi(part)
				if parseErr != nil || order < 0 || order >= cacheCount {
					httpErr(w, http.StatusBadRequest, "invalid dictionary cache order")
					return
				}
				cacheOrders[i] = order
			}
		}
		pageSize, err := navigationNumber(q.Get("pageSize"), 20, 500)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%v", err)
			return
		}
		firstSize, err := navigationNumber(q.Get("firstSize"), pageSize, 500)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%v", err)
			return
		}
		mode := q.Get("mode")
		if !validArticleFindMode(mode) {
			httpErr(w, http.StatusBadRequest, "invalid article search mode")
			return
		}
		query := strings.TrimSpace(q.Get("q"))
		if query == "" || len(query) > 256 {
			httpErr(w, http.StatusBadRequest, "invalid search query")
			return
		}
		resultQuery := strings.TrimSpace(q.Get("resultQuery"))
		if resultQuery == "" {
			resultQuery = query
		}
		if len(resultQuery) > 256 {
			httpErr(w, http.StatusBadRequest, "invalid source search query")
			return
		}
		resultMode := q.Get("resultMode")
		if resultMode == "" {
			resultMode = mode
		}
		if _, err := search.ParseMode(resultMode); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid source search mode")
			return
		}
		nearWords, err := navigationNumber(q.Get("nearWords"), 10, 50)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%v", err)
			return
		}
		// The page requests keyed by this session carry the origin the server
		// sees on them; a request that has one of its own is the authority, and
		// the bridge (which has none) supplies it in the query.
		origin := navigationOrigin(q.Get("origin"))
		if own := navigationOrigin(originOf(r)); own != "" {
			origin = own
		}
		session = &articleFindNavigationSession{id: id, query: query, mode: mode, resultQuery: resultQuery, resultMode: resultMode,
			ignore: q.Get("ignoreDiacritics") == "1", matchCase: q.Get("matchCase") == "1", examples: q.Get("examples") == "1", nearWords: nearWords,
			cacheID: q.Get("groupID"), cacheCount: cacheCount, cacheOrders: cacheOrders, rungs: strings.Split(q.Get("rungs"), ","),
			dictionaries: dictionaries, pageSize: pageSize, firstSize: firstSize, origin: origin,
			pages: make([]map[int]articleFindNavigationPage, len(dictionaries)), updated: time.Now()}
		for i := range session.pages {
			session.pages[i] = make(map[int]articleFindNavigationPage)
		}
		s.articleFindNavigationMu.Lock()
		if s.articleFindNavigationSessions == nil {
			s.articleFindNavigationSessions = make(map[string]*articleFindNavigationSession)
		}
		s.articleFindNavigationSessions[id] = session
		s.pruneArticleFindNavigationLocked(time.Now())
		s.articleFindNavigationMu.Unlock()
		var response articleFindNavigationResponse
		if q.Get("anchorDict") != "" && mode != "fts" {
			article, articleErr := navigationCoordinate(q.Get("anchorArticle"))
			anchorText := q.Get("anchorText")
			direction, directionErr := strconv.Atoi(q.Get("anchorDirection"))
			if articleErr != nil || len(anchorText) > 512 || directionErr != nil || direction != 1 && direction != -1 {
				httpErr(w, http.StatusBadRequest, "invalid article find anchor")
				return
			}
			response, err = s.startArticleFindNavigationFromAnchor(r.Context(), session, q.Get("anchorDict"), article, anchorText, direction)
		} else if q.Get("prime") == "0" {
			response, err = s.peekArticleFindNavigation(r.Context(), session)
		} else {
			response, err = s.moveArticleFindNavigation(r.Context(), session, 1)
		}
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%v", err)
			return
		}
		writeArticleFindNavigation(w, response)
	case "navNext", "navPrevious", "navGoto", "navFirst", "navLast":
		s.articleFindNavigationMu.Lock()
		session = s.articleFindNavigationSessions[id]
		s.articleFindNavigationMu.Unlock()
		if session == nil {
			httpErr(w, http.StatusGone, "article find session expired")
			return
		}
		var response articleFindNavigationResponse
		var err error
		if action := q.Get("cacheAction"); action == "navFirst" || action == "navLast" {
			direction := 1
			if action == "navLast" {
				direction = -1
			}
			response, err = s.edgeArticleFindNavigation(r.Context(), session, direction)
		} else if q.Get("cacheAction") == "navGoto" {
			position, parseErr := navigationNumber(q.Get("position"), 0, int(^uint(0)>>1))
			if parseErr != nil {
				httpErr(w, http.StatusBadRequest, "%v", parseErr)
				return
			}
			response, err = s.gotoArticleFindNavigation(r.Context(), session, position)
		} else {
			direction := 1
			if q.Get("cacheAction") == "navPrevious" {
				direction = -1
			}
			response, err = s.moveArticleFindNavigation(r.Context(), session, direction)
		}
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "%v", err)
			return
		}
		writeArticleFindNavigation(w, response)
	case "navClose":
		s.articleFindNavigationMu.Lock()
		delete(s.articleFindNavigationSessions, id)
		s.articleFindNavigationMu.Unlock()
		writeArticleFindNavigation(w, articleFindNavigationResponse{})
	default:
		httpErr(w, http.StatusBadRequest, "invalid article find action")
	}
}

func (s *Server) gotoArticleFindNavigation(ctx context.Context, session *articleFindNavigationSession, position int) (articleFindNavigationResponse, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if position < 1 || len(session.dictionaries) == 0 || len(session.pages) != len(session.dictionaries) {
		return articleFindNavigationResponse{}, errors.New("invalid article find position")
	}
	seen := 0
	for dictionaryIndex := range session.dictionaries {
		for start := 0; ; start = session.nextBlockStart(start) {
			page, err := s.loadArticleFindNavigationPage(ctx, session, dictionaryIndex, start)
			if err != nil {
				return articleFindNavigationResponse{}, err
			}
			for offset, count := range page.Counts {
				if position <= seen+count {
					target := session.target(dictionaryIndex, start, start+offset, position-seen-1)
					session.current = target
					session.fromEnd = 0
					target.FromEnd = 0
					session.updated = time.Now()
					current, total := session.position(target)
					return articleFindNavigationResponse{Found: true, Target: target, Position: current, Total: total, Complete: session.complete}, nil
				}
				seen += count
			}
			if !page.HasNext {
				break
			}
		}
	}
	session.complete = true
	session.updated = time.Now()
	return articleFindNavigationResponse{Total: seen, Complete: true}, nil
}

func navigationNumber(value string, fallback, maxValue int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > maxValue {
		return 0, errors.New("invalid article navigation number")
	}
	return n, nil
}

// navigationOrigin accepts the spelling a page uses for itself - scheme://host,
// nothing after the host - and returns "" for anything else, so a malformed or
// hostile value degrades to "no shared page cache" instead of a new key.
func navigationOrigin(raw string) string {
	if raw == "" || len(raw) > 300 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" ||
		(u.Scheme != "http" && u.Scheme != "https") ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func navigationCoordinate(value string) (int, error) {
	if value == "" {
		return 0, errors.New("missing article navigation coordinate")
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, errors.New("invalid article navigation coordinate")
	}
	return n, nil
}

func writeArticleFindNavigation(w http.ResponseWriter, response articleFindNavigationResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) pruneArticleFindNavigationLocked(now time.Time) {
	for id, session := range s.articleFindNavigationSessions {
		session.mu.Lock()
		updated := session.updated
		session.mu.Unlock()
		if now.Sub(updated) > articleFindNavigationMaxAge {
			delete(s.articleFindNavigationSessions, id)
		}
	}
	for len(s.articleFindNavigationSessions) > articleFindNavigationMaxSessions {
		oldestID := ""
		var oldest time.Time
		for id, session := range s.articleFindNavigationSessions {
			session.mu.Lock()
			updated := session.updated
			session.mu.Unlock()
			if oldestID == "" || updated.Before(oldest) {
				oldestID, oldest = id, updated
			}
		}
		delete(s.articleFindNavigationSessions, oldestID)
	}
}

func (s *Server) moveArticleFindNavigation(ctx context.Context, session *articleFindNavigationSession, direction int) (articleFindNavigationResponse, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.dictionaries) == 0 || len(session.pages) != len(session.dictionaries) {
		return articleFindNavigationResponse{}, errors.New("article find session is unavailable")
	}
	load := func(_ context.Context, dictionaryIndex, blockStart int) (articleFindNavigationPage, error) {
		return s.loadArticleFindNavigationPage(ctx, session, dictionaryIndex, blockStart)
	}
	target, _, err := session.advance(ctx, direction, load)
	if err != nil {
		return articleFindNavigationResponse{}, err
	}
	if target == nil {
		return session.endOfWalk(direction), nil
	}
	session.current = target
	if session.complete {
		session.fromEnd = 0
	} else if session.fromEnd > 0 {
		if direction < 0 {
			session.fromEnd++
		} else if session.fromEnd > 1 {
			session.fromEnd--
		} else {
			session.fromEnd = 0
		}
	}
	target.FromEnd = session.fromEnd
	session.updated = time.Now()
	position, total := session.position(target)
	return articleFindNavigationResponse{Found: true, Target: target, Position: position, Total: total, Complete: session.complete}, nil
}

func (s *Server) peekArticleFindNavigation(ctx context.Context, session *articleFindNavigationSession) (articleFindNavigationResponse, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	load := func(_ context.Context, dictionaryIndex, blockStart int) (articleFindNavigationPage, error) {
		return s.loadArticleFindNavigationPage(ctx, session, dictionaryIndex, blockStart)
	}
	target, _, err := session.move(ctx, 1, load)
	if err != nil {
		return articleFindNavigationResponse{}, err
	}
	session.updated = time.Now()
	if target == nil {
		session.complete = true
		return articleFindNavigationResponse{Complete: true, Total: session.total()}, nil
	}
	position, total := session.position(target)
	return articleFindNavigationResponse{Found: true, Target: target, Position: position, Total: total, Complete: session.complete}, nil
}

func (s *Server) startArticleFindNavigationFromAnchor(ctx context.Context, session *articleFindNavigationSession, dictionary string, article int, anchorText string, direction int) (articleFindNavigationResponse, error) {
	dictionaryIndex := -1
	for i, id := range session.dictionaries {
		if id == dictionary {
			dictionaryIndex = i
			break
		}
	}
	if dictionaryIndex < 0 {
		return articleFindNavigationResponse{}, errors.New("article find anchor dictionary is not in the result set")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	blockStart := session.blockStart(article)
	page, err := s.loadArticleFindNavigationPage(ctx, session, dictionaryIndex, blockStart)
	if err != nil {
		return articleFindNavigationResponse{}, err
	}
	row := article - blockStart
	if row < 0 || row >= len(page.Occurrences) {
		return articleFindNavigationResponse{}, errors.New("article find anchor is outside the result set")
	}
	occurrences := page.Occurrences[row]
	// The reader's position travels as a SNIPPET of the article's text, taken
	// from the selection. The server finds it in its own model of the same
	// article, which is exact even when the page's tree and the server's differ
	// (a frame's parser moves head content, and every top-level index shifts with
	// it) - a position the page can name without sharing our text model.
	path, offset := s.articleFindAnchorPosition(ctx, session, dictionaryIndex, page.EntryIDs[row], anchorText)
	cursorMatch := -1
	if path != nil {
		if direction > 0 {
			for i, match := range occurrences {
				if articleFindBefore(match, path, offset) {
					cursorMatch = i
				}
			}
		} else {
			cursorMatch = len(occurrences)
			for i, match := range occurrences {
				if !articleFindBefore(match, path, offset) {
					cursorMatch = i
					break
				}
			}
		}
	} else if direction < 0 {
		// The snippet is not in the text the search reads (a selection inside a
		// dropped example, or an empty one): the walk starts at the article's end.
		cursorMatch = len(occurrences)
	}
	session.current = session.target(dictionaryIndex, blockStart, article, cursorMatch)
	load := func(_ context.Context, index, start int) (articleFindNavigationPage, error) {
		return s.loadArticleFindNavigationPage(ctx, session, index, start)
	}
	target, wrapped, err := session.move(ctx, direction, load)
	if err != nil {
		return articleFindNavigationResponse{}, err
	}
	if target == nil {
		session.complete = true
		return articleFindNavigationResponse{Complete: true, Total: session.total()}, nil
	}
	session.current = target
	if wrapped && direction > 0 {
		session.complete = true
	}
	if session.complete {
		session.fromEnd = 0
	} else if session.fromEnd > 0 {
		if direction < 0 {
			session.fromEnd++
		} else if session.fromEnd > 1 {
			session.fromEnd--
		} else {
			session.fromEnd = 0
		}
	}
	target.FromEnd = session.fromEnd
	session.updated = time.Now()
	position, total := session.position(target)
	return articleFindNavigationResponse{Found: true, Target: target, Position: position, Total: total, Complete: session.complete}, nil
}

func (s *Server) loadArticleFindNavigationPage(ctx context.Context, session *articleFindNavigationSession, dictionaryIndex, start int) (articleFindNavigationPage, error) {
	if dictionaryIndex < 0 || dictionaryIndex >= len(session.dictionaries) || start < 0 {
		return articleFindNavigationPage{}, errors.New("invalid article result block")
	}
	if page, ok := session.pages[dictionaryIndex][start]; ok {
		logx.SystemDiagnosticIf(logx.SystemLogSearchEventsEnabled(),
			"article_find_navigation stage=page_cache_hit id=%q dict=%q start=%d", session.id, session.dictionaries[dictionaryIndex], start)
		return page, nil
	}
	pageStarted := time.Now()
	blockSize := session.pageSize
	if start == 0 {
		blockSize = session.firstSize
	}
	if session.cacheID == "" && start >= articleFindNavigationFallbackResults {
		return articleFindNavigationPage{Start: start, Complete: true}, nil
	}
	end := start + blockSize
	limit := min(end+1, articleFindNavigationFallbackResults+1)
	var results []dict.Result
	cacheComplete, cacheHasNext, resultTotal := false, false, 0
	findFullText := sessionUsesFullTextMarks(session)
	request := SearchRequest{Query: session.resultQuery, Mode: session.resultMode, Dictionary: session.dictionaries[dictionaryIndex],
		Format: "raw", Limit: limit, Highlight: findFullText, IgnoreDiacritics: session.ignore, Transport: "article_find_navigation",
		Origin: session.origin}
	if session.cacheID != "" {
		request.CacheAction = "page"
		request.CacheID = session.cacheID
		request.CacheCount = max(len(session.dictionaries), session.cacheCount)
		request.CacheOrder = dictionaryIndex
		if dictionaryIndex < len(session.cacheOrders) {
			request.CacheOrder = session.cacheOrders[dictionaryIndex]
		}
		request.Start = start
		request.Size = blockSize
		if dictionaryIndex < len(session.rungs) {
			request.Rung = session.rungs[dictionaryIndex]
		}
	} else {
		request.Limit = limit
	}
	if err := s.Search(ctx, request, func(event SearchEvent) error {
		if event.T == "hit" && event.Dict == request.Dictionary {
			results = event.Results
			cacheComplete, cacheHasNext, resultTotal = event.Complete, event.HasNext, event.Total
		}
		return nil
	}); err != nil {
		logx.SystemDiagnosticIf(logx.SystemLogSearchEventsEnabled(),
			"article_find_navigation stage=page_read_failed id=%q dict=%q start=%d elapsed_ms=%d error=%q",
			session.id, request.Dictionary, start, time.Since(pageStarted).Milliseconds(), err)
		return articleFindNavigationPage{}, err
	}
	resultsReady := time.Now()
	logx.SystemDiagnosticIf(logx.SystemLogSearchEventsEnabled(),
		"article_find_navigation stage=page_articles_ready id=%q dict=%q start=%d articles=%d elapsed_ms=%d",
		session.id, request.Dictionary, start, len(results), time.Since(pageStarted).Milliseconds())
	resultCount := len(results)
	if session.cacheID == "" {
		end = min(end, len(results))
		if start >= end {
			return articleFindNavigationPage{Start: start, Complete: true}, nil
		}
		results = results[start:end]
	}
	counts := make([]int, len(results))
	headwords := make([]string, len(results))
	entryIDs := make([]int64, len(results))
	occurrences := make([][]articleFindWireOccurrence, len(counts))
	options := articleFindMatchOptions{query: session.query, mode: session.mode, matchCase: session.matchCase,
		examples: session.examples, nearWords: session.nearWords}
	if session.mode == "wordLemma" && !strings.ContainsAny(session.query, " \t\r\n") && s.Morph.Enabled() {
		if session.lemmas == nil {
			session.lemmas = make(map[string]string)
		}
		id := request.Dictionary
		lemma, ok := session.lemmas[id]
		if !ok {
			lemma = session.query
			if entry, err := s.reg.get(id); err == nil {
				if source, openErr := entry.open(); openErr == nil {
					if candidate := s.headwordLemmaFor(entry, source, session.query); candidate != "" {
						lemma = candidate
					}
				}
			}
			session.lemmas[id] = lemma
		}
		options.variants = []string{session.query, lemma}
	}
	for i := range results {
		headwords[i], entryIDs[i] = results[i].Headword, results[i].EntryID
		if findFullText {
			// A reading's answer already carries its marks, and one head is one
			// occurrence: the number the arrows walk IS the number painted into
			// the article, so the two cannot disagree.
			var capped bool
			counts[i], capped = countArticleFindHeads(results[i].Body)
			if capped {
				session.markCapped(request.Dictionary, results[i].EntryID, session.mode)
			}
			continue
		}
		groups, matches := articleFindHTMLSpans(results[i].Body, options)
		if len(matches) > indexconst.ArticleMarkLimit {
			// The literal list is bounded the way the marks are: an article that
			// matches a short needle thousands of times would otherwise travel
			// whole and be painted whole, and no reader steps through that.
			matches = matches[:indexconst.ArticleMarkLimit]
			session.markCapped(request.Dictionary, results[i].EntryID, session.mode)
		}
		occurrences[i] = articleFindWireOccurrences(groups, matches)
		counts[i] = len(occurrences[i])
	}
	matches := 0
	for _, count := range counts {
		matches += count
	}
	logx.SystemDiagnosticIf(logx.SystemLogSearchEventsEnabled(),
		"article_find_navigation stage=page_scan_done id=%q dict=%q start=%d articles=%d matches=%d elapsed_ms=%d",
		session.id, request.Dictionary, start, len(results), matches, time.Since(resultsReady).Milliseconds())
	page := articleFindNavigationPage{Start: start, Results: resultTotal, Counts: counts, Headwords: headwords, EntryIDs: entryIDs, Occurrences: occurrences,
		HasNext:  session.cacheID != "" && cacheHasNext || session.cacheID == "" && end < articleFindNavigationFallbackResults && resultCount > end,
		Complete: session.cacheID != "" && cacheComplete || session.cacheID == "" && resultCount <= end}
	session.pages[dictionaryIndex][start] = page
	return page, nil
}

type articleFindNavigationPageLoader func(context.Context, int, int) (articleFindNavigationPage, error)

// advance is one step of the reader's arrow. The walk wraps at the ends - a
// diagnostic counts on it, and so does the anchor start - but the arrow does
// not: stepping past the last match carries nobody back to the first. It only
// says the answer is complete, and the cursor stays where the reader was.
func (session *articleFindNavigationSession) advance(ctx context.Context, direction int, load articleFindNavigationPageLoader) (*articleFindNavigationTarget, bool, error) {
	target, wrapped, err := session.move(ctx, direction, load)
	if err != nil || target == nil || !wrapped {
		return target, wrapped, err
	}
	if direction > 0 {
		session.complete = true
	}
	return nil, wrapped, nil
}

// endOfWalk answers an arrow that has nowhere left to go: no target, the end
// known, and the position the reader is standing on so the bar keeps its
// number instead of reading as an answer with no matches.
func (session *articleFindNavigationSession) endOfWalk(direction int) articleFindNavigationResponse {
	if direction > 0 {
		session.complete = true
	}
	if session.complete {
		session.fromEnd = 0
	}
	position, total := 0, session.total()
	if session.current != nil {
		position, total = session.position(session.current)
	}
	session.updated = time.Now()
	return articleFindNavigationResponse{Complete: session.complete, Position: position, Total: total}
}

func (session *articleFindNavigationSession) move(ctx context.Context, direction int, load articleFindNavigationPageLoader) (*articleFindNavigationTarget, bool, error) {
	if direction != 1 && direction != -1 {
		return nil, false, errors.New("invalid article find direction")
	}
	if session.current == nil {
		if direction < 0 {
			target, err := session.findInDictionary(ctx, len(session.dictionaries)-1, direction, -1, -1, load)
			return target, false, err
		}
		for i := range session.dictionaries {
			target, err := session.findInDictionary(ctx, i, 1, 0, -1, load)
			if err != nil || target != nil {
				return target, false, err
			}
		}
		return nil, false, nil
	}
	current := *session.current
	for distance := 0; distance < len(session.dictionaries); distance++ {
		index := (current.DictionaryIndex + direction*distance + len(session.dictionaries)) % len(session.dictionaries)
		firstPass := distance == 0
		startArticle, startMatch := 0, -1
		if firstPass {
			startArticle = current.ArticleIndex
			startMatch = current.MatchIndex
		}
		if direction < 0 && !firstPass {
			var err error
			startArticle, err = session.lastArticleIndex(ctx, index, load)
			if err != nil {
				return nil, false, err
			}
		}
		target, err := session.findInDictionary(ctx, index, direction, startArticle, startMatch, load)
		if err != nil {
			return nil, false, err
		}
		if target != nil {
			wrapped := distance > 0 && (direction > 0 && index < current.DictionaryIndex || direction < 0 && index > current.DictionaryIndex)
			return target, wrapped, nil
		}
	}
	if direction > 0 {
		target, err := session.findInDictionary(ctx, current.DictionaryIndex, 1, 0, -1, load)
		return target, target != nil, err
	}
	last, err := session.lastArticleIndex(ctx, current.DictionaryIndex, load)
	if err != nil {
		return nil, false, err
	}
	target, err := session.findInDictionary(ctx, current.DictionaryIndex, -1, last, -1, load)
	return target, target != nil, err
}

func (session *articleFindNavigationSession) findInDictionary(ctx context.Context, dictionaryIndex, direction, articleIndex, matchIndex int, load articleFindNavigationPageLoader) (*articleFindNavigationTarget, error) {
	if dictionaryIndex < 0 || dictionaryIndex >= len(session.dictionaries) {
		return nil, nil
	}
	if direction > 0 {
		start := session.blockStart(articleIndex)
		for {
			page, err := load(ctx, dictionaryIndex, start)
			if err != nil {
				return nil, err
			}
			for index := max(articleIndex, start); index < start+len(page.Counts); index++ {
				count := page.Counts[index-start]
				firstMatch := 0
				if index == articleIndex && matchIndex >= 0 {
					firstMatch = matchIndex + 1
				}
				if firstMatch < count {
					return session.target(dictionaryIndex, start, index, firstMatch), nil
				}
			}
			if !page.HasNext {
				return nil, nil
			}
			start = session.nextBlockStart(start)
			articleIndex = start
			matchIndex = -1
		}
	}
	start := session.blockStart(articleIndex)
	if articleIndex < 0 {
		start, _ = session.lastBlockStart(ctx, dictionaryIndex, load)
		page, err := load(ctx, dictionaryIndex, start)
		if err != nil {
			return nil, err
		}
		articleIndex = start + len(page.Counts) - 1
		matchIndex = -1
	}
	for {
		page, err := load(ctx, dictionaryIndex, start)
		if err != nil {
			return nil, err
		}
		for index := min(articleIndex, start+len(page.Counts)-1); index >= start; index-- {
			count := page.Counts[index-start]
			lastMatch := count - 1
			if index == articleIndex && matchIndex >= 0 {
				lastMatch = matchIndex - 1
			}
			if lastMatch >= 0 {
				return session.target(dictionaryIndex, start, index, lastMatch), nil
			}
		}
		if start == 0 {
			return nil, nil
		}
		start = session.previousBlockStart(start)
		articleIndex = start + session.blockLength(start) - 1
		matchIndex = -1
	}
}

func (session *articleFindNavigationSession) lastArticleIndex(ctx context.Context, dictionaryIndex int, load articleFindNavigationPageLoader) (int, error) {
	start, err := session.lastBlockStart(ctx, dictionaryIndex, load)
	if err != nil {
		return -1, err
	}
	page, err := load(ctx, dictionaryIndex, start)
	if err != nil {
		return -1, err
	}
	return start + len(page.Counts) - 1, nil
}

func (session *articleFindNavigationSession) lastBlockStart(ctx context.Context, dictionaryIndex int, load articleFindNavigationPageLoader) (int, error) {
	// The answer already says how many articles it found: the last of them is
	// in the last block, so no walk through the blocks is needed. The walk below
	// is only for a session that has not seen the answer's count yet.
	if dictionaryIndex < 0 || dictionaryIndex >= len(session.pages) {
		return 0, nil
	}
	if results, ok := session.anyPageWithResults(dictionaryIndex); ok {
		return session.blockStart(results - 1), nil
	}
	page, err := load(ctx, dictionaryIndex, 0)
	if err != nil {
		return 0, err
	}
	if page.Results > 0 {
		return session.blockStart(page.Results - 1), nil
	}
	start := 0
	for {
		page, err := load(ctx, dictionaryIndex, start)
		if err != nil || !page.HasNext {
			return start, err
		}
		start = session.nextBlockStart(start)
	}
}

// anyPageWithResults finds a page already loaded for this dictionary and returns
// how many articles its answer holds.
func (session *articleFindNavigationSession) anyPageWithResults(dictionaryIndex int) (int, bool) {
	for _, page := range session.pages[dictionaryIndex] {
		if page.Results > 0 {
			return page.Results, true
		}
	}
	return 0, false
}

func (session *articleFindNavigationSession) blockStart(articleIndex int) int {
	if articleIndex <= 0 || articleIndex < session.firstSize {
		return 0
	}
	return session.firstSize + ((articleIndex-session.firstSize)/session.pageSize)*session.pageSize
}

func (session *articleFindNavigationSession) nextBlockStart(start int) int {
	if start == 0 {
		return session.firstSize
	}
	return start + session.pageSize
}

func (session *articleFindNavigationSession) previousBlockStart(start int) int {
	if start <= session.firstSize {
		return 0
	}
	return start - session.pageSize
}

func (session *articleFindNavigationSession) blockLength(start int) int {
	if start == 0 {
		return session.firstSize
	}
	return session.pageSize
}

func (session *articleFindNavigationSession) target(dictionaryIndex, blockStart, articleIndex, matchIndex int) *articleFindNavigationTarget {
	target := &articleFindNavigationTarget{Dictionary: session.dictionaries[dictionaryIndex], DictionaryIndex: dictionaryIndex,
		BlockStart: blockStart, ArticleIndex: articleIndex, MatchIndex: matchIndex}
	target.Reading = session.resultMode
	if dictionaryIndex >= 0 && dictionaryIndex < len(session.rungs) && session.rungs[dictionaryIndex] != "" {
		target.Reading = session.rungs[dictionaryIndex]
	}
	if dictionaryIndex >= 0 && dictionaryIndex < len(session.pages) {
		if page, ok := session.pages[dictionaryIndex][blockStart]; ok {
			row := articleIndex - blockStart
			if row >= 0 && row < len(page.Occurrences) && matchIndex >= 0 && matchIndex < len(page.Occurrences[row]) {
				target.Occurrences = append([]articleFindWireOccurrence(nil), page.Occurrences[row]...)
			}
		}
	}
	if dictionaryIndex >= 0 && dictionaryIndex < len(session.pages) {
		if page, ok := session.pages[dictionaryIndex][blockStart]; ok {
			row := articleIndex - blockStart
			if row >= 0 && row < len(page.Counts) {
				target.ExpectedMatches = page.Counts[row]
				if row < len(page.Headwords) {
					target.Headword = page.Headwords[row]
				}
				if row < len(page.EntryIDs) {
					target.EntryID = page.EntryIDs[row]
				}
			}
		}
	}
	return target
}

// sessionUsesFullTextMarks reports whether the session walks a reading's
// marks. The strip sends mode "fts" for any of the three readings and says
// which one ran in resultMode; the dialog never sends it, because its literal
// modes have occurrence lists of their own.
// articleFindAnchorPosition locates the reader's snippet in the server's model of
// one article and returns the position an occurrence can be compared with: the
// child-index path to the text node the snippet starts in, and the UTF-16 offset
// inside it. It returns nil when the snippet is not in the text the search reads.
//
// This re-reads ONE article's body and folds it the way the matcher does; the
// gesture is rare and a page of results is not, so it stays out of the page
// cache rather than keeping every body alive to answer it.
func (s *Server) articleFindAnchorPosition(ctx context.Context, session *articleFindNavigationSession, dictionaryIndex int, entryID int64, anchorText string) ([]int, int) {
	needle := []rune(strings.TrimSpace(anchorText))
	if len(needle) == 0 || entryID == 0 || dictionaryIndex < 0 || dictionaryIndex >= len(session.dictionaries) {
		return nil, 0
	}
	entry, err := s.reg.get(session.dictionaries[dictionaryIndex])
	if err != nil {
		return nil, 0
	}
	d, err := entry.open()
	if err != nil {
		return nil, 0
	}
	source, ok := d.(interface {
		ArticlesByIDs(context.Context, []int64) ([]dict.Result, error)
	})
	if !ok {
		return nil, 0
	}
	results, err := source.ArticlesByIDs(ctx, []int64{entryID})
	if err != nil || len(results) == 0 {
		return nil, 0
	}
	groups := articleFindHTMLGroups(results[0].Body, session.examples)
	for gi := range groups {
		group := &groups[gi]
		at := articleFindRuneIndex(group.text, needle)
		if at < 0 {
			continue
		}
		for _, segment := range group.segments {
			if at < segment.start || at >= segment.end {
				continue
			}
			start, _, ok := articleFindSegmentRunes(group, segment, at, at+1)
			if !ok {
				continue
			}
			return segment.path, articleFindNodeUTF16(segment.node, start)
		}
	}
	return nil, 0
}

// articleFindRuneIndex is the first rune index where needle appears in text.
func articleFindRuneIndex(text, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(text) {
		return -1
	}
	for i := 0; i+len(needle) <= len(text); i++ {
		match := true
		for j := range needle {
			if text[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// articleFindBefore reports whether an occurrence starts before the anchor
// position, in document order: lexicographic on the child-index path, then the
// UTF-16 offset inside the node. A node precedes its descendants, so a path that
// is a prefix sorts first.
func articleFindBefore(occurrence articleFindWireOccurrence, path []int, offset int) bool {
	if len(occurrence.Spans) == 0 {
		return false
	}
	start := occurrence.Spans[0]
	for i := 0; i < len(start.Path) && i < len(path); i++ {
		if start.Path[i] != path[i] {
			return start.Path[i] < path[i]
		}
	}
	if len(start.Path) != len(path) {
		return len(start.Path) < len(path)
	}
	return start.Start < offset
}

// markCapped records that one article's marks were cut at indexconst.ArticleMarkLimit, and
// names the article once in the system log. From then on the session's total is
// a lower bound, and every response says so - the reader must not be told that an
// exact number of matches exists when the tail was never painted.
func (session *articleFindNavigationSession) markCapped(dict string, entryID int64, reading string) {
	if session.capped {
		return
	}
	session.capped = true
	logx.SystemDiagnosticIf(logx.SystemLogSearchEventsEnabled(),
		"article_find stage=capped dict=%q entry_id=%d reading=%q marks=%d", dict, entryID, reading, indexconst.ArticleMarkLimit)
}

// completeAnswer is what a response may claim: the scan finished AND no article
// was cut at the cap. A capped total is a lower bound, and the page's "+" is the
// only honest way to say that.
func (session *articleFindNavigationSession) completeAnswer() bool {
	return session.complete && !session.capped
}

func sessionUsesFullTextMarks(session *articleFindNavigationSession) bool {
	return session.mode == "fts"
}

func validArticleFindMode(mode string) bool {
	switch mode {
	case "fts", "phrase", "near", "prefix", "suffix", "contains", "word", "wordLemma":
		return true
	default:
		return false
	}
}

func (session *articleFindNavigationSession) position(target *articleFindNavigationTarget) (int, int) {
	position, total := 0, 0
	for dictionaryIndex, pages := range session.pages {
		for start, page := range pages {
			for offset, count := range page.Counts {
				article := start + offset
				total += count
				if dictionaryIndex < target.DictionaryIndex || dictionaryIndex == target.DictionaryIndex && article < target.ArticleIndex {
					position += count
				} else if dictionaryIndex == target.DictionaryIndex && article == target.ArticleIndex && start == target.BlockStart {
					position += target.MatchIndex + 1
				}
			}
		}
	}
	return position, total
}

func (session *articleFindNavigationSession) total() int {
	total := 0
	for _, pages := range session.pages {
		for _, page := range pages {
			for _, count := range page.Counts {
				total += count
			}
		}
	}
	return total
}

// edgeArticleFindNavigation answers "go to the first / last match": the edge is
// found from the edge itself - the last dictionary from its last block
// backwards, the first dictionary from block 0 forwards - so no walk through
// the whole selection is needed. The cursor lands on the edge match and, when
// the total is still provisional, says how far from the end it is.
func (s *Server) edgeArticleFindNavigation(ctx context.Context, session *articleFindNavigationSession, direction int) (articleFindNavigationResponse, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.dictionaries) == 0 || len(session.pages) != len(session.dictionaries) {
		return articleFindNavigationResponse{}, errors.New("article find session is unavailable")
	}
	load := func(_ context.Context, dictionaryIndex, blockStart int) (articleFindNavigationPage, error) {
		return s.loadArticleFindNavigationPage(ctx, session, dictionaryIndex, blockStart)
	}
	var target *articleFindNavigationTarget
	var err error
	if direction > 0 {
		for index := 0; index < len(session.dictionaries) && target == nil; index++ {
			target, err = session.findInDictionary(ctx, index, 1, 0, -1, load)
		}
	} else {
		for index := len(session.dictionaries) - 1; index >= 0 && target == nil; index-- {
			var last int
			last, err = session.lastArticleIndex(ctx, index, load)
			if err != nil {
				break
			}
			target, err = session.findInDictionary(ctx, index, -1, last, -1, load)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		return articleFindNavigationResponse{}, err
	}
	if target == nil {
		session.complete = true
		return articleFindNavigationResponse{Complete: true, Total: session.total()}, nil
	}
	session.current = target
	session.updated = time.Now()
	session.fromEnd = 0
	if direction < 0 && !session.complete {
		session.fromEnd = 1
	}
	target.FromEnd = session.fromEnd
	position, total := session.position(target)
	return articleFindNavigationResponse{Found: true, Target: target, Position: position, Total: total, Complete: session.complete}, nil
}
