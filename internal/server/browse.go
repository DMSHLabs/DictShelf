// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/indexconst"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/search"
	"github.com/wuweidict/wudict/internal/store"
)

type browseFindResp struct {
	Words         []string      `json:"words"`
	Alphabet      []dict.Letter `json:"alphabet,omitempty"`
	Positions     []int         `json:"positions,omitempty"`
	BasePositions []int         `json:"basePositions,omitempty"`
	Total         int           `json:"total"`
	Page          int           `json:"page"`
	Pages         int           `json:"pages"`
	Size          int           `json:"size"`
	Partial       bool          `json:"partial,omitempty"`
}

// handleBrowseFind searches the complete headword index, then pages the
// matches. Prepared dictionaries answer from their index; ZIM scans its native
// headword list because its path index has no arbitrary token-search operation.
func (s *Server) handleBrowseFind(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("stream") == "1" {
		s.handleBrowseFindStream(w, r)
		return
	}
	result, err := s.browseFindPage(r.Context(), r.URL.Query())
	if err != nil {
		if r.Context().Err() == nil {
			writeBrowseFindFailure(w, err)
		}
		return
	}
	writeJSON(w, result)
}

func (s *Server) browseFindPage(ctx context.Context, q url.Values) (browseFindResp, error) {
	if q.Get("merged") == "1" {
		return s.mergedBrowseFindPage(ctx, q)
	}
	query := strings.TrimSpace(q.Get("q"))
	if len(query) > 256 {
		return browseFindResp{}, &browseFindSetupError{status: http.StatusBadRequest, message: "search text is too long"}
	}
	mask := q.Get("wildcards") == "1" || q.Get("mask") == "1"
	within := strings.TrimSpace(q.Get("within"))
	if len(within) > 256 {
		return browseFindResp{}, &browseFindSetupError{status: http.StatusBadRequest, message: "filter text is too long"}
	}
	withinMask := q.Get("withinWildcards") == "1" || q.Get("withinMask") == "1"
	page, _ := strconv.Atoi(q.Get("p"))
	if page < 1 {
		page = 1
	}
	// A stale or malformed bookmark cannot drive unbounded OFFSET work.
	if page > 10000000 {
		page = 10000000
	}
	if cacheID := q.Get("wildcache"); cacheID != "" && mask {
		cacheOrder, orderErr := strconv.Atoi(q.Get("wildorder"))
		cacheCount, countErr := strconv.Atoi(q.Get("wildcount"))
		if orderErr == nil && countErr == nil {
			if rawPosition := q.Get("findpos"); rawPosition != "" {
				position, parseErr := strconv.Atoi(rawPosition)
				direction, directionErr := strconv.Atoi(q.Get("finddir"))
				if parseErr != nil || within == "" {
					return browseFindResp{}, &browseFindSetupError{status: http.StatusBadRequest, message: "invalid wildcard match position"}
				}
				if directionErr != nil || direction < -1 || direction > 1 {
					return browseFindResp{}, &browseFindSetupError{status: http.StatusBadRequest, message: "invalid wildcard match direction"}
				}
				matchIndex, found, findErr := s.wildcardPages.findPosition(cacheID, wildcardCacheSignature{
					query: query, group: q.Get("wildgroup"), wildcards: mask,
				}, q.Get("dict"), within, withinMask, position, direction, cacheOrder, cacheCount)
				if findErr != nil {
					return browseFindResp{}, &browseFindSetupError{status: http.StatusInternalServerError, message: "reading cached wildcard match: " + findErr.Error()}
				}
				if !found {
					return browseFindResp{}, &browseFindSetupError{status: http.StatusNotFound, message: "matching headword is not available yet"}
				}
				page = matchIndex/browsePageSize + 1
			}
			cached, ok, err := s.wildcardPages.page(cacheID, wildcardCacheSignature{
				query: query, group: q.Get("wildgroup"), wildcards: mask,
			}, q.Get("dict"), within, withinMask, page, cacheOrder, cacheCount)
			if err != nil {
				return browseFindResp{}, &browseFindSetupError{status: http.StatusInternalServerError, message: "reading cached wildcard block: " + err.Error()}
			}
			if ok && !(mask && within == "" && isAllWildcardMask(query) && cached.Partial) {
				return cached, nil
			}
		}
	}
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		return browseFindResp{}, &browseFindSetupError{status: http.StatusNotFound, message: err.Error()}
	}
	if !hasBrowseIndex(e.Path) {
		return browseFindResp{}, &browseFindSetupError{status: http.StatusConflict, message: "This dictionary has no index. Create one in Dictionary Settings.", entry: e}
	}
	d, err := e.open()
	if err != nil {
		return browseFindResp{}, &browseFindSetupError{status: http.StatusInternalServerError, message: err.Error(), entry: e}
	}
	finder, ok := d.(dict.BrowseFinder)
	browser, browseOK := d.(dict.Browser)
	allWords := mask && within == "" && isAllWildcardMask(query)
	if !ok && !(allWords && browseOK) {
		return browseFindResp{}, &browseFindSetupError{status: http.StatusConflict, message: "This dictionary has no index. Create one in Dictionary Settings.", entry: e}
	}
	find := func(page int) ([]string, int, []dict.Letter, []int, error) {
		if allWords && browseOK {
			letters, err := browser.Alphabet()
			if err != nil {
				return nil, 0, nil, nil, err
			}
			total := 0
			for _, letter := range letters {
				total += letter.Count
			}
			offset := (page - 1) * browsePageSize
			words, err := browser.Page(offset, browsePageSize)
			if err != nil {
				return nil, 0, nil, nil, err
			}
			positions := make([]int, len(words))
			for i := range positions {
				positions[i] = offset + i
			}
			return words, total, letters, positions, nil
		}
		if mask {
			if ok {
				if within != "" {
					if extended, yes := d.(interface {
						FindHeadwordsMaskWithin(context.Context, string, string, bool, int, int) ([]string, int, []dict.Letter, []int, error)
					}); yes {
						return extended.FindHeadwordsMaskWithin(ctx, query, within, withinMask, (page-1)*browsePageSize, browsePageSize)
					}
					return findDirectHeadwords(ctx, d, query, true, within, withinMask, (page-1)*browsePageSize, browsePageSize)
				}
				words, total, letters, err := finder.FindHeadwordsMask(ctx, query, (page-1)*browsePageSize, browsePageSize)
				return words, total, letters, nil, err
			}
			return findDirectHeadwords(ctx, d, query, true, within, withinMask, (page-1)*browsePageSize, browsePageSize)
		}
		if ok {
			words, total, err := finder.FindHeadwords(ctx, query, (page-1)*browsePageSize, browsePageSize)
			return words, total, nil, nil, err
		}
		return findDirectHeadwords(ctx, d, query, false, within, withinMask, (page-1)*browsePageSize, browsePageSize)
	}
	words, total, letters, positions, err := find(page)
	if err != nil {
		if ctx.Err() != nil {
			return browseFindResp{}, ctx.Err()
		}
		logx.System("browse find failed dict=%q mask=%v error=%q", e.ID, mask, err)
		return browseFindResp{}, &browseFindSetupError{status: http.StatusInternalServerError, message: "searching " + e.ID + ": " + err.Error(), entry: e}
	}
	pages := (total + browsePageSize - 1) / browsePageSize
	if pages > 0 && page > pages {
		page = pages
		words, _, letters, positions, err = find(page)
		if err != nil {
			if ctx.Err() != nil {
				return browseFindResp{}, ctx.Err()
			}
			return browseFindResp{}, &browseFindSetupError{status: http.StatusInternalServerError, message: "searching " + e.ID + ": " + err.Error(), entry: e}
		}
	}
	result := browseFindResp{Words: words, Alphabet: letters, Positions: positions, Total: total, Page: page, Pages: pages, Size: browsePageSize}
	if allWords {
		cacheID := q.Get("wildcache")
		cacheOrder, orderErr := strconv.Atoi(q.Get("wildorder"))
		cacheCount, countErr := strconv.Atoi(q.Get("wildcount"))
		if cacheID != "" && q.Get("wildgroup") != "" && orderErr == nil && countErr == nil {
			if err := s.wildcardPages.storePage(cacheID, wildcardCacheSignature{
				query: query, group: q.Get("wildgroup"), wildcards: true,
			}, browsePageSize, cacheCount, e.ID, d.Meta().Name, cacheOrder, page, result, s.wildcardCacheDir()); err != nil {
				logx.System("wildcard cache page save failed dict=%q page=%d error=%q", e.ID, page, err)
			}
		}
	}
	return result, nil
}

