// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/lang"
	"github.com/wuweidict/wudict/internal/lemmas"
	"github.com/wuweidict/wudict/internal/logx"
)

// The lemma installer over HTTP (D91). `wudict lemmas` (D88) answers this for
// anyone with a terminal; Android is a WebView over an exec'd binary and has
// none, so on the platform where a 2 MB download matters most this is the only
// way to obtain one. The same page serves a desktop user who never opens a
// shell.
//
// Everything below is plumbing: internal/lemmas already fetches, validates,
// verifies and installs, and morph.Cache.Rescan already makes an installed
// language searchable without a restart. What this file adds is the state a
// GUI needs and a CLI does not - which language is being downloaded right now,
// and how far along it is.

// lemmaJob is one install in flight, or the error the last one ended with.
// Downloads run in the SERVER, not in the request that started them: a
// request-scoped download dies with the page, and the page dies whenever
// Android rotates the device or backgrounds the app (D64). Held here, a
// download survives a reload and the reopened page shows it still running.
type lemmaJob struct {
	State string `json:"state"` // "downloading" or "error"
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Err   string `json:"error,omitempty"`
}

// lemmaState is the installer's caches; its downloads are jobs (jobs.go). Two
// locks, deliberately: a catalogue fetch holds catMu for as long as the
// network takes, and the digest cache must not queue behind it.
type lemmaState struct {
	mu sync.Mutex
	// hashes memoizes lemmas.Hash keyed by path, size and mtime. Without it a
	// polling page would re-read every installed file - up to 21 MB - once per
	// second to decide whether to draw one "differs from the catalogue" mark.
	hashes map[string]string

	catMu  sync.Mutex
	cat    *lemmas.Catalog
	catErr string
	catAt  time.Time
}

const (
	// catalogTTL bounds how often a running server asks the catalogue host
	// anything. The page polls while a download runs; without this that poll
	// would be a request to GitHub every 800 ms.
	catalogTTL = 10 * time.Minute
	// catalogErrTTL is the same bound for a FAILED fetch, kept short because
	// the usual cause is a network that is about to come back. Long enough
	// that an offline poll loop is not a retry loop.
	catalogErrTTL = 30 * time.Second
	// installTimeout is the outer bound on one download. internal/lemmas sets
	// dial and response-header timeouts; neither bounds a connection that
	// accepts bytes forever at one per minute.
	installTimeout = 30 * time.Minute
)

// catalog returns the cached manifest, fetching it when the cache is cold,
// stale, or force is set. It returns (nil, message) when the fetch failed:
// callers report that, they do not treat it as fatal, because what is already
// installed is the half of the answer that needs no network.
func (s *Server) catalog(ctx context.Context, force bool) (*lemmas.Catalog, string) {
	st := &s.lemmas
	st.catMu.Lock()
	defer st.catMu.Unlock()

	ttl := catalogTTL
	if st.cat == nil {
		ttl = catalogErrTTL
	}
	if !force && !st.catAt.IsZero() && time.Since(st.catAt) < ttl {
		return st.cat, st.catErr
	}
	cat, err := lemmas.Fetch(ctx, s.LemmaURL)
	st.catAt = time.Now()
	if err != nil {
		// The previous catalogue, if there is one, stays: it is still true
		// enough to install from, and blanking the list because one poll
		// failed would be worse than showing it.
		st.catErr = err.Error()
		return st.cat, st.catErr
	}
	st.cat, st.catErr = cat, ""
	return cat, ""
}

// lemmaRow is one language as the page draws it: what the catalogue offers,
// what is on disk, and what is happening to it right now.
type lemmaRow struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`      // download size from the catalogue
	Local   int64  `json:"localSize"` // bytes on disk, 0 when not installed
	Lemmas  int64  `json:"lemmas"`
	HeapMB  int    `json:"heapMB"`
	Source  string `json:"source"`
	License string `json:"license"`

	Installed  bool `json:"installed"`
	Catalogued bool `json:"catalogued"`
	// Builtin marks the one language compiled into the binary (D87). It is
	// never "not installed", and removing a downloaded file for it falls back
	// to the built-in pack rather than losing the language.
	Builtin bool `json:"builtin"`
	// Mismatch is the CLI's [!]: a file is installed but its digest is not the
	// catalogue's. Not an error - a hand-written file is a legitimate thing to
	// have - so it is reported, never corrected.
	Mismatch bool `json:"mismatch"`

	State string `json:"state,omitempty"`
	// Done and Total are NOT omitempty: a download that has just started has
	// done 0, and omitting the field leaves the page dividing by undefined.
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error,omitempty"`
}

