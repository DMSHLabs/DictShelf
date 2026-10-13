// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/search"
)

type browseFindResp struct {
	Words     []string      `json:"words"`
	Alphabet  []dict.Letter `json:"alphabet,omitempty"`
	Positions []int         `json:"positions,omitempty"`
	Total     int           `json:"total"`
	Page      int           `json:"page"`
	Pages     int           `json:"pages"`
	Size      int           `json:"size"`
}

// handleBrowseFind searches the complete headword index, then pages the
// matches. Prepared dictionaries answer from their index; ZIM scans its native
// headword list because its path index has no arbitrary token-search operation.
func (s *Server) handleBrowseFind(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("stream") == "1" {
		s.handleBrowseFindStream(w, r)
		return
	}
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	if len(query) > 256 {
		httpErr(w, 400, "search text is too long")
		return
	}
	mask := q.Get("wildcards") == "1" || q.Get("mask") == "1"
	within := strings.TrimSpace(q.Get("within"))
	if len(within) > 256 {
		httpErr(w, 400, "filter text is too long")
		return
	}
	withinMask := q.Get("withinWildcards") == "1" || q.Get("withinMask") == "1"
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		httpErr(w, 404, "%v", err)
		return
	}
	if !hasBrowseIndex(e.Path) {
		browseUnprepared(w, e)
		return
	}
	d, err := e.open()
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	finder, ok := d.(dict.BrowseFinder)
	if !ok {
		browseUnprepared(w, e)
		return
	}
	page, _ := strconv.Atoi(q.Get("p"))
	if page < 1 {
		page = 1
	}
	// A stale or malformed bookmark cannot drive unbounded OFFSET work.
	if page > 10000000 {
		page = 10000000
	}
	find := func(page int) ([]string, int, []dict.Letter, []int, error) {
		if mask {
			if ok {
				if within != "" {
					if extended, yes := d.(interface {
						FindHeadwordsMaskWithin(context.Context, string, string, bool, int, int) ([]string, int, []dict.Letter, []int, error)
					}); yes {
						return extended.FindHeadwordsMaskWithin(r.Context(), query, within, withinMask, (page-1)*browsePageSize, browsePageSize)
					}
					return findDirectHeadwords(r, d, query, true, within, withinMask, (page-1)*browsePageSize, browsePageSize)
				}
				words, total, letters, err := finder.FindHeadwordsMask(r.Context(), query, (page-1)*browsePageSize, browsePageSize)
				return words, total, letters, nil, err
			}
			return findDirectHeadwords(r, d, query, true, within, withinMask, (page-1)*browsePageSize, browsePageSize)
		}
		if ok {
			words, total, err := finder.FindHeadwords(r.Context(), query, (page-1)*browsePageSize, browsePageSize)
			return words, total, nil, nil, err
		}
		return findDirectHeadwords(r, d, query, false, within, withinMask, (page-1)*browsePageSize, browsePageSize)
	}
	words, total, letters, positions, err := find(page)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		logx.System("browse find failed dict=%q mask=%v error=%q", e.ID, mask, err)
		httpErr(w, 500, "searching %s: %v", e.ID, err)
		return
	}
	pages := (total + browsePageSize - 1) / browsePageSize
	if pages > 0 && page > pages {
		page = pages
		words, _, letters, positions, err = find(page)
		if err != nil {
			httpErr(w, 500, "searching %s: %v", e.ID, err)
			return
		}
	}
	writeJSON(w, browseFindResp{Words: words, Alphabet: letters, Positions: positions, Total: total, Page: page, Pages: pages, Size: browsePageSize})
}

