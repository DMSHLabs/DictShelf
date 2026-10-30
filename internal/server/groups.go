// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DMSHLabs/DictShelf/internal/facet"
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
	writeJSON(w, s.groupsSnapshot())
}

// PUT /api/groups - replace the file with the request body. What does not
// parse is reported, never refused: the rest of the file still applies, and
// the editor shows what was skipped. A body that IS the default removes the
// file instead, so an unchanged copy does not stop the list from following
// the built-in one. A file that exists but cannot be used is the user's and
// is not replaced unless the request says so (?replace=1): the editor showed
// an empty box for it, and a save from there would otherwise destroy it.
func (s *Server) handleSaveGroups(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxGroupsBytes))
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			httpErr(w, http.StatusRequestEntityTooLarge, "%s", GroupsFileName+" is larger than 256 KB")
		} else {
			httpErr(w, http.StatusBadRequest, "%s", "reading the request: "+err.Error())
		}
		return
	}
	result, err := s.saveGroups(body, r.URL.Query().Get("replace") == "1")
	if err != nil {
		var operation groupsFileError
		if errors.As(err, &operation) {
			httpErr(w, operation.status, "%s", operation.message)
		} else {
			httpErr(w, http.StatusInternalServerError, "%s", "could not save "+GroupsFileName+": "+err.Error())
		}
		return
	}
	writeJSON(w, result)
}

// DELETE /api/groups - back to the embedded default.
func (s *Server) handleResetGroups(w http.ResponseWriter, r *http.Request) {
	result, err := s.resetGroups()
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "%s", "could not remove "+GroupsFileName+": "+err.Error())
		return
	}
	writeJSON(w, result)
}

type groupsFileError struct {
	status  int
	message string
}

func (e groupsFileError) Error() string { return e.message }

func (s *Server) saveGroups(body []byte, replace bool) (map[string]any, error) {
	if s.User.Groups() == "" {
		return nil, groupsFileError{status: http.StatusConflict, message: "no config directory: there is nowhere to save groups"}
	}
	if len(body) > maxGroupsBytes {
		return nil, groupsFileError{status: http.StatusRequestEntityTooLarge, message: GroupsFileName + " is larger than 256 KB"}
	}
	if !utf8.Valid(body) {
		return nil, groupsFileError{status: http.StatusBadRequest, message: "not UTF-8 text"}
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	var st fileState[*facet.Rules]
	var err error
	if text == facet.DefaultText {
		st, err = s.groups.remove()
	} else {
		st, err = s.groups.save(text, replace)
	}
	if errors.Is(err, errUnusable) {
		return nil, groupsFileError{status: http.StatusConflict, message: err.Error() + " - replace=1 overwrites it"}
	}
	if err != nil {
		return nil, fmt.Errorf("could not save %s: %w", GroupsFileName, err)
	}
	return groupsResponse(s.User.Groups(), st), nil
}

func (s *Server) resetGroups() (map[string]any, error) {
	st, err := s.groups.remove()
	if err != nil {
		return nil, fmt.Errorf("could not remove %s: %w", GroupsFileName, err)
	}
	return groupsResponse(s.User.Groups(), st), nil
}

// groupsPage is the editor as served. Like the lemma installer, nothing is
// baked in: the page asks /api/groups for everything it shows.
var groupsPage = bytes.ReplaceAll(groupsHTML, []byte("{{CSS}}"), []byte(cssTag))

func (s *Server) handleGroupsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Add("Vary", "Accept-Language")
	_, _ = w.Write(s.groupsPageHTMLFor(r.Header.Get("Accept-Language")))
}

func (s *Server) groupsPageHTML() []byte {
	return s.groupsPageHTMLFor("")
}

func (s *Server) groupsPageHTMLFor(acceptLanguage string) []byte {
	page := bytes.ReplaceAll(groupsPage, []byte("{{PRESETS}}"), []byte(s.pagePresetLinks()))
	page = bytes.ReplaceAll(page, []byte("{{WORKSPACECSS}}"), []byte(assetTag(workspaceCSS)))
	return s.renderUserUIChoice(string(page), s.reg.prefs.ResolvedLanguage(acceptLanguage), s.reg.prefs.Language())
}
