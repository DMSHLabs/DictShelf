// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/logx"
)

// Built-in style presets as toggleable layers, replacing the old Examples
// menu that pasted their text into the user's two stylesheets.
//
// The problem with pasting was never the applying, it was the UNapplying:
// "Compact" arrived as fifteen lines at the bottom of App and Article, and
// turning it off meant hand-deleting text out of two boxes the reader may
// not have written. So a preset is now a file pair the user never edits,
// attached and detached as whole layers: the page composes
//
//	enabled presets, in manifest order  →  the user's own app/article
//
// and "disable" removes a layer instead of asking somebody to find its
// lines. Presets stay read-only on purpose: customising one is a button in
// the pane ("insert as text") that copies it into the user's own boxes,
// where the old model already works.
//
// The layout on disk IS the conflict model, and it is the user's, not ours:
// each subdirectory of web/presets/ holds ONE group of presets that exclude
// each other (background/ holds the four looks that all claim the same
// surfaces - background image, sepia, true black, warm dark), so enabling
// one switches the others off, radio-style. A preset that conflicts with
// nothing lives in a directory of its own and behaves as a plain toggle.
// manifest.json carries what the file names cannot: display order, titles,
// the one-line description the pane shows, and which half each file is.

//go:embed web/presets/manifest.json
var presetManifest []byte

//go:embed web/presets
var presetFS embed.FS

type preset struct {
	ID, Title, Desc string
	// Dir names the conflict group this preset belongs to.
	Dir string
	// RequiresImage hides the preset until the shell has a background image
	// active - it has nothing to show otherwise, exactly like the old menu
	// item it replaces.
	RequiresImage bool
	// App / Article name the DAY files inside Dir; AppNight / ArticleNight the
	// NIGHT ones; "" when the preset has no half for that scope.
	//
	// WHICH THEME A HALF BELONGS TO IS THE FILE'S NAME, not a selector inside
	// it. The theme is decided by which file is linked, so there is no guard to
	// forget and no `data-theme` to get wrong - and that mistake is what this
	// replaced: a guard reading "the reader pinned dark by hand" while the page
	// was dark under Auto, which let a light preset's inks onto a dark page.
	//
	// A preset with a day half and no night half applies by day and does
	// NOTHING at night. One with both applies in both, each half in its own
	// theme. A preset that belongs to neither theme names the SAME file in
	// both slots, so "works in both themes" is written down rather than
	// implied.
	App, Article           string
	AppNight, ArticleNight string

	// Pages says this preset's app half belongs on the three standalone
	// documents too - Edit Folders, Lemmatization and Browse - which are
	// documents the SERVER serves and which have no layer machinery of their
	// own: the server attaches the half as a <link> there (pagePresetLinks).
	//
	// It is for a preset that speaks in TOKENS rather than in rules about the
	// app's markup (Quiet labels is the first): the app's own classes do not
	// exist on those pages, so a half that restyles them would land on nothing.
	// A page preset must name ONE file for both themes (a test enforces it),
	// because the pages resolve their theme themselves - the OS preference, or
	// a pinned theme read from localStorage - and have none of the machinery
	// that switches a day half for a night one on the app page.
	Pages bool

	// Paper / PaperNight name the colour this preset paints the app's OWN
	// paper with, as "#rrggbb" - the --bg its app half declares, written down
	// beside the file name it lives in. It is a second statement of something
	// the CSS already says, and that is deliberate: the app's paper is not
	// only the page's business. Three of the app's windows are whole DOCUMENTS
	// the shell hosts (Edit Folders, Lemmatization, Browse) and the dictionary
	// list, the search-mode list and the popup are native windows - none of
	// them reads a CSS file, so the colour has to travel to the host as a
	// value. "" means the preset leaves the app's own background alone, which
	// is why only the four presets that repaint --bg declare one; a test
	// compares each declaration with the file, so the two cannot drift.
	Paper, PaperNight string

	appCSS, articleCSS           []byte
	appTag, articleTag           string
	appURL, articleURL           string
	appNightCSS, articleNightCSS []byte
	appNightTag, articleNightTag string
	appNightURL, articleNightURL string
}

// competes is the radio rule's question: do these two presets claim the same
// surface? Inside a group it is the THEME that decides, and the test is
// whether the two overlap - a day preset and a night one never compete, since
// each applies in its own theme, while a preset that covers both competes
// with either. Equality was the old test and it was too weak: "background"
// and "sepia" could both be on, and both of them paint the paper.
func (p *preset) competes(o *preset) bool {
	if p.Dir != o.Dir {
		return false
	}
	day := func(x *preset) bool { return x.appCSS != nil || x.articleCSS != nil }
	night := func(x *preset) bool { return x.appNightCSS != nil || x.articleNightCSS != nil }
	return (day(p) && day(o)) || (night(p) && night(o))
}

