// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

// The manifest says which theme a preset is for by WHICH SLOTS IT FILLS, and
// the pane, the radio rule and the page all read that. A manifest that also
// declares a theme is a second answer to the same question, and the two have
// disagreed before: high_contrast was declared for both themes and guarded to
// the light one, so the pane offered it at night and it did nothing there.
func TestPresetManifestDeclaresNoTheme(t *testing.T) {
	var man struct {
		Groups []struct {
			Presets []map[string]any `json:"presets"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(presetManifest, &man); err != nil {
		t.Fatal(err)
	}
	if len(man.Groups) == 0 {
		t.Fatal("no groups parsed out of the manifest")
	}
	for _, g := range man.Groups {
		for _, p := range g.Presets {
			if _, ok := p["theme"]; ok {
				t.Errorf("preset %v declares a theme; the slots decide it now", p["id"])
			}
		}
	}
}

// Every slot the manifest names must be a file that is there, and every preset
// must have a half for at least one theme. A name with no file is skipped with
// a warning at load, which leaves a switch that turns on nothing - the one
// failure this pipeline cannot report on its own.
func TestPresetSlotsAreReadable(t *testing.T) {
	groups, _ := presetRegistry()
	if len(groups) == 0 {
		t.Fatal("no presets at all")
	}
	for _, g := range groups {
		for _, p := range g.Presets {
			if p.Theme() == "" {
				t.Errorf("preset %s has no half for either theme", p.ID)
			}
			for _, name := range []string{p.App, p.Article, p.AppNight, p.ArticleNight} {
				if name == "" {
					continue
				}
				if _, err := presetFS.ReadFile(path.Join("web", "presets", p.Dir, name)); err != nil {
					t.Errorf("preset %s names %s, which is not there: %v", p.ID, name, err)
				}
			}
		}
	}
}

// A preset's paper is the --bg its app half paints, written down in the
// manifest as well because the windows the PAGE does not own (the three
// standalone pages, the dictionary list, the search-mode list, the popup)
// are painted by the shell, which reads no CSS. Two statements of one fact
// drift unless something compares them - this is that something.
func TestPresetPaperMatchesItsOwnHalf(t *testing.T) {
	groups, _ := presetRegistry()
	declared := 0
	for _, g := range groups {
		for _, p := range g.Presets {
			for _, half := range []struct {
				name  string
				paper string
				css   []byte
			}{
				{p.App, p.Paper, p.appCSS},
				{p.AppNight, p.PaperNight, p.appNightCSS},
			} {
				if half.paper == "" {
					continue
				}
				if half.css == nil {
					t.Errorf("preset %s declares a paper for %s, which is not a half it has",
						p.ID, half.name)
					continue
				}
				declared++
				body := string(half.css)
				at := strings.Index(body, "--bg:")
				if at < 0 {
					t.Errorf("preset %s declares paper %s, but %s sets no --bg",
						p.ID, half.paper, half.name)
					continue
				}
				rest := body[at+len("--bg:"):]
				if end := strings.IndexAny(rest, ";}\n"); end >= 0 {
					rest = rest[:end]
				}
				if got := presetPaper(p.ID, rest); got != half.paper {
					t.Errorf("preset %s (%s): manifest says paper %s, the file says %s",
						p.ID, half.name, half.paper, got)
				}
			}
		}
	}
	if declared == 0 {
		t.Fatal("no preset declares a paper at all; the windows the page does not own would go white")
	}
}

// The three standalone documents are served by the server and have none of the
// app's layer machinery, so a preset that asks for them (manifest "pages") is
// attached by the SERVER — and only while it is on. Quiet labels is the first:
// it speaks in the label TOKENS, which those pages define for the same names.
func TestPagePresetsReachTheStandalonePages(t *testing.T) {
	s, _ := newStyleServer(t)
	const needle = `data-preset="quiet_labels"`
	paths := []string{"/", "/setup", "/lemmas", "/browse"}
	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, newRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("GET %s: got %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	for _, p := range paths {
		if strings.Contains(get(p), needle) {
			t.Errorf("%s carries the page preset with nothing enabled", p)
		}
	}
	if err := s.presetStateWrite([]string{"quiet_labels"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if !strings.Contains(get(p), needle) {
			t.Errorf("%s does not carry the enabled page preset", p)
		}
	}
}

// A preset that asks for the standalone pages must name ONE file for both
// themes: those pages resolve their own theme — the OS preference, or the theme
// the reader pinned in the app — and have none of the machinery that swaps a
// day half for a night one, so the server would have to guess, and a guess is
// how a light half lands on a dark page.
func TestPagePresetsNameOneFileForBothThemes(t *testing.T) {
	groups, _ := presetRegistry()
	found := 0
	for _, g := range groups {
		for _, p := range g.Presets {
			if !p.Pages {
				continue
			}
			found++
			if p.App == "" || p.App != p.AppNight {
				t.Errorf("preset %s asks for the standalone pages but names app=%q appNight=%q",
					p.ID, p.App, p.AppNight)
			}
		}
	}
	if found == 0 {
		t.Fatal("no preset asks for the standalone pages, so the wiring above is untested")
	}
}

// GREY TEXT IS A STATE, and this test is the door it has to come through.
// Everything readable in the app reads --label / --label-quiet (the pages call
// them --label too, over their own greys), and a `color:` that takes a grey
// directly must be on a selector in the list below — a thing the reader cannot
// act on.
//
// It exists because the rule has already been broken once, by a NEW window:
// the System pane arrived on 2026-09-29 with --fg-soft on its group heads,
// hints, units and notes, so it was grey in BOTH looks — the one outcome the
// register exists to prevent, and one no other test could see. A new label
// wants var(--label); a disabled thing wants an entry here, with its reason.
func TestGreyTextIsOnlyForDisabledStates(t *testing.T) {
	allowed := map[string]string{
		".qled":                   "the search field's mark: not a label, and its lightness is argued where it lives",
		"details.noindex>summary": "a section the search could not run on",
		".pd .rmgo.busy":          "a removal that is running and cannot be pressed",
		".group-control-disabled": "a control the reader cannot act on",
		"#cands label.row.no":     "a candidate that cannot be installed",
	}
	// Each sheet with the names ITS greys go by: the app page's own, and the
	// two shorthands the standalone pages spell them with. The leading
	// delimiter is what keeps `border-color` and `background-color` out — a
	// border is not text, and one of the app's hover rules sets a grey one on
	// purpose.
	const appGrey = `(?:^|[;{\s])color:var\(--fg-(?:soft|faint)\)`
	const pageGrey = `(?:^|[;{\s])color:var\(--(?:soft|faint)\)`
	sheets := []struct {
		name    string
		css     string
		greyPat string
	}{
		{"app.css", string(appCSS), appGrey},
		{"group-editor.css", string(groupEditorCSS), appGrey},
		{"history.css", string(historyCSS), appGrey},
		{"setup.css", string(setupCSS), pageGrey},
		{"setup.html", setupHTML, pageGrey},
		{"lemmas.html", string(lemmasHTML), pageGrey},
		{"browse.html", string(browseHTML), pageGrey},
	}
	seen := map[string]bool{}
	for _, sheet := range sheets {
		re := regexp.MustCompile(sheet.greyPat)
		for _, rule := range cssRules(sheet.css) {
			if !re.MatchString(rule.body) {
				continue
			}
			if _, ok := allowed[rule.selector]; !ok {
				t.Errorf("%s: %s paints text with a grey that is not in the allowed list: use var(--label) / var(--label-quiet), or add it here with a reason",
					sheet.name, rule.selector)
				continue
			}
			seen[rule.selector] = true
		}
	}
	// An entry that nothing uses any more is a permission nobody needs, and it
	// would quietly cover a rule that comes back.
	for selector := range allowed {
		if !seen[selector] {
			t.Errorf("the allowed list keeps %s, which no stylesheet paints grey any more", selector)
		}
	}
}

// The label register, both halves of it: app.css must default BOTH tokens to
// the ink — that is what makes the quiet look a choice rather than the
// baseline — and the preset that restores the greys must set both, or half the
// labels would stay switched over.
func TestLabelRegisterDefaultsToInkAndTheQuietPresetSetsBoth(t *testing.T) {
	for _, decl := range []string{"--label:var(--fg)", "--label-quiet:var(--fg)"} {
		if !strings.Contains(string(appCSS), decl) {
			t.Errorf("app.css does not declare %s: the ink default is what keeps the new look the baseline", decl)
		}
	}
	body, err := presetFS.ReadFile("web/presets/labels/quiet_labels_app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range []string{"--label:var(--fg-soft)", "--label-quiet:var(--fg-faint)"} {
		if !strings.Contains(string(body), decl) {
			t.Errorf("the quiet-labels preset does not declare %s", decl)
		}
	}
}

// An app half has to be able to WIN against the palette of the theme it is
// attached in, or its switch turns on and the page does not move. That is not
// a hypothetical: app.css declares the light palette on `:root` (0,1,0) and
// the dark one on `html[data-theme="dark"]`, with an `html:not([data-theme])`
// copy for Auto under a dark OS — both (0,1,1). Sepia and High contrast tie
// with the light one and win by being later; True black and Warm dark were
// written on a bare `:root` too, so they LOST, and all a reader saw was the
// article tokens (the ones that block does not declare) taking effect while
// the chrome stayed grey. Measured 2026-09-28 with the theme pinned dark:
// `--bg` stayed #191a1c; fixed the same day by moving both files to
// `html[data-dark]`.
//
// The comparison is per PROPERTY, against app.css's own weight for it, so a
// preset that only paints tokens the palette does not touch — the fonts, the
// article surface — is left alone. app.css is read from the embedded asset,
// so it is the file that ships that is being asked.
func TestPresetAppHalvesOutrankTheirPalette(t *testing.T) {
	palette := appPaletteWeight(t)
	groups, _ := presetRegistry()
	checked := 0
	for _, g := range groups {
		for _, p := range g.Presets {
			for _, half := range []struct {
				name string
				css  []byte
				slot string // the palette the half has to beat
			}{
				{p.App, p.appCSS, "light"},
				{p.AppNight, p.appNightCSS, "dark"},
			} {
				if half.css == nil {
					continue
				}
				checked++
				for selector, props := range cssTokenBlocks(string(half.css)) {
					have := specificity(selector)
					for _, prop := range props {
						want := palette[half.slot][prop]
						if lessSpecific(have, want) {
							t.Errorf("%s (%s): %s declares %s at %v, which loses to app.css's %v for the %s palette",
								p.ID, half.name, selector, prop, have, want, half.slot)
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no app halves were checked at all")
	}
}

// The counter above is compared with itself, so a reading that is too heavy
// everywhere would pass the test above while being wrong. These are the
// numbers it has to produce - the four selectors the presets and palettes
// actually use, plus one at the weight app.css's Auto copy of the dark palette
// carries.
func TestSpecificityCountsTheSelectorsTheseFilesUse(t *testing.T) {
	cases := []struct {
		selector string
		want     specific
	}{
		{":root", specific{0, 1, 0}},
		{"html[data-dark]", specific{0, 1, 1}},
		{"html[data-shell-image]", specific{0, 1, 1}},
		{`html[data-theme="dark"]`, specific{0, 1, 1}},
		{"html:not([data-theme])", specific{0, 2, 1}},
		{"body", specific{0, 0, 1}},
		{".card input", specific{0, 1, 1}},
		{":root, html[data-dark]", specific{0, 1, 1}},
	}
	for _, c := range cases {
		if got := specificity(c.selector); got != c.want {
			t.Errorf("specificity(%q) = %v, want %v", c.selector, got, c.want)
		}
	}
}

// appPaletteWeight is app.css's weight for each palette token, per theme: the
// `:root` block for the light palette, the max of `:root` and
// `html[data-theme="dark"]` for the dark one (its `html:not([data-theme])`
// twin in the media query carries the same weight and the same values).
func appPaletteWeight(t *testing.T) map[string]map[string]specific {
	t.Helper()
	out := map[string]map[string]specific{"light": {}, "dark": {}}
	root, dark := false, false
	for selector, props := range cssTokenBlocks(string(appCSS)) {
		switch selector {
		case ":root":
			root = true
			for _, prop := range props {
				out["light"][prop] = specificity(selector)
				out["dark"][prop] = specificity(selector)
			}
		case `html[data-theme="dark"]`:
			dark = true
			for _, prop := range props {
				out["dark"][prop] = specificity(selector)
			}
		}
	}
	if !root || !dark {
		t.Fatalf("app.css's palettes were not found (%v): this test is looking at the wrong asset", []bool{root, dark})
	}
	return out
}

// specific is a CSS specificity, a-b-c (ids, classes/attributes/pseudo-classes,
// elements), compared lexicographically.
type specific [3]int

func (s specific) String() string { return fmt.Sprintf("(%d,%d,%d)", s[0], s[1], s[2]) }

func lessSpecific(a, b specific) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// specificity is deliberately a small counter for the selectors these files
// hold - `:root`, `html[data-dark]`, `html[data-shell-image]`,
// `html[data-theme="dark"]` - and not a CSS engine: `:not()`/`:is()` count as
// written rather than by their argument (which makes a selector read a shade
// heavier, never lighter, so the test can only be too quiet, not wrong about
// the shape it exists to catch), and a selector LIST counts as its heaviest
// part, which is how the cascade reads one too.
func specificity(selector string) specific {
	var out specific
	for _, part := range strings.Split(selector, ",") {
		if s := oneSpecificity(part); lessSpecific(out, s) {
			out = s
		}
	}
	return out
}

func oneSpecificity(part string) specific {
	var out specific
	for i := 0; i < len(part); {
		switch c := part[i]; {
		case c == '#':
			out[0]++
			i++
			i = skipName(part, i)
		case c == '.' || c == '[' || c == ':':
			// A class, an attribute or a pseudo-class: the name that follows
			// belongs to it and is not an element name.
			out[1]++
			i = skipName(part, i+1)
		case c == '"' || c == '\'':
			// An attribute's value: `[data-theme="dark"]` is one attribute,
			// and `dark` is not an element.
			i++
			for i < len(part) && part[i] != c {
				i++
			}
			i++
		case isNameByte(c):
			if !isLetter(c) {
				i++ // a lone `-` or a digit at this position names nothing
				break
			}
			out[2]++
			i = skipName(part, i)
		default:
			i++
		}
	}
	return out
}

func skipName(s string, i int) int {
	for i < len(s) && isNameByte(s[i]) {
		i++
	}
	return i
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '_' || c == '-'
}

// cssRulePair is one rule of a stylesheet: the selector as written and the
// body between its braces.
type cssRulePair struct{ selector, body string }

// cssRules lists the rules of a stylesheet, in order. At-rule bodies are
// descended into (a rule inside @media is still a rule) and comments are
// dropped first, so `/* color:var(--fg-soft) */` cannot be mistaken for a
// declaration.
func cssRules(css string) []cssRulePair {
	var out []cssRulePair
	var walk func(string)
	walk = func(src string) {
		for i := 0; ; {
			open := strings.IndexByte(src[i:], '{')
			if open < 0 {
				return
			}
			open += i
			selector := strings.TrimSpace(src[i:open])
			depth, j := 1, open+1
			for j < len(src) && depth > 0 {
				if src[j] == '{' {
					depth++
				} else if src[j] == '}' {
					depth--
				}
				j++
			}
			body := src[open+1 : j-1]
			if strings.HasPrefix(selector, "@") {
				walk(body)
			} else {
				out = append(out, cssRulePair{selector, body})
			}
			i = j
		}
	}
	walk(stripComments(css))
	return out
}

// cssTokenBlocks lists the rules of a stylesheet that declare custom
// properties, as selector -> property names.
func cssTokenBlocks(css string) map[string][]string {
	out := map[string][]string{}
	for _, rule := range cssRules(css) {
		if props := customProps(rule.body); len(props) > 0 {
			out[rule.selector] = props
		}
	}
	return out
}

// customProps names the custom properties a rule body declares.
func customProps(body string) []string {
	var out []string
	for i := 0; i+1 < len(body); i++ {
		if body[i] != '-' || body[i+1] != '-' {
			continue
		}
		j := i + 2
		for j < len(body) && isNameByte(body[j]) {
			j++
		}
		if j > i+2 && j < len(body) && body[j] == ':' {
			out = append(out, body[i:j])
		}
		i = j
	}
	return out
}

func stripComments(css string) string {
	var b strings.Builder
	for i := 0; i < len(css); i++ {
		if css[i] == '/' && i+1 < len(css) && css[i+1] == '*' {
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 3
			continue
		}
		b.WriteByte(css[i])
	}
	return b.String()
}

// The resolved set is what the page hands the shell: one value per theme, from
// the presets that are ON. A day preset says nothing about the night, and the
// last declarer in manifest order wins a theme - the order the page attaches
// the layers in, so the value matches the --bg the page ends up showing.
func TestPresetPaperFollowsTheEnabledSet(t *testing.T) {
	groups, _ := presetRegistry()
	on := func(ids ...string) map[string]string { return presetPaperSet(groups, ids) }

	if got := on(); got["light"] != "" || got["dark"] != "" {
		t.Fatalf("nothing enabled must declare nothing: %v", got)
	}
	if got := on("sepia"); got["light"] != "#f4ecd8" || got["dark"] != "" {
		t.Fatalf("Sepia is a day paper and nothing else: %v", got)
	}
	if got := on("true_black"); got["light"] != "" || got["dark"] != "#000000" {
		t.Fatalf("True black is a night paper, in six digits: %v", got)
	}
	if got := on("unknown_preset_id", "compact"); got["light"] != "" || got["dark"] != "" {
		t.Fatalf("presets that paint no paper must leave both slots empty: %v", got)
	}
	// Two groups can be on together (the radio rule is per group), and both of
	// these paint the day paper: high_contrast comes later in the manifest,
	// which is the layer the page attaches last and the one that wins.
	if got := on("sepia", "high_contrast"); got["light"] != "#f1f0ed" {
		t.Fatalf("the last declarer in manifest order must win: %v", got)
	}
	if got := on("sepia", "high_contrast", "warm_dark"); got["light"] != "#f1f0ed" || got["dark"] != "#1c1a17" {
		t.Fatalf("the two themes are resolved independently: %v", got)
	}
}

// The radio rule is about the surface two presets claim, and the theme is what
// decides it: Sepia and True black are both on the same group's paper, but one
// is the day's and the other the night's, so they never compete.
func TestPresetCompetesByTheme(t *testing.T) {
	_, index := presetRegistry()
	sepia, trueBlack := index["sepia"], index["true_black"]
	if sepia == nil || trueBlack == nil {
		t.Fatal("sepia and true_black are both expected to ship")
	}
	if sepia.competes(trueBlack) {
		t.Fatal("a day preset and a night preset must not compete")
	}
	background := index["background"]
	if background == nil {
		t.Fatal("the background preset is expected to ship")
	}
	if !background.competes(sepia) || !background.competes(trueBlack) {
		t.Fatal("a preset that covers both themes competes with either")
	}
	if sepia.Theme() != "light" || trueBlack.Theme() != "dark" || background.Theme() != "both" {
		t.Fatalf("themes must be derived from the slots: sepia=%q true_black=%q background=%q",
			sepia.Theme(), trueBlack.Theme(), background.Theme())
	}
}
