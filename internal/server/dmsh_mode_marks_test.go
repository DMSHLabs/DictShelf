// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/ftsq"
)

// The three article readings are drawn marks whose middle carries the rule:
// nothing for a phrase, the bounded distance for "near", dots for "anywhere".
//
// Three things about them can rot without any test in Go noticing, and each of
// them has already gone wrong once in this area:
//
//  1. the number the "near" mark writes has to be the number that runs - the
//     page can only copy internal/ftsq's default, not read it, so the copy is
//     compared here;
//  2. a reading the menu offers has to have a mark, or its row falls back to
//     plain text while the two beside it wear one;
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

	m := regexp.MustCompile(`const NEAR_WORDS=(\d+);`).FindStringSubmatch(text)
	if m == nil {
		t.Fatal("the page declares no NEAR_WORDS: the near mark's number has no home")
	}
	if m[1] != strconv.Itoa(ftsq.NearDefault) {
		t.Errorf("the near mark draws %s, but FTS5's NEAR runs with %d", m[1], ftsq.NearDefault)
	}

	marks := regexp.MustCompile(`(?s)MODE_SVG_ICONS=\(\(\)=>\{.*?\}\)\(\);`).FindString(text)
	if marks == "" {
		t.Fatal("the page draws no mode marks at all")
	}
	for _, mode := range []string{"phrase", "near", "fts"} {
		if !strings.Contains(marks, mode+":head+") {
			t.Errorf("the %q reading has no drawn mark", mode)
		}
	}
	if !strings.Contains(marks, "<text") || !strings.Contains(marks, "${NEAR_WORDS}") {
		t.Error("the near mark does not write its number from NEAR_WORDS")
	}
	if strings.Contains(marks, ">10<") {
		t.Error("the near mark writes 10 literally instead of from NEAR_WORDS")
	}

	if regexp.MustCompile(`hasOwnProperty\.call\(MODE_SHORT`).MatchString(text) {
		t.Error("setSearchMode gates on MODE_SHORT again: a mode that is in the select but not in that table cannot be restored")
	}
	if !strings.Contains(text, "function searchModeExists") {
		t.Error("setSearchMode no longer reads the modes from the control")
	}
}