// Theme is DERIVED from the slots, never declared. A manifest that says one
// thing while its files do another is the class of bug this replaced:
// high_contrast was declared for both themes and guarded to the light one, so
// the pane offered it at night and it did nothing there.
func (p *preset) Theme() string {
	day := p.appCSS != nil || p.articleCSS != nil
	night := p.appNightCSS != nil || p.articleNightCSS != nil
	switch {
	case day && night:
		return "both"
	case night:
		return "dark"
	case day:
		return "light"
	}
	return ""
}

type presetGroup struct {
	Dir, Title string
	Presets    []*preset
}

var (
	presetOnce   sync.Once
	presetGroups []*presetGroup
	presetIndex  map[string]*preset
)

// presetRegistry parses the manifest and reads every referenced file, once.
// A broken or missing entry is skipped with a warning rather than taking the
// server down: the worst case is a preset missing from the pane, which is
// where every other soft failure in the styling surface lands too.
func presetRegistry() ([]*presetGroup, map[string]*preset) {
	presetOnce.Do(func() {
		var man struct {
			Groups []struct {
				Dir, Title string
				Presets    []struct {
					ID, Title, Desc        string
					RequiresImage          bool
					Pages                  bool
					App, Article           string
					AppNight, ArticleNight string
					Paper, PaperNight      string
				}
			} `json:"groups"`
		}
		if err := json.Unmarshal(presetManifest, &man); err != nil {
			logx.Warn("preset manifest unreadable, the presets pane will be empty: %v", err)
			presetIndex = map[string]*preset{}
			return
		}
		presetIndex = map[string]*preset{}
		for _, g := range man.Groups {
			group := &presetGroup{Dir: g.Dir, Title: g.Title}
			for _, p := range g.Presets {
				preset := &preset{
					ID: p.ID, Title: p.Title, Desc: p.Desc,
					Dir: g.Dir, RequiresImage: p.RequiresImage, Pages: p.Pages,
					App: p.App, Article: p.Article,
					AppNight: p.AppNight, ArticleNight: p.ArticleNight,
					Paper:      presetPaper(p.ID, p.Paper),
					PaperNight: presetPaper(p.ID, p.PaperNight),
				}
				load := func(name string) ([]byte, string, string) {
					if name == "" {
						return nil, "", ""
					}
					key := path.Join("web", "presets", g.Dir, name)
					b, err := presetFS.ReadFile(key)
					if err != nil {
						logx.Warn("preset %s: %s unreadable, the half is skipped: %v", p.ID, name, err)
						return nil, "", ""
					}
					tag := assetTag(b)
					return b, tag, "/assets/presets/" + g.Dir + "/" + name + "?v=" + tag
				}
				preset.appCSS, preset.appTag, preset.appURL = load(preset.App)
				preset.articleCSS, preset.articleTag, preset.articleURL = load(preset.Article)
				preset.appNightCSS, preset.appNightTag, preset.appNightURL = load(preset.AppNight)
				preset.articleNightCSS, preset.articleNightTag, preset.articleNightURL = load(preset.ArticleNight)
				if preset.appCSS == nil && preset.articleCSS == nil &&
					preset.appNightCSS == nil && preset.articleNightCSS == nil {
					continue
				}
				group.Presets = append(group.Presets, preset)
				presetIndex[preset.ID] = preset
			}
			if len(group.Presets) > 0 {
				presetGroups = append(presetGroups, group)
			}
		}
	})
	return presetGroups, presetIndex
}

// presetPaper normalizes a declared paper into the one spelling the shell
// validates: "#rrggbb", lower case, with the three-digit shorthand expanded.
// Anything else is dropped with a warning rather than passed on - a colour the
// shell cannot parse is a window that keeps the app's own background, which is
// the state an install has before any preset is switched on.
func presetPaper(id, raw string) string {
	s := strings.TrimPrefix(strings.TrimSpace(raw), "#")
	switch len(s) {
	case 3:
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	case 6:
	default:
		s = ""
	}
	if s == "" || strings.Trim(s, "0123456789abcdefABCDEF") != "" {
		if strings.TrimSpace(raw) != "" {
			logx.Warn("preset %s: paper %q is not a colour, ignored", id, raw)
		}
		return ""
	}
	return "#" + strings.ToLower(s)
}

