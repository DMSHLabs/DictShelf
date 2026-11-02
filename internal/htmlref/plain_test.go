// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package htmlref

import "testing"

// Moved here with the function, from internal/store: the stripper is what the
// article index of a prepared database is built from, and the archive reader
// has to fold the same text, so it cannot live in a package that imports the
// formats it strips for (internal/format/zim, which would close a cycle).
func TestStripHTML(t *testing.T) {
	cases := map[string]string{
		`<p>a<br/>b</p>`:                             "a b",
		`<script>var x=1;</script>text`:              "text",
		`<style>.a{color:red}</style>word`:           "word",
		`a &amp; b &lt;c&gt;`:                        "a & b <c>",
		`<div class="k1 k2" style="x:y">inner</div>`: "inner",
		// A self-closing script has no contents and no closing tag: the skip
		// it opens has to end with the tag itself, or everything after it in
		// the article is silently missing from the full-text index.
		`<script src="x.js"/>after`:         "after",
		`<style/>after`:                     "after",
		`<script/>a<script>bad()</script>b`: "a b",
		// ...but a "/>" INSIDE script contents is script text, not the end of
		// the skip.
		`<script>f(a<b/>c)</script>tail`: "tail",
		``:                               "",
	}
	for in, want := range cases {
		if got := PlainText(in); got != want {
			t.Errorf("PlainText(%q) = %q, want %q", in, got, want)
		}
	}
}
