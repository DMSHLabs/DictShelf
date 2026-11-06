// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/ftsq"
)

// The three article readings are marked by typed glyphs, and three things about
// them can rot without any test in Go noticing - two of which have already gone
// wrong once in this area:
//
//  1. the bound is stated in the row's own hint (the mark is two words and what
//     stands between them), and since 2026-11-06 that bound is the READER's
//     setting: the hint renders the number the search runs with, so what the
//     catalogue has to carry is the placeholder the page fills - a literal would
//     be a second number, free to drift;
//  2. the numbers a reader can still read - the panel's own sentence, and the
//     bounds of the stepper - have to be the numbers the search enforces
//     (internal/ftsq), not copies of them;
//  3. which modes can be restored must come from the control, not from a table
//     of marks - it came from MODE_SHORT, and the three readings were absent
//     from it, so choosing "Phrase" and reloading dropped the reader back to
//     the first option in silence (found by driving the page, 2026-11-02).
func TestModeMarksQuoteTheNearRule(t *testing.T) {
	page, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(page)

	glyphs := regexp.MustCompile(`const MODE_GLYPHS=\{[^}]*\}`).FindString(text)
	if glyphs == "" {
		t.Fatal("the page declares no mode glyphs")
	}
	for _, want := range []string{`phrase:"[A B]"`, `near:"[A ↔ B]"`, `fts:"[A … B]"`} {
		if !strings.Contains(glyphs, want) {
			t.Errorf("the %s reading has no mark", want[:strings.Index(want, ":")])
		}
	}

	// The page takes the window's floor, default and ceiling from the server's
	// substitution (server.go renders them from internal/ftsq), and the settings
	// panel reads them from there - so a stepper can never offer a number the
	// search would refuse, and the three cannot drift apart.
	for _, want := range []string{"{{NEAR_WORDS_MIN}}", "{{NEAR_WORDS_DEFAULT}}", "{{NEAR_WORDS_MAX}}"} {
		if !strings.Contains(text, want) {
			t.Errorf("web/index.html does not take the near bounds from %s", want)
		}
	}
	pageServer, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"{{NEAR_WORDS_MIN}}", "{{NEAR_WORDS_DEFAULT}}", "{{NEAR_WORDS_MAX}}"} {
		if !strings.Contains(string(pageServer), want) {
			t.Errorf("server.go does not substitute %s: the page would render the placeholder itself", want)
		}
	}

	number := strconv.Itoa(ftsq.NearDefault)
	floor := strconv.Itoa(ftsq.NearMin)
	for _, catalog := range []string{"web/i18n/ru.json", "web/i18n/en.json"} {
		data, err := os.ReadFile(catalog)
		if err != nil {
			t.Fatal(err)
		}
		// any, not string: the catalogs hold plural forms as nested objects.
		var entries map[string]any
		if err := json.Unmarshal(data, &entries); err != nil {
			t.Fatal(err)
		}
		forms, _ := entries["search.modeHintNear"].(map[string]any)
		if len(forms) == 0 {
			t.Errorf("%s: the near hint is not a set of plural forms", catalog)
		}
		for form, value := range forms {
			if hint, _ := value.(string); !strings.Contains(hint, "{number}") {
				t.Errorf("%s: the %s form of the near hint does not carry {number}: %q", catalog, form, hint)
			}
		}
		panel, _ := entries["panel.nearWordsHint"].(string)
		if !strings.Contains(panel, number) || !strings.Contains(panel, floor) {
			t.Errorf("%s: the panel's near sentence does not state the default (%s) and the floor (%s): %q", catalog, number, floor, panel)
		}
	}

	if regexp.MustCompile(`hasOwnProperty\.call\(MODE_SHORT`).MatchString(text) {
		t.Error("setSearchMode gates on MODE_SHORT again: a mode that is in the select but not in that table cannot be restored")
	}
	if !strings.Contains(text, "function searchModeExists") {
		t.Error("setSearchMode no longer reads the modes from the control")
	}
}