// presetPaperSet is the paper the ENABLED layers paint, one value per theme -
// what the page hands the shell so the windows the page does not own wear it
// too (preset.Paper's comment says which those are).
//
// Manifest order decides when two presets declare one for the same theme, and
// the LAST declarer wins. That is not a new rule: it is the order the page
// attaches the layers in, so it is what CSS itself does with two :root blocks,
// and the page's --bg - the thing this value has to match - follows it.
// Presets of different groups can both be on (the radio rule is per group), so
// the case is reachable: sepia and high_contrast both paint the light paper,
// and high_contrast, which comes later in the manifest, is what the page shows.
func presetPaperSet(groups []*presetGroup, enabled []string) map[string]string {
	on := make(map[string]bool, len(enabled))
	for _, id := range enabled {
		on[id] = true
	}
	out := map[string]string{"light": "", "dark": ""}
	for _, g := range groups {
		for _, p := range g.Presets {
			if !on[p.ID] {
				continue
			}
			if p.Paper != "" {
				out["light"] = p.Paper
			}
			if p.PaperNight != "" {
				out["dark"] = p.PaperNight
			}
		}
	}
	return out
}

// presetStatePath is where the enabled list lives: beside the stylesheets it
// composes with, in the folder that is backed up as one piece (D32). Absent
// StyleDir means the feature has nowhere to remember anything, and the pane
// says so instead of failing silently.
func (s *Server) presetStatePath() string {
	if s.StyleDir == "" {
		return ""
	}
	return filepath.Join(s.StyleDir, "presets.json")
}

// presetEnabled reads the enabled list. An unknown id (a preset this build
// no longer ships) is dropped rather than shown; an unreadable or absent
// file is "none enabled", which is where everyone starts.
func (s *Server) presetEnabled() []string {
	p := s.presetStatePath()
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var st struct {
		Enabled []string `json:"enabled"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		logx.Warn("preset state unreadable, treating as none enabled: %v", err)
		return nil
	}
	_, index := presetRegistry()
	out := make([]string, 0, len(st.Enabled))
	for _, id := range st.Enabled {
		if _, ok := index[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// presetStateWrite replaces the enabled list, temp-file + rename like every
// other file this surface writes: a crash mid-save must not lose the reader's
// whole preset selection.
func (s *Server) presetStateWrite(enabled []string) error {
	p := s.presetStatePath()
	if p == "" {
		return os.ErrPermission
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(map[string]any{"enabled": enabled})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".presets-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// presetLinksHTML is what the page template gets for the app half: one
// <link> per enabled preset, in manifest order, each stamped with its own
// content hash - the same content addressing every other asset wears. The
// server cannot know the user link's hash here (it is per request), so the
// caller splices these BEFORE it: presets lose every tie to the user's own
// sheet, which is the whole point of the layering.
// pagePresetLinks is what the three standalone documents are sent: the app
// halves of the enabled presets that asked for them (preset.Pages), and
// nothing else. The app page's own links cannot be reused here — those halves
// restyle the app's markup, which these pages do not have — and the pages have
// no script of their own to enable one skin or another, which is why a page
// preset names ONE file for both themes (a test says so). Attached in manifest
// order, so a later half wins the same ties it would win on the app page.
func (s *Server) pagePresetLinks() string {
	enabled := s.presetEnabled()
	if len(enabled) == 0 {
		return ""
	}
	on := make(map[string]bool, len(enabled))
	for _, id := range enabled {
		on[id] = true
	}
	var b bytes.Buffer
	groups, _ := presetRegistry()
	for _, g := range groups {
		for _, p := range g.Presets {
			if !on[p.ID] || !p.Pages || p.App == "" || p.App != p.AppNight || p.appCSS == nil {
				continue
			}
			b.WriteString(`<link rel="stylesheet" data-preset="` + p.ID + `" href="` + p.appURL + `">` + "\n")
		}
	}
	return b.String()
}

func (s *Server) presetLinksHTML() string {
	enabled := s.presetEnabled()
	if len(enabled) == 0 {
		return ""
	}
	on := make(map[string]bool, len(enabled))
	for _, id := range enabled {
		on[id] = true
	}
	var b bytes.Buffer
	groups, _ := presetRegistry()
	for _, g := range groups {
		for _, p := range g.Presets {
			if !on[p.ID] {
				continue
			}
			// Both halves of a paired preset, each marked with the theme it
			// belongs to. The server does not know which theme this page will
			// resolve to - that is localStorage's business - so it writes both
			// and the page enables the one that applies, exactly as the
			// reader's own two sheets are linked.
			for _, half := range []struct {
				css  []byte
				url  string
				attr string
			}{
				{p.appCSS, p.appURL, ""},
				{p.appNightCSS, p.appNightURL, " data-night"},
			} {
				if half.css == nil {
					continue
				}
				b.WriteString(`<link rel="stylesheet" data-preset="`)
				b.WriteString(p.ID)
				b.WriteString(`"`)
				b.WriteString(half.attr)
				b.WriteString(` href="`)
				b.WriteString(half.url)
				b.WriteString(`">` + "\n")
			}
		}
	}
	return b.String()
}

