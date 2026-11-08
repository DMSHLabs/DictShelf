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
)

const (
	articleFindNavigationMaxSessions = 8
	articleFindNavigationMaxResults  = 500
	articleFindNavigationMaxAge      = 30 * time.Minute
)

type articleFindNavigationTarget struct {
	Dictionary      string `json:"dict"`
	DictionaryIndex int    `json:"dictionaryIndex"`
	BlockStart      int    `json:"blockStart"`
	ArticleIndex    int    `json:"articleIndex"`
	MatchIndex      int    `json:"matchIndex"`
}

type articleFindNavigationResponse struct {
	Found    bool                         `json:"found"`
	Target   *articleFindNavigationTarget `json:"target,omitempty"`
	Position int                          `json:"position,omitempty"`
	Total    int                          `json:"total,omitempty"`
	Complete bool                         `json:"complete,omitempty"`
}

type articleFindNavigationPage struct {
	Start    int
	Counts   []int
	HasNext  bool
	Complete bool
}

type articleFindNavigationSession struct {
	mu           sync.Mutex
	id           string
	query        string
	mode         string
	dictionaries []string
	pageSize     int
	firstSize    int
	current      *articleFindNavigationTarget
	pages        []map[int]articleFindNavigationPage
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
		if mode != "fts" && mode != "phrase" && mode != "near" {
			httpErr(w, http.StatusBadRequest, "invalid full-text mode")
			return
		}
		query := strings.TrimSpace(q.Get("q"))
		if query == "" || len(query) > 256 {
			httpErr(w, http.StatusBadRequest, "invalid search query")
			return
		}
		session = &articleFindNavigationSession{id: id, query: query, mode: mode,
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
		response, err := s.moveArticleFindNavigation(r.Context(), session, 1)
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
			position, parseErr := navigationNumber(q.Get("position"), 0, articleFindNavigationMaxResults*1000)
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
		for start := 0; start < articleFindNavigationMaxResults; start = session.nextBlockStart(start) {
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
	if start >= articleFindNavigationMaxResults {
		return articleFindNavigationPage{Start: start, Complete: true}, nil
	}
	end := min(start+blockSize, articleFindNavigationMaxResults)
	limit := min(end+1, articleFindNavigationMaxResults+1)
	var results []dict.Result
	request := SearchRequest{Query: session.query, Mode: session.mode, Dictionary: session.dictionaries[dictionaryIndex],
		Format: "raw", Limit: limit, Highlight: true}
	if err := s.Search(ctx, request, func(event SearchEvent) error {
		if event.T == "hit" && event.Dict == request.Dictionary {
			results = event.Results
		}
		return nil
	}); err != nil {
		return articleFindNavigationPage{}, err
	}
	pageEnd := min(end, len(results))
	counts := make([]int, max(0, pageEnd-start))
	for i := start; i < pageEnd; i++ {
		counts[i-start] = strings.Count(results[i].Body, `<mark class="wu-hl">`)
	}
	page := articleFindNavigationPage{Start: start, Counts: counts,
		HasNext:  end < articleFindNavigationMaxResults && len(results) > end,
		Complete: len(results) <= end}
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
	return &articleFindNavigationTarget{Dictionary: session.dictionaries[dictionaryIndex], DictionaryIndex: dictionaryIndex,
		BlockStart: blockStart, ArticleIndex: articleIndex, MatchIndex: matchIndex}
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
