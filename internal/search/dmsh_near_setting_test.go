// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package search

import (
	"strings"
	"testing"
)

// The near READING is the reader's window, so the rung a near search runs must
// carry the setting - that is the whole promise of the panel field. The words
// reading must not move with it: it asks for the words anywhere in the article
// whatever the window is, and a window that leaked into it would turn the mode
// into a second proximity search.
func TestNearRungCarriesTheSetting(t *testing.T) {
	defer SetNearWords(0)
	SetNearWords(3)
	if rung, ok := rungFor(Near, "собака кошка"); !ok || !strings.HasSuffix(rung.Match, ", 3)") {
		t.Fatalf("the near rung is %q (ok=%v); it must run at the setting", rung.Match, ok)
	}
	if rung, ok := rungFor(FullText, "собака кошка"); !ok || strings.Contains(rung.Match, "NEAR(") {
		t.Fatalf("the words reading became %q", rung.Match)
	}
	SetNearWords(0)
	if rung, _ := rungFor(Near, "собака кошка"); !strings.HasSuffix(rung.Match, ", 10)") {
		t.Fatalf("an unset window produced %q; it must be the default", rung.Match)
	}
}
