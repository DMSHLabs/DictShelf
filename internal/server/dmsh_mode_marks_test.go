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

// The three article readings are marked by typed glyphs, and two things about
// them can rot without any test in Go noticing - both of which have already
// gone wrong once in this area:
//
//  1. the bound is stated in the row's own hint (the mark is two words and what
//     stands between them), so the hint has to quote the number that runs -
//     internal/ftsq's default, which a page and a catalog can only copy;
//  2. which modes can be restored must come from the control, not from a table
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

	number := strconv.Itoa(ftsq.NearDefault)
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
		hint, _ := entries["search.modeHintNear"].(string)
		if !strings.Contains(hint, number) {
			t.Errorf("%s: the near hint %q does not state the %s the search runs with", catalog, hint, number)
		}
	}

	if regexp.MustCompile(`hasOwnProperty\.call\(MODE_SHORT`).MatchString(text) {
		t.Error("setSearchMode gates on MODE_SHORT again: a mode that is in the select but not in that table cannot be restored")
	}
	if !strings.Contains(text, "function searchModeExists") {
		t.Error("setSearchMode no longer reads the modes from the control")
	}
}
