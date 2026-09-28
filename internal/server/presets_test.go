// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"path"
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