func isAllWildcardMask(query string) bool {
	return query != "" && strings.Trim(query, "*") == ""
}

type browseFindStreamEvent struct {
	Type      string        `json:"type"`
	Words     []string      `json:"words,omitempty"`
	Positions []int         `json:"positions,omitempty"`
	Alphabet  []dict.Letter `json:"alphabet,omitempty"`
	Total     int           `json:"total"`
	Scanned   int           `json:"scanned,omitempty"`
	ScanTotal int           `json:"scanTotal,omitempty"`
	Size      int           `json:"size,omitempty"`
	DiskCache bool          `json:"diskCache,omitempty"`
	Done      bool          `json:"done,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type browseHeadwordStreamer interface {
	StreamBrowseHeadwords(context.Context, string, bool, func([]string, int, bool, int, int) error) error
}

type browseFindStreamRequest struct {
	entry      *entry
	dict       dict.Dictionary
	query      string
	mask       bool
	within     string
	withinMask bool
	withinRE   *regexp.Regexp
	cacheID    string
	cacheGroup string
	cacheOrder int
	cacheCount int
	page       int
}

type browseFindSetupError struct {
	status  int
	message string
	entry   *entry
}

func (e *browseFindSetupError) Error() string { return e.message }

func writeBrowseFindFailure(w http.ResponseWriter, err error) {
	var failure *browseFindSetupError
	if errors.As(err, &failure) {
		if failure.status == http.StatusConflict && failure.entry != nil {
			browseUnprepared(w, failure.entry)
			return
		}
		httpErr(w, failure.status, "%s", failure.message)
		return
	}
	httpErr(w, http.StatusInternalServerError, "%s", err.Error())
}

func (s *Server) prepareBrowseFindStream(q url.Values) (browseFindStreamRequest, error) {
	query := strings.TrimSpace(q.Get("q"))
	if len(query) > 256 {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusBadRequest, message: "search text is too long"}
	}
	mask := q.Get("wildcards") == "1" || q.Get("mask") == "1"
	within := strings.TrimSpace(q.Get("within"))
	if len(within) > 256 {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusBadRequest, message: "filter text is too long"}
	}
	withinMask := q.Get("withinWildcards") == "1" || q.Get("withinMask") == "1"
	cacheID := q.Get("wildcache")
	cacheGroup := q.Get("wildgroup")
	cacheOrder, orderErr := strconv.Atoi(q.Get("wildorder"))
	cacheCount, countErr := strconv.Atoi(q.Get("wildcount"))
	if cacheID != "" && !mask {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusBadRequest, message: "wildcard cache requires wildcard search"}
	}
	if cacheID != "" && !wildcardCacheIDPattern.MatchString(cacheID) {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusBadRequest, message: "invalid wildcard cache id"}
	}
	if cacheID != "" && (cacheGroup == "" || len(cacheGroup) > 256 || orderErr != nil || countErr != nil ||
		cacheCount < 1 || cacheCount > 10000 || cacheOrder < 0 || cacheOrder >= cacheCount) {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusBadRequest, message: "invalid wildcard cache group"}
	}
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusNotFound, message: err.Error()}
	}
	if !hasBrowseIndex(e.Path) {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusConflict,
			message: "This dictionary has no index. Create one in Dictionary Settings.", entry: e}
	}
	d, err := e.open()
	if err != nil {
		return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusInternalServerError, message: err.Error(), entry: e}
	}
	var withinRE *regexp.Regexp
	if within != "" && withinMask {
		withinRE, err = search.CompileMask(within)
		if err != nil {
			return browseFindStreamRequest{}, &browseFindSetupError{status: http.StatusBadRequest, message: "invalid filter: " + err.Error(), entry: e}
		}
	}
	page, _ := strconv.Atoi(q.Get("p"))
	if page < 1 {
		page = 1
	}
	if page > 10000000 {
		page = 10000000
	}
	return browseFindStreamRequest{entry: e, dict: d, query: query, mask: mask, within: within,
		withinMask: withinMask, withinRE: withinRE, cacheID: cacheID, cacheGroup: cacheGroup,
		cacheOrder: cacheOrder, cacheCount: cacheCount, page: page}, nil
}

func (s *Server) handleBrowseFindStream(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("merged") == "1" {
		st, ok := newStream(w, "application/x-ndjson")
		if !ok {
			return
		}
		if err := s.streamMergedBrowseFind(r.Context(), r.URL.Query(), func(event browseFindStreamEvent) error { st.line(event); return r.Context().Err() }); err != nil && r.Context().Err() == nil {
			st.line(browseFindStreamEvent{Type: "error", Error: err.Error()})
		}
		return
	}
	request, err := s.prepareBrowseFindStream(r.URL.Query())
	if err != nil {
		var failure *browseFindSetupError
		if errors.As(err, &failure) {
			if failure.status == http.StatusConflict && failure.entry != nil {
				browseUnprepared(w, failure.entry)
			} else {
				httpErr(w, failure.status, "%s", failure.message)
			}
		}
		return
	}
	st, ok := newStream(w, "application/x-ndjson")
	if !ok {
		return
	}
	err = s.streamBrowseFindCached(r.Context(), request, func(event browseFindStreamEvent) error {
		st.line(event)
		return r.Context().Err()
	})
	if err != nil && r.Context().Err() == nil {
		logx.System("browse find stream failed dict=%q mask=%v error=%q", request.entry.ID, request.mask, err)
		st.line(browseFindStreamEvent{Type: "error", Error: err.Error()})
	}
}

func (s *Server) streamBrowseFindCached(ctx context.Context, request browseFindStreamRequest, send func(browseFindStreamEvent) error) error {
	// An all-asterisk mask is Browse paging, not a scan of every headword.
	// Stream Browse's alphabet sampling progress while the first page is prepared.
	if request.mask && request.within == "" && isAllWildcardMask(request.query) {
		return s.streamAllWordsPage(ctx, request, send)
	}
	if request.cacheID == "" {
		return s.streamBrowseFind(ctx, request, func(event browseFindStreamEvent) error {
			event.Size = browsePageSize
			return send(event)
		})
	}
	entry, token, err := s.wildcardPages.begin(request.cacheID, wildcardCacheSignature{
		query: request.query, group: request.cacheGroup, wildcards: request.mask,
	}, browsePageSize, request.cacheCount, request.entry.ID, request.dict.Meta().Name, request.cacheOrder,
		request.within, request.withinMask, s.wildcardCacheDir())
	if err != nil {
		return err
	}
	streamKey := wildcardStreamKey{dictID: request.entry.ID, within: request.within, withinMask: request.withinMask}
	defer s.wildcardPages.finish(entry, &streamKey, token)
	err = s.streamBrowseFind(ctx, request, func(event browseFindStreamEvent) error {
		if err := entry.append(request.entry.ID, request.within, request.withinMask, token, event); err != nil {
			return err
		}
		event.Size = browsePageSize
		event.DiskCache = true
		return send(event)
	})
	if errors.Is(err, errWildcardCacheStreamReplaced) {
		return nil
	}
	return err
}

func (s *Server) streamAllWordsPage(ctx context.Context, request browseFindStreamRequest, send func(browseFindStreamEvent) error) error {
	browser, ok := request.dict.(dict.Browser)
	if !ok {
		return errors.New("dictionary does not support Browse paging")
	}
	meta := request.dict.Meta()
	var alphabet []dict.Letter
	var err error
	if streamer, ok := request.dict.(browseMaskAlphabetStreamer); ok {
		alphabet, _, err = streamer.StreamBrowseMaskAlphabet(ctx, func(scanned, scanTotal int) error {
			return send(browseFindStreamEvent{Type: "progress", Scanned: scanned, ScanTotal: scanTotal,
				Size: browsePageSize, DiskCache: request.cacheID != ""})
		})
		if err != nil {
			return err
		}
	} else if streamer, canStream := request.dict.(browseAlphabetStreamer); canStream {
		err = streamer.StreamBrowseAlphabet(ctx, func(letters []dict.Letter, scanned, scanTotal int) error {
			return send(browseFindStreamEvent{Type: "progress", Alphabet: letters, Total: max(meta.EntryCount, 0),
				Scanned: scanned, ScanTotal: scanTotal, Size: browsePageSize, DiskCache: request.cacheID != ""})
		})
		if err != nil {
			return err
		}
	}
	if alphabet == nil {
		alphabet, err = browser.Alphabet()
		if err != nil {
			return err
		}
	}
	total := 0
	for _, letter := range alphabet {
		total += letter.Count
	}
	pages := (total + browsePageSize - 1) / browsePageSize
	page := min(request.page, max(pages, 1))
	offset := (page - 1) * browsePageSize
	words, err := browser.Page(offset, browsePageSize)
	if err != nil {
		return err
	}
	positions := make([]int, len(words))
	for i := range positions {
		positions[i] = offset + i
	}
	event := browseFindStreamEvent{Type: "done", Words: words, Positions: positions, Alphabet: alphabet,
		Total: total, Scanned: total, ScanTotal: total, Size: browsePageSize, DiskCache: request.cacheID != "", Done: true}
	if request.cacheID != "" {
		cacheOrder, count := request.cacheOrder, request.cacheCount
		if err := s.wildcardPages.storePage(request.cacheID, wildcardCacheSignature{
			query: request.query, group: request.cacheGroup, wildcards: true,
		}, browsePageSize, count, request.entry.ID, meta.Name, cacheOrder, page,
			browseFindResp{Words: words, Positions: positions, Alphabet: alphabet, Total: total, Page: page,
				Pages: pages, Size: browsePageSize}, s.wildcardCacheDir()); err != nil {
			logx.System("wildcard cache page save failed dict=%q page=%d error=%q", request.entry.ID, page, err)
		}
	}
	return send(event)
}

func (s *Server) streamBrowseFind(ctx context.Context, request browseFindStreamRequest, send func(browseFindStreamEvent) error) error {
	var baseMaskSections dict.MaskSections
	emit := func(words []string, total int, done bool, scanned, scanTotal int) error {
		typ := "progress"
		if len(words) > 0 {
			typ = "block"
		}
		if done {
			typ = "done"
		}
		event := browseFindStreamEvent{Type: typ, Words: words, Total: total, Done: done,
			Scanned: scanned, ScanTotal: scanTotal}
		if request.mask && request.within == "" {
			for _, word := range words {
				baseMaskSections.Add(word)
			}
			if done {
				event.Alphabet = baseMaskSections.Finish()
			}
		}
		return send(event)
	}
	var err error
	if request.within == "" {
		if streamer, ok := request.dict.(browseHeadwordStreamer); ok {
			err = streamer.StreamBrowseHeadwords(ctx, request.query, request.mask, emit)
		} else {
			err = streamDirectBrowseHeadwords(ctx, request.dict, request.query, request.mask, emit)
		}
	} else {
		browseFindStreamsActive.Add(1)
		defer browseFindStreamsActive.Add(-1)
		terms := dict.HeadwordTermMatcher(request.within)
		baseOffset, filteredTotal := 0, 0
		filteredWords := make([]string, 0, browsePageSize)
		filteredPositions := make([]int, 0, browsePageSize)
		var sections dict.MaskSections
		filteredEmit := func(words []string, total int, done bool, scanned, scanTotal int) error {
			base := baseOffset
			for i, word := range words {
				if request.withinMask && !request.withinRE.MatchString(word) || !request.withinMask && !terms(word) {
					continue
				}
				sections.Add(word)
				filteredWords = append(filteredWords, word)
				filteredPositions = append(filteredPositions, base+i)
				filteredTotal++
				if len(filteredWords) == 1 || len(filteredWords) >= browsePageSize {
					if err := send(browseFindStreamEvent{Type: "block", Words: filteredWords,
						Positions: filteredPositions, Total: filteredTotal, Scanned: scanned, ScanTotal: scanTotal}); err != nil {
						return err
					}
					filteredWords = make([]string, 0, browsePageSize)
					filteredPositions = make([]int, 0, browsePageSize)
				}
			}
			baseOffset = total
			if done {
				if err := send(browseFindStreamEvent{Type: "done", Words: filteredWords, Positions: filteredPositions,
					Total: filteredTotal, Done: true, Alphabet: sections.Finish(), Scanned: scanned, ScanTotal: scanTotal}); err != nil {
					return err
				}
				filteredWords = nil
				filteredPositions = nil
				return ctx.Err()
			}
			if len(filteredWords) == 0 {
				if err := send(browseFindStreamEvent{Type: "progress", Total: filteredTotal,
					Scanned: scanned, ScanTotal: scanTotal}); err != nil {
					return err
				}
			}
			return ctx.Err()
		}
		if streamer, ok := request.dict.(browseHeadwordStreamer); ok {
			err = streamer.StreamBrowseHeadwords(ctx, request.query, request.mask, filteredEmit)
		} else {
			err = streamDirectBrowseHeadwords(ctx, request.dict, request.query, request.mask, filteredEmit)
		}
	}
	return err
}

func (s *Server) desktopBrowseFindStream(ctx context.Context, query string, emit func([]byte)) error {
	values, err := url.ParseQuery(query)
	if err != nil {
		return s.emitBrowseFindError(ctx, emit, "invalid browse search request")
	}
	if values.Get("merged") == "1" {
		return s.streamMergedBrowseFind(ctx, values, func(event browseFindStreamEvent) error {
			data, err := json.Marshal(event)
			if err == nil && emit != nil {
				emit(data)
			}
			return err
		})
	}
	request, err := s.prepareBrowseFindStream(values)
	if err != nil {
		return s.emitBrowseFindError(ctx, emit, err.Error())
	}
	encode := func(event browseFindStreamEvent) error {
		message, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if emit != nil {
			emit(message)
		}
		return ctx.Err()
	}
	err = s.streamBrowseFindCached(ctx, request, encode)
	if err != nil && ctx.Err() == nil {
		logx.System("browse find stream failed dict=%q mask=%v error=%q", request.entry.ID, request.mask, err)
		return s.emitBrowseFindError(ctx, emit, err.Error())
	}
	return err
}

func (s *Server) emitBrowseFindError(ctx context.Context, emit func([]byte), message string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	payload, err := json.Marshal(browseFindStreamEvent{Type: "error", Error: message})
	if err != nil {
		return err
	}
	if emit != nil {
		emit(payload)
	}
	return nil
}

func streamDirectBrowseHeadwords(ctx context.Context, d dict.Dictionary, query string, mask bool, emit func([]string, int, bool, int, int) error) error {
	var re *regexp.Regexp
	var err error
	if mask {
		re, err = search.CompileMask(query)
		if err != nil {
			return err
		}
	}
	terms := dict.HeadwordTermMatcher(query)
	words := make([]string, 0, browsePageSize)
	total, previous := 0, ""
	scanned := 0
	firstMatch := false
	entryCount := d.Meta().EntryCount
	flush := func(done bool) error {
		if err := emit(words, total, done, scanned, entryCount); err != nil {
			return err
		}
		words = make([]string, 0, browsePageSize)
		return nil
	}
	lastProgress := time.Now()
	for at := 0; at < entryCount; at += 256 {
		if err := ctx.Err(); err != nil {
			return err
		}
		page := d.Keywords(at, min(256, entryCount-at))
		for _, word := range page {
			scanned++
			if word == previous || strings.HasPrefix(word, "@_") {
				continue
			}
			previous = word
			matched := terms(word)
			if mask {
				matched = re.MatchString(word)
			}
			if matched {
				words = append(words, word)
				total++
				if !firstMatch {
					if err := flush(false); err != nil {
						return err
					}
					firstMatch = true
				}
			}
		}
		scanned = min(at+len(page), entryCount)
		now := time.Now()
		if len(words) >= browsePageSize || (len(words) > 0 && now.Sub(lastProgress) >= time.Second) {
			if err := flush(false); err != nil {
				return err
			}
			lastProgress = now
		} else if now.Sub(lastProgress) >= time.Second {
			if err := emit(nil, total, false, scanned, entryCount); err != nil {
				return err
			}
			lastProgress = now
		}
	}
	if len(words) > 0 {
		if err := flush(false); err != nil {
			return err
		}
	}
	return emit(nil, total, true, entryCount, entryCount)
}

// Direct dictionaries expose headwords in pages. Browse search scans those
// pages without opening article bodies and checks cancellation between them.
func findDirectHeadwords(ctx context.Context, d dict.Dictionary, query string, mask bool, within string, withinMask bool, offset, limit int) ([]string, int, []dict.Letter, []int, error) {
	var re *regexp.Regexp
	var err error
	if mask {
		re, err = search.CompileMask(query)
		if err != nil {
			return nil, 0, nil, nil, err
		}
	}
	var withinRE *regexp.Regexp
	if within != "" && withinMask {
		withinRE, err = search.CompileMask(within)
		if err != nil {
			return nil, 0, nil, nil, err
		}
	}
	matchTerms := dict.HeadwordTermMatcher(query)
	withinTerms := dict.HeadwordTermMatcher(within)
	words := make([]string, 0, limit)
	positions := make([]int, 0, limit)
	var sections dict.MaskSections
	total, basePosition, previous := 0, 0, ""
	entryCount := d.Meta().EntryCount
	for at := 0; at < entryCount; at += 256 {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, nil, err
		}
		n := min(256, entryCount-at)
		page := d.Keywords(at, n)
		for _, word := range page {
			if word == previous || strings.HasPrefix(word, "@_") {
				continue
			}
			previous = word
			matched := false
			if mask {
				matched = re.MatchString(word)
			} else {
				matched = matchTerms(word)
			}
			if !matched {
				continue
			}
			position := basePosition
			basePosition++
			if within != "" && (withinMask && !withinRE.MatchString(word) || !withinMask && !withinTerms(word)) {
				continue
			}
			sections.Add(word)
			if total >= offset && len(words) < limit {
				words = append(words, word)
				positions = append(positions, position)
			}
			total++
		}
	}
	return words, total, sections.Finish(), positions, nil
}

// browsePageSize is how many headwords one browse page holds, and it is a
// constant rather than a parameter on purpose: the page number is the whole
// address of a browse view, so a bookmark taken today has to mean the same
// place tomorrow. A client-chosen size would make ?p=37 a different page for
// every reader who changed it.
//
// 300 is a screenful of columns on a desktop and a short scroll on a phone,
// which is the trade a paper page makes too.
const browsePageSize = indexconst.BrowsePageSize

type browsePageMeasurer interface {
	PageWithStats(offset, n int) ([]string, store.BrowsePageStats, error)
}

func readBrowsePage(b dict.Browser, offset, size int, diagnostics bool, transport, dictID, name string) ([]string, error) {
	if !diagnostics {
		return b.Page(offset, size)
	}
	started := time.Now()
	if measured, ok := b.(browsePageMeasurer); ok {
		words, stats, err := measured.PageWithStats(offset, size)
		errorText := ""
		if err != nil {
			errorText = err.Error()
		}
		logx.System("browse_page transport=%s dict=%q name=%q page=%d offset=%d size=%d anchor_index=%t anchor_used=%t anchor_ordinal=%d anchor_lookup_ms=%d read_ms=%d rows_read=%d words=%d total_ms=%d error=%q",
			transport, dictID, name, offset/size+1, offset, size, stats.AnchorIndex, stats.AnchorUsed, stats.AnchorOrdinal,
			stats.AnchorLookup.Milliseconds(), stats.Read.Milliseconds(), stats.RowsRead, stats.Words,
			time.Since(started).Milliseconds(), errorText)
		return words, err
	}
	words, err := b.Page(offset, size)
	errorText := ""
	if err != nil {
		errorText = err.Error()
	}
	logx.System("browse_page transport=%s dict=%q name=%q page=%d offset=%d size=%d anchor_index=false anchor_used=false rows_read=%d words=%d total_ms=%d error=%q",
		transport, dictID, name, offset/size+1, offset, size, len(words), len(words), time.Since(started).Milliseconds(), errorText)
	return words, err
}

// browseResp is one page of the word list plus everything needed to draw the
// chrome around it, so a page turn is one request. The strip travels with
// every page (a kilobyte) instead of being a second endpoint the client has to
// sequence against the first.
type browseResp struct {
	Dict  string `json:"dict"`
	Name  string `json:"name"`
	Title string `json:"title,omitempty"` // user-facing name, if customized
	Total int    `json:"total"`
	Page  int    `json:"page"`  // 1-based, as the URL spells it
	Pages int    `json:"pages"` // 0 when the dictionary is empty
	Size  int    `json:"size"`
	// At is the row this view was ASKED for, which is not the row it starts
	// at: a jump snaps down to the page grid, so a page reached by asking for
	// K usually opens in the tail of J. The client highlights the strip by
	// this, or clicking K lights up J.
	At            int           `json:"at"`
	Words         []string      `json:"words"`
	Alphabet      []dict.Letter `json:"alphabet"`
	mergedCacheID string
}

type browsePageFailure struct {
	status     int
	message    string
	entry      *entry
	unprepared bool
}

func (e *browsePageFailure) Error() string { return e.message }

// handleBrowse serves the dictionary as an ordered list of headwords: the
// reading view, as opposed to /api/search's answering view.
//
// Prepared dictionaries browse through dict.Browser's SQLite index. ZIM also
// implements dict.Browser over its mandatory native path-pointer list, without
// preparing a second index. Other unprepared direct dictionaries are answered
// with 409 rather than sorted into a costly in-memory list.
//
// Same-origin, deliberately absent from corsAllowed (D69): the extension grant
// is search, and a full headword dump is not a lookup.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("stream") == "1" {
		s.handleBrowseStream(w, r)
		return
	}
	result, err := s.browsePage(r.URL.Query())
	if err != nil {
		var failure *browsePageFailure
		if errors.As(err, &failure) {
			if failure.unprepared && failure.entry != nil {
				browseUnprepared(w, failure.entry)
			} else {
				httpErr(w, failure.status, "%s", failure.message)
			}
		} else {
			httpErr(w, http.StatusInternalServerError, "%s", err)
		}
		return
	}
	writeJSON(w, result)
}

func (s *Server) browsePage(q url.Values) (browseResp, error) {
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		return browseResp{}, &browsePageFailure{status: http.StatusNotFound, message: err.Error()}
	}
	// Checked BEFORE the open: Browse pages the prepared headword index. ZIM's
	// native path list remains available for search, but does not support this
	// paged token-search and filtering view.
	if !hasBrowseIndex(e.Path) {
		return browseResp{}, &browsePageFailure{status: http.StatusConflict,
			message: "This dictionary has no index. Create one in Dictionary Settings.", entry: e, unprepared: true}
	}
	d, err := e.open()
	if err != nil {
		return browseResp{}, &browsePageFailure{status: http.StatusInternalServerError, message: err.Error()}
	}
	b, ok := d.(dict.Browser)
	if !ok { // prepared on disk, but this handle is not the prepared backend
		return browseResp{}, &browsePageFailure{status: http.StatusConflict,
			message: "This dictionary has no index. Create one in Dictionary Settings.", entry: e, unprepared: true}
	}
	// The client already has the completed strip on a strip=0 request. For
	// ZIM, building it walks the native index and can take minutes, so a page
	// return must not wait for Alphabet merely to discard its answer.
	zimNative := entryHasSelfIndex(e) && !d.Meta().HeadwordsOnly
	var alphabet []dict.Letter
	if !(zimNative && q.Get("strip") == "0") {
		alphabet, err = b.Alphabet()
		if err != nil {
			return browseResp{}, &browsePageFailure{status: http.StatusInternalServerError, message: fmt.Sprintf("browsing %s: %v", e.ID, err)}
		}
	}
	// The strip partitions the whole list, so its counts ARE the total - no
	// COUNT(*) of its own, and no way for the two to disagree.
	//
	// Summed, not taken from the tail chip: chips merge runs that are far
	// apart (A holds both "Ä…" and "ä…", with half the Latin supplement in
	// between), so the last chip in strip order is not always the one the last
	// headword falls under, and offset+count of it would report a dictionary
	// short of its own ending.
	total := 0
	for _, l := range alphabet {
		total += l.Count
	}
	if zimNative {
		// ZIM's strip samples the first visible headword in each native page;
		// its offsets and total therefore use native index slots.
		total = d.Meta().EntryCount
	}

	offset, mark := 0, -1
	if at := q.Get("at"); at != "" {
		// A jump lands on the PAGE holding the word, not on the word: the
		// reader asked to be put down in the list there, and a page that
		// started mid-letter every time would have no stable address.
		if mark, err = b.Locate(at); err != nil {
			return browseResp{}, &browsePageFailure{status: http.StatusInternalServerError, message: fmt.Sprintf("browsing %s: %v", e.ID, err)}
		}
		offset = mark - mark%browsePageSize
	} else if p, _ := strconv.Atoi(q.Get("p")); p > 1 {
		offset = (p - 1) * browsePageSize
	}
	// Past the end - a stale bookmark, or a jump past the last headword -
	// shows the last page rather than an empty one or an error.
	if total > 0 && offset >= total {
		offset = (total - 1) / browsePageSize * browsePageSize
	}
	// A mark outside the page it produced is a stale bookmark or a jump past
	// the end; the page's own start is then the only honest answer.
	if mark < offset || mark >= offset+browsePageSize {
		mark = offset
	}
	words, err := readBrowsePage(b, offset, browsePageSize, q.Get("diagnostics") == "1", "page", e.ID, d.Meta().Name)
	if err != nil {
		return browseResp{}, &browsePageFailure{status: http.StatusInternalServerError, message: fmt.Sprintf("browsing %s: %v", e.ID, err)}
	}
	if words == nil {
		words = []string{}
	}
	if alphabet == nil {
		alphabet = []dict.Letter{}
	}
	responseAlphabet := alphabet
	if q.Get("strip") == "0" {
		responseAlphabet = nil
	}
	return browseResp{
		Dict:     e.ID,
		Name:     d.Meta().Name,
		Title:    s.reg.prefs.DisplayTitle(e.ID),
		Total:    total,
		Page:     offset/browsePageSize + 1,
		Pages:    (total + browsePageSize - 1) / browsePageSize,
		Size:     browsePageSize,
		At:       mark,
		Words:    words,
		Alphabet: responseAlphabet,
	}, nil
}

type browseStreamEvent struct {
	Type    string        `json:"type"`
	Status  int           `json:"status,omitempty"`
	Page    *browseResp   `json:"page,omitempty"`
	Letters []dict.Letter `json:"letters,omitempty"`
	At      int           `json:"at,omitempty"`
	Total   int           `json:"total,omitempty"`
	Error   string        `json:"error,omitempty"`
}

type browseAlphabetStreamer interface {
	StreamBrowseAlphabet(context.Context, func([]dict.Letter, int, int) error) error
}

type browseMaskAlphabetStreamer interface {
	StreamBrowseMaskAlphabet(context.Context, func(int, int) error) ([]dict.Letter, int, error)
}

// desktopBrowseStream runs the paged Browse view directly in the Windows
// bridge. Browser and Android clients continue to use handleBrowseStream.
func (s *Server) desktopBrowseStream(ctx context.Context, query string, emit func([]byte)) error {
	q, err := url.ParseQuery(query)
	if err != nil {
		return fmt.Errorf("invalid browse request: %w", err)
	}
	diagnostics := q.Get("diagnostics") == "1"
	started := time.Now()
	phase := atomic.Int64{}
	progressAt, progressTotal, letterCount := atomic.Int64{}, atomic.Int64{}, atomic.Int64{}
	phases := []string{"dictionary_lookup", "dictionary_open", "page_read", "alphabet_stream", "complete"}
	if diagnostics {
		logx.System("browse_stream event=start transport=desktop dict=%q", q.Get("dict"))
		stop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					logx.System("browse_stream event=slow_phase transport=desktop dict=%q phase=%s elapsed_ms=%d progress_at=%d total=%d letters=%d",
						q.Get("dict"), phases[min(int(phase.Load()), len(phases)-1)], time.Since(started).Milliseconds(), progressAt.Load(), progressTotal.Load(), letterCount.Load())
				}
			}
		}()
		defer func() {
			close(stop)
			logx.System("browse_stream event=finished transport=desktop dict=%q phase=%s elapsed_ms=%d progress_at=%d total=%d letters=%d canceled=%t",
				q.Get("dict"), phases[min(int(phase.Load()), len(phases)-1)], time.Since(started).Milliseconds(), progressAt.Load(), progressTotal.Load(), letterCount.Load(), ctx.Err() != nil)
		}()
	}
	send := func(event browseStreamEvent) error {
		if emit == nil {
			return nil
		}
		data, err := json.Marshal(event)
		if err == nil {
			emit(data)
		}
		return err
	}
	fail := func(status int, err error) error {
		if diagnostics {
			logx.System("browse_stream event=error transport=desktop dict=%q phase=%s status=%d error=%q",
				q.Get("dict"), phases[min(int(phase.Load()), len(phases)-1)], status, err)
		}
		return send(browseStreamEvent{Type: "error", Status: status, Error: err.Error()})
	}
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		return fail(http.StatusNotFound, err)
	}
	if !hasBrowseIndex(e.Path) {
		return fail(http.StatusConflict, errors.New("This dictionary has no index. Create one in Dictionary Settings."))
	}
	phase.Store(1)
	d, err := e.open()
	if err != nil {
		return fail(http.StatusInternalServerError, err)
	}
	b, ok := d.(dict.Browser)
	if !ok {
		return fail(http.StatusConflict, errors.New("This dictionary has no index. Create one in Dictionary Settings."))
	}
	meta := d.Meta()
	total := max(meta.EntryCount, 0)
	streamer, canStreamAlphabet := d.(browseAlphabetStreamer)
	var readyAlphabet []dict.Letter
	if !canStreamAlphabet {
		phase.Store(3)
		readyAlphabet, err = b.Alphabet()
		if err != nil {
			return fail(http.StatusInternalServerError, err)
		}
		total = 0
		for _, letter := range readyAlphabet {
			total += letter.Count
		}
	}
	offset, mark := 0, -1
	if at := q.Get("at"); at != "" {
		if mark, err = b.Locate(at); err != nil {
			return fail(http.StatusInternalServerError, err)
		}
		offset = mark - mark%browsePageSize
	} else if p, _ := strconv.Atoi(q.Get("p")); p > 1 {
		offset = (p - 1) * browsePageSize
	}
	if total > 0 && offset >= total {
		offset = (total - 1) / browsePageSize * browsePageSize
	}
	if mark < offset || mark >= offset+browsePageSize {
		mark = offset
	}
	phase.Store(2)
	words, err := readBrowsePage(b, offset, browsePageSize, diagnostics, "desktop", e.ID, meta.Name)
	if err != nil {
		return fail(http.StatusInternalServerError, err)
	}
	if words == nil {
		words = []string{}
	}
	page := offset/browsePageSize + 1
	if err := send(browseStreamEvent{Type: "page", Page: &browseResp{Dict: e.ID, Name: meta.Name, Title: s.reg.prefs.DisplayTitle(e.ID),
		Total: total, Page: page, Pages: (total + browsePageSize - 1) / browsePageSize,
		Size: browsePageSize, At: mark, Words: words, Alphabet: []dict.Letter{}}}); err != nil {
		return err
	}
	emitLetters := func(letters []dict.Letter, at, all int) error {
		progressAt.Store(int64(at))
		progressTotal.Store(int64(all))
		letterCount.Add(int64(len(letters)))
		if len(letters) > 0 {
			return send(browseStreamEvent{Type: "letters", Letters: letters, At: at, Total: all})
		}
		return send(browseStreamEvent{Type: "progress", At: at, Total: all})
	}
	if canStreamAlphabet {
		phase.Store(3)
		err = streamer.StreamBrowseAlphabet(ctx, emitLetters)
	} else {
		phase.Store(3)
		for start := 0; start < len(readyAlphabet); start += 512 {
			end := min(start+512, len(readyAlphabet))
			if err = emitLetters(readyAlphabet[start:end], end*browsePageSize, total); err != nil {
				break
			}
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if diagnostics {
			logx.System("browse_stream event=error transport=desktop dict=%q phase=alphabet_stream error=%q", q.Get("dict"), err)
		}
		return send(browseStreamEvent{Type: "error", Error: err.Error()})
	}
	phase.Store(4)
	return send(browseStreamEvent{Type: "done", At: total, Total: total})
}

func (s *Server) handleBrowseStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	started := time.Now()
	diagnostics := q.Get("diagnostics") == "1"
	phase := atomic.Int64{}
	progressAt, progressTotal, letterCount := atomic.Int64{}, atomic.Int64{}, atomic.Int64{}
	phaseNames := []string{"dictionary_lookup", "dictionary_open", "page_read", "alphabet_stream", "complete"}
	stopHeartbeat := make(chan struct{})
	if diagnostics {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-r.Context().Done():
					return
				case <-ticker.C:
					index := min(int(phase.Load()), len(phaseNames)-1)
					logx.System("browse_stream event=slow_phase phase=%s elapsed_ms=%d progress_at=%d total=%d letters=%d",
						phaseNames[index], time.Since(started).Milliseconds(), progressAt.Load(), progressTotal.Load(), letterCount.Load())
				}
			}
		}()
		defer func() {
			close(stopHeartbeat)
			index := min(int(phase.Load()), len(phaseNames)-1)
			logx.System("browse_stream event=finished phase=%s elapsed_ms=%d progress_at=%d total=%d letters=%d",
				phaseNames[index], time.Since(started).Milliseconds(), progressAt.Load(), progressTotal.Load(), letterCount.Load())
		}()
	}
	phase.Store(0)
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		httpErr(w, 404, "%v", err)
		return
	}
	if !hasBrowseIndex(e.Path) {
		browseUnprepared(w, e)
		return
	}
	phase.Store(1)
	d, err := e.open()
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	b, ok := d.(dict.Browser)
	if !ok {
		browseUnprepared(w, e)
		return
	}
	phase.Store(2)
	st, ok := newStream(w, "application/x-ndjson")
	if !ok {
		return
	}
	meta := d.Meta()
	total := meta.EntryCount
	if total < 0 {
		total = 0
	}
	streamer, canStreamAlphabet := d.(browseAlphabetStreamer)
	var readyAlphabet []dict.Letter
	if !canStreamAlphabet {
		phase.Store(3)
		readyAlphabet, err = b.Alphabet()
		if err != nil {
			st.line(browseStreamEvent{Type: "error", Error: err.Error()})
			return
		}
		total = 0
		for _, letter := range readyAlphabet {
			total += letter.Count
		}
	}
	offset, mark := 0, -1
	if at := q.Get("at"); at != "" {
		if mark, err = b.Locate(at); err != nil {
			st.line(browseStreamEvent{Type: "error", Error: err.Error()})
			return
		}
		offset = mark - mark%browsePageSize
	} else if p, _ := strconv.Atoi(q.Get("p")); p > 1 {
		offset = (p - 1) * browsePageSize
	}
	if total > 0 && offset >= total {
		offset = (total - 1) / browsePageSize * browsePageSize
	}
	if mark < offset || mark >= offset+browsePageSize {
		mark = offset
	}
	words, err := readBrowsePage(b, offset, browsePageSize, diagnostics, "http", e.ID, meta.Name)
	if err != nil {
		st.line(browseStreamEvent{Type: "error", Error: err.Error()})
		return
	}
	if words == nil {
		words = []string{}
	}
	pages := (total + browsePageSize - 1) / browsePageSize
	page := offset/browsePageSize + 1
	st.line(browseStreamEvent{Type: "page", Page: &browseResp{Dict: e.ID, Name: meta.Name, Title: s.reg.prefs.DisplayTitle(e.ID), Total: total,
		Page: page, Pages: pages, Size: browsePageSize, At: mark, Words: words, Alphabet: []dict.Letter{}}})
	phase.Store(3)

	emit := func(letters []dict.Letter, at, all int) error {
		progressAt.Store(int64(at))
		progressTotal.Store(int64(all))
		letterCount.Add(int64(len(letters)))
		if len(letters) > 0 {
			st.line(browseStreamEvent{Type: "letters", Letters: letters, At: at, Total: all})
		} else {
			st.line(browseStreamEvent{Type: "progress", At: at, Total: all})
		}
		return r.Context().Err()
	}
	if canStreamAlphabet {
		err = streamer.StreamBrowseAlphabet(r.Context(), emit)
	} else {
		for start := 0; start < len(readyAlphabet); start += 512 {
			end := min(start+512, len(readyAlphabet))
			if err = emit(readyAlphabet[start:end], end*dict.MaskPageSize, total); err != nil {
				break
			}
		}
	}
	if err != nil {
		if r.Context().Err() == nil {
			st.line(browseStreamEvent{Type: "error", Error: err.Error()})
		}
		return
	}
	st.line(browseStreamEvent{Type: "done", At: total, Total: total})
	phase.Store(4)
}

// browseUnprepared answers the state where the dictionary has no library index
// to page through. The user can create the index in Dictionary Settings.
func browseUnprepared(w http.ResponseWriter, e *entry) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{
		"error":    "This dictionary has no index. Create one in Dictionary Settings.",
		"prepared": false,
	})
}

// handleBrowsePage serves the browse view. Like the lemma installer, no state
// is baked in: the page asks /api/browse for everything it draws, so the same
// bytes serve every dictionary and the browser caches nothing that can go
// stale against the library.
//
// Preset links carry the standalone-page palette, and the list search script
// uses its content hash so a cached chooser cannot keep stale search behavior.
func (s *Server) handleBrowsePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Add("Vary", "Accept-Language")
	_, _ = w.Write(s.browsePageHTMLFor(r.Header.Get("Accept-Language")))
}

func (s *Server) browsePageHTML() []byte {
	return s.browsePageHTMLFor("")
}

func (s *Server) browsePageHTMLFor(acceptLanguage string) []byte {
	page := bytes.ReplaceAll(browseHTML, []byte("{{PRESETS}}"), []byte(s.pagePresetLinks()))
	page = bytes.ReplaceAll(page, []byte("{{BROWSE_STRIP_EXACT_WIDTHS}}"), []byte(strconv.Itoa(indexconst.BrowseStripExactWidthCount)))
	page = bytes.ReplaceAll(page, []byte("{{SEARCH_DEBOUNCE_MS}}"), []byte(strconv.Itoa(indexconst.SearchDebounceMS)))
	page = bytes.ReplaceAll(page, []byte("{{WORKSPACECSS}}"), []byte(assetTag(workspaceCSS)))
	page = bytes.ReplaceAll(page, []byte("{{LISTSEARCHJS}}"), []byte(assetTag(listSearchJS)))
	page = bytes.ReplaceAll(page, []byte("{{HEADWORDJS}}"), []byte(assetTag(headwordControlJS)))
	page = bytes.ReplaceAll(page, []byte("{{RIBBONJS}}"), []byte(assetTag(headwordRibbonJS)))
	page = bytes.ReplaceAll(page, []byte("{{HEADWORDCSS}}"), []byte(assetTag(headwordControlCSS)))
	return s.renderUserUIChoice(string(page), s.reg.prefs.ResolvedLanguage(acceptLanguage), s.reg.prefs.Language())
}
