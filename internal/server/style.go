// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/wuweidict/wudict/internal/facet"
)

// Global user styling: two CSS files the user owns, applied to everything.
//
// D59 already lets a user replace a file a dictionary ships
// (<library folder>/res/<name>). That is per-dictionary and whole-file, and it
// cannot express either of the two things people actually ask for: "this
// dictionary's desktop side padding wastes half my phone screen" (one rule,
// but res/ demands a copy of the whole stylesheet, which then goes stale) and
// "give me sepia" (the chrome is not a dictionary resource at all). So this is
// the complementary layer: ADDITIVE, GLOBAL, and ordered last so it wins.
//
// Two files rather than one, because a single box leaks in the damaging
// direction: body{}, p{}, a{}, table{} and div{} are exactly what someone
// styling articles writes, and every one of them would also hit the chrome.
// Two boxes make that structurally impossible, and the split is one sentence:
// app.css is colours and the app's own layout, article.css is what
// dictionaries render. They do not fragment the "sepia everywhere" case,
// because :root custom properties set in app.css already inherit into every
// shadow root and are carried into iframes by the existing bridge - so tokens
// are written once, in app.css, and reach all three surfaces.
//
// Files on disk beside the wudict.toml in effect, not a blob in state.json:
// CSS is multi-line and users will want a real editor, a diff and a backup,
// and the res/ precedent is already "a file on disk, served no-cache, an edit
// lands on reload". state.json is for settings; this is a document.
const (
	// StyleDirName is the folder, beside the config file in effect, that holds
	// the two files. The CLI resolves the parent (see userDir), the same way
	// it resolves StateFile, so a portable install (D32) keeps its styling on
	// the same stick as its config.
	StyleDirName = "style"

	appCSSName     = "app.css"
	articleCSSName = "article.css"

	// The night pair. A theme is a look, and the look is largely these two
	// files: one pair per theme means switching the theme switches what the
	// reader wrote FOR it, instead of a single sheet having to serve both -
	// which is what left a day sheet's paper colours on a dark page. The DAY
	// pair is the original two names, so an install that predates this keeps
	// editing exactly what it always did and needs no migration.
	appNightCSSName     = "app_night.css"
	articleNightCSSName = "article_night.css"

	// maxUserCSSBytes caps one file. Generous for hand-written CSS by three
	// orders of magnitude, and the only thing standing between a PUT and the
	// user's disk.
	maxUserCSSBytes = 256 << 10
)

// styleNames is the whole namespace, stated once. Membership is checked by
// equality against it rather than by cleaning a path: there is no traversal to
// defend against when the only reachable names are literals.
var styleNames = []string{appCSSName, articleCSSName, appNightCSSName, articleNightCSSName}

// styleNamesFor maps a theme to the pair it edits. The API speaks the page's
// vocabulary - light/dark, the RESOLVED theme - while the files are named
// day/night, which is what the reader sees them as. The mapping lives here and
// nowhere else, so a third theme would be one more line rather than a hunt.
func styleNamesFor(theme string) (app, article string) {
	if theme == "dark" {
		return appNightCSSName, articleNightCSSName
	}
	return appCSSName, articleCSSName
}

func isStyleName(n string) bool {
	for _, s := range styleNames {
		if s == n {
			return true
		}
	}
	return false
}

// stylePath is the file's location, or "" when there is nowhere to put it -
// which is the same "no config file and no home directory" case that makes
// state in-memory rather than scattering a file somewhere nobody asked for.
func (s *Server) stylePath(name string) string {
	if s.User.Style() == "" || !isStyleName(name) {
		return ""
	}
	return filepath.Join(s.User.Style(), name)
}

// styleCSS is one stylesheet as served: its bytes and their content tag,
// computed once per change rather than on every page load.
type styleCSS struct {
	body []byte
	tag  string // "" for no stylesheet
}

func parseCSS(text string) (styleCSS, []facet.Problem) {
	if text == "" {
		return styleCSS{}, nil
	}
	b := []byte(text)
	return styleCSS{body: b, tag: assetTag(b)}, nil
}

// initStyles makes each stylesheet an ownedFile (ownedfile.go): bounded,
// cached by content, written atomically, an unreadable one kept until the
// user says to replace it.
func (s *Server) initStyles() {
	s.styles = map[string]*ownedFile[styleCSS]{}
	for _, n := range styleNames {
		s.styles[n] = &ownedFile[styleCSS]{
			name: n, path: func() string { return s.stylePath(n) }, max: maxUserCSSBytes, perm: 0o644,
			parse: parseCSS, absent: func() styleCSS { return styleCSS{} },
		}
	}
}

// styleRead returns the file's bytes, or nil. It never fails: absent is the
// normal case, and an unreadable file must not stop the app from serving
// dictionaries - the worst outcome is unstyled, which is where everyone
// starts. Why it is unreadable is in the editor (handleStyle).
func (s *Server) styleRead(name string) []byte { return s.styles[name].now().val.body }