type browseFindStreamEvent struct {
	Type      string        `json:"type"`
	Words     []string      `json:"words,omitempty"`
	Positions []int         `json:"positions,omitempty"`
	Alphabet  []dict.Letter `json:"alphabet,omitempty"`
	Total     int           `json:"total"`
	Done      bool          `json:"done,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type browseHeadwordStreamer interface {
	StreamBrowseHeadwords(context.Context, string, bool, func([]string, int, bool) error) error
}

func (s *Server) handleBrowseFindStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	if len(query) > 256 {
		httpErr(w, 400, "search text is too long")
		return
	}
	mask := q.Get("wildcards") == "1" || q.Get("mask") == "1"
	within := strings.TrimSpace(q.Get("within"))
	if len(within) > 256 {
		httpErr(w, 400, "filter text is too long")
		return
	}
	withinMask := q.Get("withinWildcards") == "1" || q.Get("withinMask") == "1"
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		httpErr(w, 404, "%v", err)
		return
	}
	if !hasBrowseIndex(e.Path) {
		browseUnprepared(w, e)
		return
	}
	d, err := e.open()
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	st, ok := newStream(w, "application/x-ndjson")
	if !ok {
		return
	}
	emit := func(words []string, total int, done bool) error {
		typ := "progress"
		if len(words) > 0 {
			typ = "block"
		}
		if done {
			typ = "done"
		}
		st.line(browseFindStreamEvent{Type: typ, Words: words, Total: total, Done: done})
		return r.Context().Err()
	}
	if within == "" {
		if streamer, ok := d.(browseHeadwordStreamer); ok {
			err = streamer.StreamBrowseHeadwords(r.Context(), query, mask, emit)
		} else {
			err = streamDirectBrowseHeadwords(r, d, query, mask, emit)
		}
	} else {
		browseFindStreamsActive.Add(1)
		defer browseFindStreamsActive.Add(-1)
		var withinRE *regexp.Regexp
		if withinMask {
			withinRE, err = search.CompileMask(within)
			if err != nil {
				httpErr(w, 400, "invalid filter: %v", err)
				return
			}
		}
		terms := dict.HeadwordTermMatcher(within)
		baseOffset, filteredTotal := 0, 0
		filteredWords := make([]string, 0, browsePageSize)
		filteredPositions := make([]int, 0, browsePageSize)
		var sections dict.MaskSections
		filteredEmit := func(words []string, total int, done bool) error {
			base := baseOffset
			for i, word := range words {
				if withinMask && !withinRE.MatchString(word) || !withinMask && !terms(word) {
					continue
				}
				sections.Add(word)
				filteredWords = append(filteredWords, word)
				filteredPositions = append(filteredPositions, base+i)
				filteredTotal++
			}
			baseOffset = total
			if len(filteredWords) >= browsePageSize || done {
				typ := "block"
				if done {
					typ = "done"
				}
				st.line(browseFindStreamEvent{Type: typ, Words: filteredWords, Positions: filteredPositions,
					Total: filteredTotal, Done: done, Alphabet: func() []dict.Letter {
						if done {
							return sections.Finish()
						}
						return nil
					}()})
				filteredWords = make([]string, 0, browsePageSize)
				filteredPositions = make([]int, 0, browsePageSize)
				return r.Context().Err()
			}
			typ := "progress"
			st.line(browseFindStreamEvent{Type: typ, Total: filteredTotal})
			return r.Context().Err()
		}
		if streamer, ok := d.(browseHeadwordStreamer); ok {
			err = streamer.StreamBrowseHeadwords(r.Context(), query, mask, filteredEmit)
		} else {
			err = streamDirectBrowseHeadwords(r, d, query, mask, filteredEmit)
		}
	}
	if err != nil && r.Context().Err() == nil {
		logx.System("browse find stream failed dict=%q mask=%v error=%q", e.ID, mask, err)
		st.line(browseFindStreamEvent{Type: "error", Error: err.Error()})
	}
}

func streamDirectBrowseHeadwords(r *http.Request, d dict.Dictionary, query string, mask bool, emit func([]string, int, bool) error) error {
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
	flush := func(done bool) error {
		if err := emit(words, total, done); err != nil {
			return err
		}
		words = make([]string, 0, browsePageSize)
		return nil
	}
	entryCount := d.Meta().EntryCount
	lastProgress := time.Now()
	for at := 0; at < entryCount; at += 256 {
		if err := r.Context().Err(); err != nil {
			return err
		}
		page := d.Keywords(at, min(256, entryCount-at))
		for _, word := range page {
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
			}
		}
		now := time.Now()
		if len(words) >= browsePageSize || (len(words) > 0 && now.Sub(lastProgress) >= time.Second) {
			if err := flush(false); err != nil {
				return err
			}
			lastProgress = now
		} else if now.Sub(lastProgress) >= time.Second {
			if err := emit(nil, total, false); err != nil {
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
	return emit(nil, total, true)
}

// Direct dictionaries expose headwords in pages. Browse search scans those
// pages without opening article bodies and checks cancellation between them.
func findDirectHeadwords(r *http.Request, d dict.Dictionary, query string, mask bool, within string, withinMask bool, offset, limit int) ([]string, int, []dict.Letter, []int, error) {
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
		if err := r.Context().Err(); err != nil {
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
const browsePageSize = dict.MaskPageSize

// browseResp is one page of the word list plus everything needed to draw the
// chrome around it, so a page turn is one request. The strip travels with
// every page (a kilobyte) instead of being a second endpoint the client has to
// sequence against the first.
type browseResp struct {
	Dict  string `json:"dict"`
	Name  string `json:"name"`
	Total int    `json:"total"`
	Page  int    `json:"page"`  // 1-based, as the URL spells it
	Pages int    `json:"pages"` // 0 when the dictionary is empty
	Size  int    `json:"size"`
	// At is the row this view was ASKED for, which is not the row it starts
	// at: a jump snaps down to the page grid, so a page reached by asking for
	// K usually opens in the tail of J. The client highlights the strip by
	// this, or clicking K lights up J.
	At       int           `json:"at"`
	Words    []string      `json:"words"`
	Alphabet []dict.Letter `json:"alphabet"`
}

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
	q := r.URL.Query()
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		httpErr(w, 404, "%v", err)
		return
	}
	// Checked BEFORE the open: Browse pages the prepared headword index. ZIM's
	// native path list remains available for search, but does not support this
	// paged token-search and filtering view.
	if !hasBrowseIndex(e.Path) {
		browseUnprepared(w, e)
		return
	}
	d, err := e.open()
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	b, ok := d.(dict.Browser)
	if !ok { // prepared on disk, but this handle is not the prepared backend
		browseUnprepared(w, e)
		return
	}
	// The client already has the completed strip on a strip=0 request. For
	// ZIM, building it walks the native index and can take minutes, so a page
	// return must not wait for Alphabet merely to discard its answer.
	zimNative := entryHasSelfIndex(e) && !d.Meta().HeadwordsOnly
	var alphabet []dict.Letter
	if !(zimNative && q.Get("strip") == "0") {
		alphabet, err = b.Alphabet()
		if err != nil {
			httpErr(w, 500, "browsing %s: %v", e.ID, err)
			return
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
			httpErr(w, 500, "browsing %s: %v", e.ID, err)
			return
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
	words, err := b.Page(offset, browsePageSize)
	if err != nil {
		httpErr(w, 500, "browsing %s: %v", e.ID, err)
		return
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
	writeJSON(w, browseResp{
		Dict:     e.ID,
		Name:     d.Meta().Name,
		Total:    total,
		Page:     offset/browsePageSize + 1,
		Pages:    (total + browsePageSize - 1) / browsePageSize,
		Size:     browsePageSize,
		At:       mark,
		Words:    words,
		Alphabet: responseAlphabet,
	})
}

type browseStreamEvent struct {
	Type    string        `json:"type"`
	Page    *browseResp   `json:"page,omitempty"`
	Letters []dict.Letter `json:"letters,omitempty"`
	At      int           `json:"at,omitempty"`
	Total   int           `json:"total,omitempty"`
	Error   string        `json:"error,omitempty"`
}

type browseAlphabetStreamer interface {
	StreamBrowseAlphabet(context.Context, func([]dict.Letter, int, int) error) error
}

func (s *Server) handleBrowseStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	e, err := s.reg.get(q.Get("dict"))
	if err != nil {
		httpErr(w, 404, "%v", err)
		return
	}
	if !hasBrowseIndex(e.Path) {
		browseUnprepared(w, e)
		return
	}
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
	words, err := b.Page(offset, browsePageSize)
	if err != nil {
		st.line(browseStreamEvent{Type: "error", Error: err.Error()})
		return
	}
	if words == nil {
		words = []string{}
	}
	pages := (total + browsePageSize - 1) / browsePageSize
	page := offset/browsePageSize + 1
	st.line(browseStreamEvent{Type: "page", Page: &browseResp{Dict: e.ID, Name: meta.Name, Total: total,
		Page: page, Pages: pages, Size: browsePageSize, At: mark, Words: words, Alphabet: []dict.Letter{}}})

	emit := func(letters []dict.Letter, at, all int) error {
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
	page := bytes.ReplaceAll(browseHTML, []byte("{{PRESETS}}"), []byte(s.pagePresetLinks()))
	page = bytes.ReplaceAll(page, []byte("{{WORKSPACECSS}}"), []byte(assetTag(workspaceCSS)))
	page = bytes.ReplaceAll(page, []byte("{{LISTSEARCHJS}}"), []byte(assetTag(listSearchJS)))
	page = bytes.ReplaceAll(page, []byte("{{HEADWORDJS}}"), []byte(assetTag(headwordControlJS)))
	page = bytes.ReplaceAll(page, []byte("{{HEADWORDCSS}}"), []byte(assetTag(headwordControlCSS)))
	_, _ = w.Write([]byte(renderUI(string(page), s.reg.prefs.Language())))
}
