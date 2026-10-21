// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/facet"
)

// The reader's two word lists beside wudict.toml: the headwords they looked
// up (history.txt) and the ones they starred (favourites.txt). Plain text, one
// headword per line, most recent first - a file to read, edit, diff or carry
// to another tool as it is. They describe the person, not the browser, so
// they live on the server and follow the reader to a phone on the LAN or a
// different port, as state.json does.
//
// A word is a headword, never a dictionary's article: opening one searches
// the way typing it does. Words are compared without case (sameWord), as the
// search they open compares them: "CASA" and "casa" are one entry, kept in
// the spelling last used.

const (
	HistoryFileName   = "history.txt"
	FavouritesFileName = "favourites.txt"

	// maxWordRunes drops what is not a headword: a pasted paragraph.
	maxWordRunes = 256
	// maxHistory is how many lookups are kept; the oldest goes first.
	maxHistory = 500
	// maxFavourites only bounds a hostile or runaway client; nobody stars
	// this many words by hand.
	maxFavourites = 10000
	// maxWordsBytes bounds either file: maxFavourites words of maxWordRunes
	// runes at 4 bytes is 10 MB; real lists are a few KB.
	maxWordsBytes = 12 << 20
)

// wordLists are the two files, and the lock that makes a change to one of
// them a whole read-modify-write.
type wordLists struct {
	history, favourites ownedFile[[]string]
	mu                 sync.Mutex
}

func (s *Server) initWords() {
	file := func(name string, path func(UserDir) string) ownedFile[[]string] {
		return ownedFile[[]string]{
			// a closure over s.User, as groups.ini is: the CLI sets the
			// folder after New
			name: name, path: func() string { return path(s.User) }, max: maxWordsBytes,
			recheck: time.Second, perm: 0o600, // what a person looked up is theirs alone
			parse:  parseWords,
			absent: func() []string { return nil },
		}
	}
	s.words.history = file(HistoryFileName, UserDir.History)
	s.words.favourites = file(FavouritesFileName, UserDir.Favourites)
}

// cleanWord is w as a stored line, or "" when it is not a headword: line
// breaks and tabs become spaces (a line is one word), the ends are trimmed,
// and invalid UTF-8 or more than maxWordRunes runes is refused.
func cleanWord(w string) string {
	if !utf8.ValidString(w) {
		return "" // checked first: strings.Map would turn a bad byte into U+FFFD
	}
	w = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, w))
	if utf8.RuneCountInString(w) > maxWordRunes {
		return ""
	}
	return w
}

// parseWords reads a list: one word per line, blank and invalid lines
// skipped, the first of two equal lines kept. It never fails - a hand edit
// that breaks a line costs that line.
func parseWords(text string) ([]string, []facet.Problem) {
	var out []string
	seen := map[string]bool{}
	for line := range strings.Lines(text) {
		if w := cleanWord(line); w != "" && !seen[strings.ToLower(w)] {
			seen[strings.ToLower(w)] = true
			out = append(out, w)
		}
	}
	return out, nil
}

// sameWord is the page's comparison too (index.html, wordKey): lower-cased,
// so a word typed in capitals is the entry already there.
func sameWord(a, b string) bool { return strings.ToLower(a) == strings.ToLower(b) }

// putWord is list with w moved to the front (on) or taken out (off), at most
// limit long, and whether that changed anything.
func putWord(list []string, w string, on bool, limit int) ([]string, bool) {
	out := make([]string, 0, len(list)+1)
	if on {
		out = append(out, w)
	}
	for _, x := range list {
		if !sameWord(x, w) {
			out = append(out, x)
		}
	}
	out = out[:min(len(out), limit)]
	return out, !slices.Equal(out, list)
}

func joinWords(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return strings.Join(list, "\n") + "\n"
}

// historyOff is the reader's "Keep history" switch, off. Read here as well
// as on the page, so a page loaded before the switch was turned off cannot
// go on recording.
func (s *Server) historyOff() bool {
	u := s.reg.prefs.UI()
	return u != nil && u.HistoryOff
}

func (s *Server) writeWords(w http.ResponseWriter) {
	list := func(f *ownedFile[[]string]) []string {
		if v := f.now().val; v != nil {
			return v
		}
		return []string{} // [] on the wire, never null
	}
	writeJSON(w, map[string]any{"favourites": list(&s.words.favourites), "history": list(&s.words.history)})
}

// GET /api/words - both lists, most recent first.
func (s *Server) handleWords(w http.ResponseWriter, r *http.Request) {
	s.writeWords(w)
}

// POST /api/words - put one word at the front of a list, or take it out,
// and answer both lists as they now stand. A change that changes nothing
// (the word already first) writes nothing.
func (s *Server) handleSaveWord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		List string `json:"list"` // "history" | "favourites"
		Word string `json:"word"`
		On   bool   `json:"on"`
	}
	if !decodeJSON(w, r, &req, 4<<10) {
		return
	}
	f, limit := &s.words.history, maxHistory
	switch req.List {
	case "history":
	case "favourites":
		f, limit = &s.words.favourites, maxFavourites
	default:
		httpErr(w, http.StatusBadRequest, "list must be history or favourites")
		return
	}
	word := cleanWord(req.Word)
	if word == "" {
		httpErr(w, http.StatusBadRequest, "not a word: empty, or longer than %d characters", maxWordRunes)
		return
	}
	if !(req.List == "history" && req.On && s.historyOff()) {
		s.words.mu.Lock()
		list, changed := putWord(f.fresh().val, word, req.On, limit)
		var err error
		if changed {
			_, err = f.save(joinWords(list), false)
		}
		s.words.mu.Unlock()
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "%s", "could not save: "+err.Error())
			return
		}
	}
	s.writeWords(w)
}

// DELETE /api/words?list=history - empty a list: its file is removed.
func (s *Server) handleClearWords(w http.ResponseWriter, r *http.Request) {
	f := &s.words.history
	switch r.URL.Query().Get("list") {
	case "history":
	case "favourites":
		f = &s.words.favourites
	default:
		httpErr(w, http.StatusBadRequest, "list must be history or favourites")
		return
	}
	s.words.mu.Lock()
	_, err := f.remove()
	s.words.mu.Unlock()
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "%s", "could not remove: "+err.Error())
		return
	}
	s.writeWords(w)
}