type lemmaInfo struct {
	Dir string `json:"dir"`
	// Enabled is MORPH_CACHE > 0. Said out loud because it is the one state in
	// which every download on this page is inert: the search path never asks
	// for a lemma, so nothing installed here would ever be read.
	Enabled     bool       `json:"enabled"`
	CacheSize   int        `json:"cacheSize"`
	URL         string     `json:"url"`
	Attribution string     `json:"attribution,omitempty"`
	Error       string     `json:"error,omitempty"`
	Shadowed    []string   `json:"shadowed,omitempty"`
	Languages   []lemmaRow `json:"languages"`
}

func (s *Server) handleLemmas(w http.ResponseWriter, r *http.Request) {
	refresh, _ := queryFlag(r.URL.Query(), "refresh")
	cat, cerr := s.catalog(r.Context(), refresh)
	local, shadowed := lemmas.Installed(s.LemmaDir)

	info := lemmaInfo{
		Dir: s.LemmaDir, Enabled: s.Morph.Enabled(), CacheSize: s.Morph.Max(),
		URL: s.LemmaURL, Attribution: attributionURL(s.LemmaURL),
		Error: cerr, Shadowed: shadowed, Languages: []lemmaRow{},
	}

	seen := map[string]bool{}
	if cat != nil {
		for _, e := range cat.Languages {
			seen[e.Code] = true
			info.Languages = append(info.Languages, s.lemmaRow(e, local))
		}
	}
	// On disk but not in the catalogue: a file the user wrote, or a language
	// dropped from the catalogue after it was installed. It still works, so it
	// is shown as working - and it is shown at all, which is the point: this
	// page is the only place a GUI user can see that folder's contents.
	for code, l := range local {
		if seen[code] {
			continue
		}
		info.Languages = append(info.Languages, lemmaRow{
			Code: code, Name: lang.Name(code), Local: l.Size,
			Installed: true, Builtin: code == "en",
		})
	}
	if !seen["en"] && local["en"].Path == "" {
		info.Languages = append(info.Languages, lemmaRow{
			Code: "en", Name: "English", Installed: true, Builtin: true,
		})
	}
	// Sorted by name: the page is a list a human reads, and code order would
	// put Asturian before English for no reason a reader can see.
	sort.Slice(info.Languages, func(i, j int) bool {
		return info.Languages[i].Name < info.Languages[j].Name
	})
	for i := range info.Languages {
		if j := s.job(info.Languages[i].Code); j != nil {
			info.Languages[i].State, info.Languages[i].Done = j.State, j.Done
			info.Languages[i].Total, info.Languages[i].Error = j.Total, j.Err
		}
	}
	writeJSON(w, info)
}

// lemmaRow merges one catalogue entry with what is on disk. Same rules as
// `wudict lemmas list` (catalogRow in internal/cli), because two views of one
// folder disagreeing would be a bug report nobody could reproduce.
func (s *Server) lemmaRow(e lemmas.Entry, local map[string]lemmas.Local) lemmaRow {
	r := lemmaRow{
		Code: e.Code, Name: e.Name, Size: e.Size, Lemmas: e.Lemmas,
		HeapMB: e.HeapMB, Source: e.Source, License: e.License,
		Catalogued: true, Builtin: e.Code == "en",
	}
	l, ok := local[e.Code]
	if !ok {
		r.Installed = e.Code == "en" // built in, nothing to download
		return r
	}
	r.Installed, r.Local = true, l.Size
	if h, err := s.lemmaHash(l); err != nil || h != e.SHA256 {
		r.Mismatch = true
	}
	return r
}

// lemmaHash is lemmas.Hash memoized on the file's identity. A file that has
// been rewritten has a different size or mtime and is read again.
func (s *Server) lemmaHash(l lemmas.Local) (string, error) {
	fi, err := os.Stat(l.Path)
	if err != nil {
		return "", err
	}
	key := l.Path + "|" + fi.ModTime().UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(fi.Size(), 10)

	st := &s.lemmas
	st.mu.Lock()
	h, ok := st.hashes[key]
	st.mu.Unlock()
	if ok {
		return h, nil
	}
	h, err = lemmas.Hash(l.Path)
	if err != nil {
		return "", err
	}
	st.mu.Lock()
	if st.hashes == nil {
		st.hashes = map[string]string{}
	}
	// Bounded by the number of files in one folder, which is bounded by the
	// number of languages; a rewritten file adds a key and the old one is
	// dropped rather than accumulating.
	for k := range st.hashes {
		if strings.HasPrefix(k, l.Path+"|") {
			delete(st.hashes, k)
		}
	}
	st.hashes[key] = h
	st.mu.Unlock()
	return h, nil
}