// styleWrite stores one stylesheet's text, or removes the file when the text
// is empty - the rule PUT /api/style applies, for the callers that are not
// that handler (the Appearance sheet's saved looks).
func (s *Server) styleWrite(name, body string) error {
	f, ok := s.styles[name]
	if !ok {
		return fmt.Errorf("unknown stylesheet %q", name)
	}
	if body == "" {
		_, err := f.remove()
		return err
	}
	_, err := f.save(body, true)
	return err
}

// appStyleTag is the content hash index.html stamps into its <link>, or "" when
// there is no stylesheet under that name. Same content addressing as
// assetTag's other callers (D45): the URL changes exactly when the bytes do.
// One name per call, because the day and night pairs are hashed separately -
// editing the day sheet must not invalidate the night one's cache entry. The
// hash is the one parseCSS computed, so a tag costs no re-read.
func (s *Server) appStyleTag(name string) string {
	if f, ok := s.styles[name]; ok {
		return f.now().val.tag
	}
	return ""
}

// styleOff reports the escape hatch. app.css can hide its own editor - one
// `*{display:none}` is enough - so there has to be a way back in that does not
// require finding a text editor on a phone. ?style=off omits the chrome
// stylesheet and tells the client to skip the article one.
func styleOff(r *http.Request) bool {
	return r.URL.Query().Get("style") == "off"
}

// GET /style/app.css, /style/article.css and their night pair.
//
// no-cache, exactly like a res/ override and for the same reason: this is a
// file the user is actively editing, and a cache that outlives their next
// attempt would look like the feature being broken. An absent file is 200 with
// an empty body rather than 404 - it is a stylesheet that says nothing, not a
// missing resource, and a 404 in the network panel would send someone hunting
// for a bug that is not there.
func (s *Server) handleUserCSS(w http.ResponseWriter, r *http.Request) {
	// The exact path, not filepath.Base: /style/anything/app.css must be a 404
	// and not a second URL for the same file. There are four names here, and
	// each of them has one address.
	name := strings.TrimPrefix(r.URL.Path, "/style/")
	if !isStyleName(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if styleOff(r) {
		return
	}
	_, _ = w.Write(s.styleRead(name))
}

// GET /api/style - all four files, plus where they live and whether they can
// be written. The path is reported because the editor shows it: an in-app
// editor that hides which file it is editing turns a plain text file into a
// black box, and the point of using files was that an external editor works
// too.
func (s *Server) handleStyle(w http.ResponseWriter, r *http.Request) {
	// a stylesheet that exists but is not applied (too large, not a file)
	// says why, so the editor's empty box is not mistaken for an empty file
	problems := map[string]string{}
	for _, n := range styleNames {
		if st := s.styles[n].now(); st.unusable() {
			problems[strings.TrimSuffix(n, ".css")] = st.why
		}
	}
	writeJSON(w, map[string]any{
		"app":          string(s.styleRead(appCSSName)),
		"article":      string(s.styleRead(articleCSSName)),
		"appNight":     string(s.styleRead(appNightCSSName)),
		"articleNight": string(s.styleRead(articleNightCSSName)),
		"dir":          s.User.Style(),
		"writable":     s.User.Style() != "",
		"problems":     problems,
	})
}

// PUT /api/style - replace either file of ONE theme's pair, or both.
//
// Pointers, not strings: absent and empty are different. The App tab and the
// Article tab are the same editor sending different bodies, and a request that
// omits one must never be read as clearing it - the same trap /api/prefs has
// with `ui`. `theme` names WHICH pair, and its absence is the day pair, which
// is what every request written before the night files existed meant.
func (s *Server) handleSaveStyle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App     *string `json:"app"`
		Article *string `json:"article"`
		Theme   string  `json:"theme"`
	}
	if !decodeJSON(w, r, &req, 4*maxUserCSSBytes+(1<<12)) {
		return
	}
	if s.User.Style() == "" {
		httpErr(w, http.StatusConflict, "%s", "no config directory: there is nowhere to save custom styles")
		return
	}
	appName, articleName := styleNamesFor(req.Theme)
	files := []struct {
		name string
		body *string
	}{{appName, req.App}, {articleName, req.Article}}
	replace := r.URL.Query().Get("replace") == "1"
	// Both checked before either is written: a request that saves one file
	// and is refused on the other would leave the editor out of step.
	for _, f := range files {
		if f.body == nil {
			continue
		}
		if len(*f.body) > maxUserCSSBytes {
			httpErr(w, http.StatusRequestEntityTooLarge, "%s", f.name+" is too large")
			return
		}
		if st := s.styles[f.name].fresh(); st.unusable() && !replace {
			httpErr(w, http.StatusConflict, "%s", st.why+" - replace=1 overwrites it")
			return
		}
	}
	for _, f := range files {
		if f.body == nil {
			continue
		}
		var err error
		if *f.body == "" {
			_, err = s.styles[f.name].remove()
		} else {
			_, err = s.styles[f.name].save(*f.body, true)
		}
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "%s", "could not save "+f.name+": "+err.Error())
			return
		}
	}
	s.handleStyle(w, r)
}