// presetPayload is everything the pane and the composer need: the full list
// with each preset's contents inlined (they are small and static, and the
// article layer is assembled on the page, which then never fetches preset
// files itself), plus the enabled list so the pane can render its switches
// from one response.
func (s *Server) presetPayload() map[string]any {
	groups, _ := presetRegistry()
	enabled := s.presetEnabled()
	on := make(map[string]bool, len(enabled))
	for _, id := range enabled {
		on[id] = true
	}
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		presets := make([]map[string]any, 0, len(g.Presets))
		for _, p := range g.Presets {
			row := map[string]any{
				"id":            p.ID,
				"title":         p.Title,
				"desc":          p.Desc,
				"requiresImage": p.RequiresImage,
				"theme":         p.Theme(),
				"enabled":       on[p.ID],
			}
			// Every half the preset has, each with the URL the page needs to
			// attach it and the text it needs to compose the article layer.
			for key, half := range map[string]struct {
				css []byte
				url string
			}{
				"app":          {p.appCSS, p.appURL},
				"article":      {p.articleCSS, p.articleURL},
				"appNight":     {p.appNightCSS, p.appNightURL},
				"articleNight": {p.articleNightCSS, p.articleNightURL},
			} {
				if half.css != nil {
					row[key] = map[string]string{"url": half.url, "css": string(half.css)}
				}
			}
			presets = append(presets, row)
		}
		out = append(out, map[string]any{"dir": g.Dir, "title": g.Title, "presets": presets})
	}
	return map[string]any{
		"writable": s.StyleDir != "",
		"enabled":  enabled,
		"groups":   out,
		// The paper the ENABLED set paints, per theme, resolved here rather
		// than summed up by the page: the manifest order is the server's, and
		// so is the rule that reads it. The page's only job is to hand it to
		// the shell (presetPaperPush), which is what makes the standalone pages
		// and the native windows wear the preset the app page is wearing.
		"paper": presetPaperSet(groups, enabled),
	}
}

// GET /api/presets - the pane's whole world: every preset with its contents,
// grouped as the manifest groups them, plus what is currently enabled.
func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.presetPayload())
}

// PUT /api/presets - flip one switch. The body is {id, on}; the radio rule
// is the server's to enforce, because it is the one rule: switching a preset
// on inside a conflict group switches its group-mates off, so the state file
// can never hold two presets that claim the same surface. The fresh payload
// comes back, the way /api/style answers its own PUT.
func (s *Server) handlePresetSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
		On bool   `json:"on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if s.StyleDir == "" {
		http.Error(w, "no config directory: there is nowhere to remember preset choices", http.StatusConflict)
		return
	}
	_, index := presetRegistry()
	p, ok := index[req.ID]
	if !ok {
		http.Error(w, "unknown preset: "+req.ID, http.StatusBadRequest)
		return
	}
	enabled := s.presetEnabled()
	if req.On {
		// Radio within the group AND the theme: only presets competing for
		// the same surface go off. A group-mate in the OTHER theme is not a
		// competitor - each applies in its own theme, by its own CSS - so a
		// light and a dark preset can be on together, which is the whole point
		// of naming a theme on a preset. Groups of one, and presets in no
		// theme at all, simply never find a mate to switch off.
		next := make([]string, 0, len(enabled)+1)
		for _, id := range enabled {
			if other, ok := index[id]; ok && other.competes(p) {
				continue
			}
			next = append(next, id)
		}
		enabled = append(next, req.ID)
	} else {
		var kept []string
		for _, id := range enabled {
			if id != req.ID {
				kept = append(kept, id)
			}
		}
		enabled = kept
	}
	if err := s.presetStateWrite(enabled); err != nil {
		http.Error(w, "could not save preset choices: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.presetPayload())
}

// GET /assets/presets/<group>/<file> - the app half of an enabled preset
// needs a real URL for its <link>. Immutable caching is safe here, unlike
// /style/: these bytes are embedded in the binary, and the URL carries their
// content hash, so an app update changes the URL and never serves a stale
// sheet. manifest.json is state, not a stylesheet, and answers 404.
func (s *Server) handlePresetFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/assets/presets/")
	// Backslashes and dot-dot are rejected, not cleaned: an embedded FS is
	// always forward-slash, and a name carrying either is a second spelling
	// of something this route must not answer. (ContainsAny with "..\\" here
	// would reject every file - the dot in the extension is a member of that
	// set - which is why this is two Contains checks.)
	if name == "" || name == "manifest.json" ||
		strings.Contains(name, "..") || strings.Contains(name, `\`) ||
		strings.Contains(name, "//") {
		http.NotFound(w, r)
		return
	}
	b, err := presetFS.ReadFile(path.Join("web", "presets", name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	_, _ = w.Write(b)
}