// lemmaKey is a download's job (jobs.go): one per language.
func lemmaKey(code string) string { return "lemma:" + code }

// job is the download of code as the page shows it, or nil: a download that
// succeeded is not a job any more, only an installed file.
func (s *Server) job(code string) *lemmaJob {
	st, ok := s.jobs.status(lemmaKey(code))
	if !ok {
		return nil
	}
	j := &lemmaJob{State: "downloading", Done: st.Done, Total: st.Total, Err: st.Err}
	if !st.Running {
		j.State = "error"
	}
	return j
}

func (s *Server) handleLemmaInstall(w http.ResponseWriter, r *http.Request) {
	code := lang.Normalize(r.URL.Query().Get("code"))
	if code == "" {
		httpErr(w, 400, "not a language: %q", r.URL.Query().Get("code"))
		return
	}
	if s.LemmaDir == "" {
		httpErr(w, 500, "no lemma folder configured (LEMMA_DIR)")
		return
	}
	cat, cerr := s.catalog(r.Context(), false)
	if cat == nil {
		httpErr(w, 502, "could not read the catalogue: %s", cerr)
		return
	}
	e, ok := cat.Find(code)
	if !ok {
		httpErr(w, 400, "the catalogue has no lemma data for %s", code)
		return
	}
	// Already running is the same answer as starting it: the caller wanted
	// this language downloaded and it is being downloaded. A FAILED job is
	// replaced: asking again is how a user retries, and the old error must
	// not outlive the retry.
	s.jobs.start(lemmaKey(code), installTimeout, jobStatus{Total: e.Size}, func(j *job) { s.install(j, cat, e) })

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"code": code, "state": "downloading"})
}

// install runs one download to completion, outliving the request that asked
// for it. On success the job is DELETED rather than marked done: the file is
// then simply installed, which is a fact about the folder and not a fact about
// this process, and a page loaded tomorrow must not be told about a download.
func (s *Server) install(j *job, cat *lemmas.Catalog, e lemmas.Entry) {
	dst, err := cat.Install(j.ctx, s.LemmaDir, e, func(done int64) {
		j.update(func(st *jobStatus) { st.Done = done })
	})
	if err != nil {
		j.update(func(st *jobStatus) { st.Err = err.Error() })
		logx.Warn("lemma download failed: %v", err)
		return
	}
	j.dropWhenDone()
	// The whole reason this can be done from a page rather than from a restart
	// (D88 left Rescan here for exactly this).
	s.Morph.Rescan()
	logx.V("installed lemma data for %s: %s", e.Code, dst)
}

func (s *Server) handleLemmaRemove(w http.ResponseWriter, r *http.Request) {
	code := lang.Normalize(r.URL.Query().Get("code"))
	if code == "" {
		httpErr(w, 400, "not a language: %q", r.URL.Query().Get("code"))
		return
	}
	if j := s.job(code); j != nil && j.State == "downloading" {
		httpErr(w, 409, "%s is being downloaded", code)
		return
	}
	gone, err := lemmas.Remove(s.LemmaDir, code)
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	// A failed job for this language is cleared too: the user has just said
	// they do not want it, and an error message about a download they have
	// abandoned is noise.
	s.jobs.forget(lemmaKey(code))

	s.Morph.Rescan()
	writeJSON(w, map[string]any{"code": code, "removed": gone, "builtin": code == "en"})
}

// attributionURL points at the ATTRIBUTION.txt published beside the manifest.
// The data is ODbL: redistributing it obliges attribution, and the page that
// installs it is where a user will look for the licence. Empty for a local
// catalogue - the folder is already in front of them - and empty rather than
// wrong if the URL cannot be parsed.
func attributionURL(u string) string {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return ""
	}
	p, err := url.Parse(u)
	if err != nil || p.Path == "" {
		return ""
	}
	p.RawQuery, p.Fragment = "", ""
	p.Path = path.Join(path.Dir(p.Path), "ATTRIBUTION.txt")
	return p.String()
}
