// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/facet"
)

// The dictionary picker's groups after language and language pair come from
// groups.ini beside the wudict.toml in effect (D162), edited on the /groups
// page or in any text editor. While there is no such file the embedded one is
// in effect, so a library that never customised anything follows the built-in
// list as it improves; the first save makes the user's copy the whole truth.

const (
	// GroupsFileName is the file, beside wudict.toml.
	GroupsFileName = "groups.ini"
	// A list of items, not a document: far above any real file, far below
	// anything that would cost a reader.
	maxGroupsBytes = 256 << 10
)

// groupsNow is groups.ini in effect (userfile.go): the rules, re-read at most
// once a second.
func (s *Server) groupsNow() fileState[*facet.Rules] { return s.groups.now() }

func (s *Server) initGroups() {
	s.groups = ownedFile[*facet.Rules]{
		// a closure, not the method value s.User.Groups: that would copy the
		// folder as it is now, before the CLI sets it
		name: GroupsFileName, path: func() string { return s.User.Groups() }, max: maxGroupsBytes, perm: 0o644,
		recheck: time.Second,
		parse:   facet.Parse, absent: facet.Default,
	}
}

// GET /api/groups - the file in effect, what it defines, and what was skipped.
func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	writeGroups(w, s.User.Groups(), s.groupsNow())
}

func writeGroups(w http.ResponseWriter, file string, st fileState[*facet.Rules]) {
	probs := st.problems
	if probs == nil {
		probs = []facet.Problem{}
	}
	text := st.text
	if !st.exists {
		text = facet.DefaultText // what is in effect, offered for editing
	}
	writeJSON(w, map[string]any{
		"text":     text,
		"custom":   st.exists,
		"unusable": st.unusable(),
		"file":     file,
		"writable": file != "",
		"problems": probs,
		"facets":   st.val.Facets(),
	})
}

// PUT /api/groups - replace the file with the request body. What does not
// parse is reported, never refused: the rest of the file still applies, and
// the editor shows what was skipped. A body that IS the default removes the
// file instead, so an unchanged copy does not stop the list from following
// the built-in one. A file that exists but cannot be used is the user's and
// is not replaced unless the request says so (?replace=1): the editor showed
// an empty box for it, and a save from there would otherwise destroy it.
func (s *Server) handleSaveGroups(w http.ResponseWriter, r *http.Request) {
	if s.User.Groups() == "" {
		httpErr(w, http.StatusConflict, "%s", "no config directory: there is nowhere to save groups")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxGroupsBytes))
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			httpErr(w, http.StatusRequestEntityTooLarge, "%s", GroupsFileName+" is larger than 256 KB")
		} else {
			httpErr(w, http.StatusBadRequest, "%s", "reading the request: "+err.Error())
		}
		return
	}
	if !utf8.Valid(body) {
		httpErr(w, http.StatusBadRequest, "%s", "not UTF-8 text")
		return
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	var st fileState[*facet.Rules]
	if text == facet.DefaultText {
		st, err = s.groups.remove()
	} else {
		st, err = s.groups.save(text, r.URL.Query().Get("replace") == "1")
	}
	switch {
	case errors.Is(err, errUnusable):
		httpErr(w, http.StatusConflict, "%s", err.Error()+" - replace=1 overwrites it")
		return
	case err != nil:
		httpErr(w, http.StatusInternalServerError, "%s", "could not save "+GroupsFileName+": "+err.Error())
		return
	}
	writeGroups(w, s.User.Groups(), st)
}

// DELETE /api/groups - back to the embedded default.
func (s *Server) handleResetGroups(w http.ResponseWriter, r *http.Request) {
	st, err := s.groups.remove()
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "%s", "could not remove "+GroupsFileName+": "+err.Error())
		return
	}
	writeGroups(w, s.User.Groups(), st)
}

// groupsPage is the editor as served. Like the lemma installer, nothing is
// baked in: the page asks /api/groups for everything it shows.
var groupsPage = bytes.ReplaceAll(groupsHTML, []byte("{{CSS}}"), []byte(cssTag))

func (s *Server) handleGroupsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	page := bytes.ReplaceAll(groupsPage, []byte("{{PRESETS}}"), []byte(s.pagePresetLinks()))
	_, _ = w.Write([]byte(renderUI(string(page), s.reg.prefs.Language())))
}
