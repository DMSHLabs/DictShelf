// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/search"
)

const (
	articleFindNavigationMaxSessions     = 8
	articleFindNavigationFallbackResults = 500
	articleFindNavigationMaxAge          = 30 * time.Minute
)

type articleFindNavigationTarget struct {
	Dictionary      string                  `json:"dict"`
	DictionaryIndex int                     `json:"dictionaryIndex"`
	BlockStart      int                     `json:"blockStart"`
	ArticleIndex    int                     `json:"articleIndex"`
	MatchIndex      int                     `json:"matchIndex"`
	Group           int                     `json:"group,omitempty"`
	Start           int                     `json:"start,omitempty"`
	End             int                     `json:"end,omitempty"`
	Occurrences     []articleFindOccurrence `json:"occurrences,omitempty"`
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
	Occurrences [][]articleFindOccurrence
	HasNext     bool
	Complete    bool
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
	rungs        []string
	dictionaries []string
	pageSize     int
	firstSize    int
	current      *articleFindNavigationTarget
	pages        []map[int]articleFindNavigationPage
	lemmas       map[string]string
	complete     bool
	updated      time.Time
}

func (s *Server) handleArticleFindNavigation(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
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
		session = &articleFindNavigationSession{id: id, query: query, mode: mode, resultQuery: resultQuery, resultMode: resultMode,
			ignore: q.Get("ignoreDiacritics") == "1", matchCase: q.Get("matchCase") == "1", examples: q.Get("examples") == "1", nearWords: nearWords,
			cacheID: q.Get("groupID"), rungs: strings.Split(q.Get("rungs"), ","),
			dictionaries: dictionaries, pageSize: pageSize, firstSize: firstSize,
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
			article, articleErr := navigationNumber(q.Get("anchorArticle"), 0, int(^uint(0)>>1))
			group, groupErr := navigationNumber(q.Get("anchorGroup"), 0, int(^uint(0)>>1))
			offset, offsetErr := navigationNumber(q.Get("anchorOffset"), 0, int(^uint(0)>>1))
			direction, directionErr := strconv.Atoi(q.Get("anchorDirection"))
			if articleErr != nil || groupErr != nil || offsetErr != nil || directionErr != nil || direction != 1 && direction != -1 {
				httpErr(w, http.StatusBadRequest, "invalid article find anchor")
				return
			}
			response, err = s.startArticleFindNavigationFromAnchor(r.Context(), session, q.Get("anchorDict"), article, group, offset, direction)
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
	case "navNext", "navPrevious", "navGoto":
		s.articleFindNavigationMu.Lock()
		session = s.articleFindNavigationSessions[id]
		s.articleFindNavigationMu.Unlock()
		if session == nil {
			httpErr(w, http.StatusGone, "article find session expired")
			return
		}
		var response articleFindNavigationResponse
		var err error
		if q.Get("cacheAction") == "navGoto" {
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
	target, wrapped, err := session.move(ctx, direction, load)
	if err != nil {
		return articleFindNavigationResponse{}, err
	}
	if target == nil {
		return articleFindNavigationResponse{}, nil
	}
	session.current = target
	if wrapped && direction > 0 {
		session.complete = true
	}
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

func (s *Server) startArticleFindNavigationFromAnchor(ctx context.Context, session *articleFindNavigationSession, dictionary string, article, group, offset, direction int) (articleFindNavigationResponse, error) {
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
	cursorMatch := -1
	if direction > 0 {
		for i, match := range occurrences {
			if match.Group < group || match.Group == group && match.Start < offset {
				cursorMatch = i
			}
		}
	} else {
		cursorMatch = len(occurrences)
		for i, match := range occurrences {
			if match.Group > group || match.Group == group && match.Start >= offset {
				cursorMatch = i
				break
			}
		}
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
	session.updated = time.Now()
	position, total := session.position(target)
	return articleFindNavigationResponse{Found: true, Target: target, Position: position, Total: total, Complete: session.complete}, nil
}

func (s *Server) loadArticleFindNavigationPage(ctx context.Context, session *articleFindNavigationSession, dictionaryIndex, start int) (articleFindNavigationPage, error) {
	if dictionaryIndex < 0 || dictionaryIndex >= len(session.dictionaries) || start < 0 {
		return articleFindNavigationPage{}, errors.New("invalid article result block")
	}
	if page, ok := session.pages[dictionaryIndex][start]; ok {
		return page, nil
	}
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
	cacheComplete, cacheHasNext := false, false
	findFullText := sessionUsesFullTextMarks(session)
	request := SearchRequest{Query: session.resultQuery, Mode: session.resultMode, Dictionary: session.dictionaries[dictionaryIndex],
		Format: "raw", Limit: limit, Highlight: findFullText, IgnoreDiacritics: session.ignore}
	if session.cacheID != "" {
		request.CacheAction = "page"
		request.CacheID = session.cacheID
		request.CacheCount = len(session.dictionaries)
		request.CacheOrder = dictionaryIndex
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
			cacheComplete, cacheHasNext = event.Complete, event.HasNext
		}
		return nil
	}); err != nil {
		return articleFindNavigationPage{}, err
	}
	resultCount := len(results)
	if session.cacheID == "" {
		end = min(end, len(results))
		if start >= end {
			return articleFindNavigationPage{Start: start, Complete: true}, nil
		}
		results = results[start:end]
	}
	counts := make([]int, len(results))
	occurrences := make([][]articleFindOccurrence, len(counts))
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
		if findFullText {
			counts[i] = strings.Count(results[i].Body, `<mark class="wu-hl">`)
			continue
		}
		occurrences[i] = matchArticleFindHTML(results[i].Body, options)
		counts[i] = len(occurrences[i])
	}
	page := articleFindNavigationPage{Start: start, Counts: counts, Occurrences: occurrences,
		HasNext:  session.cacheID != "" && cacheHasNext || session.cacheID == "" && end < articleFindNavigationFallbackResults && resultCount > end,
		Complete: session.cacheID != "" && cacheComplete || session.cacheID == "" && resultCount <= end}
	session.pages[dictionaryIndex][start] = page
	return page, nil
}

type articleFindNavigationPageLoader func(context.Context, int, int) (articleFindNavigationPage, error)

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
	start := 0
	for {
		page, err := load(ctx, dictionaryIndex, start)
		if err != nil || !page.HasNext {
			return start, err
		}
		start = session.nextBlockStart(start)
	}
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
	if !sessionUsesFullTextMarks(session) && dictionaryIndex >= 0 && dictionaryIndex < len(session.pages) {
		if page, ok := session.pages[dictionaryIndex][blockStart]; ok {
			row := articleIndex - blockStart
			if row >= 0 && row < len(page.Occurrences) && matchIndex >= 0 && matchIndex < len(page.Occurrences[row]) {
				match := page.Occurrences[row][matchIndex]
				target.Group, target.Start, target.End = match.Group, match.Start, match.End
				target.Occurrences = append([]articleFindOccurrence(nil), page.Occurrences[row]...)
			}
		}
	}
	return target
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
