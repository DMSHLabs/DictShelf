// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package htmlref

import (
	"strings"
	"testing"
)

// The contract prepared databases depend on: text keeps its quotes, an
// attribute value does not.
func TestEscapers(t *testing.T) {
	for _, tc := range []struct{ in, text, attr string }{
		{"", "", ""},
		{"plain", "plain", "plain"},
		{`a&b<c>d`, `a&amp;b&lt;c&gt;d`, `a&amp;b&lt;c&gt;d`},
		{`say "hi" it's`, `say "hi" it's`, `say &quot;hi&quot; it's`},
		{"&amp;", "&amp;amp;", "&amp;amp;"},
	} {
		if got := EscapeText(tc.in); got != tc.text {
			t.Errorf("EscapeText(%q) = %q, want %q", tc.in, got, tc.text)
		}
		if got := EscapeAttr(tc.in); got != tc.attr {
			t.Errorf("EscapeAttr(%q) = %q, want %q", tc.in, got, tc.attr)
		}
		var b strings.Builder
		if _, err := WriteText(&b, tc.in); err != nil || b.String() != tc.text {
			t.Errorf("WriteText(%q) = %q, %v; want %q", tc.in, b.String(), err, tc.text)
		}
	}
}
