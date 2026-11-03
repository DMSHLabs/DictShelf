// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bytes"

	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/util"
)

// Link reference definitions apply file-wide, so every parse of a chunk is
// given the definitions it can use. Giving it all of them costs every parse
// the whole set: with 100,000 definitions that is 12 ms a parse, minutes for a
// large file. A parse needs only the labels its text can reference, and those
// are found in one pass over the bytes.

// prefillAll is the count of definitions below which a parse is simply given
// all of them, cheaper than looking for the ones it references. A variable so
// the tests can force the lookup.
var prefillAll = 1000

// prefill gives pc the definitions of defs (keyed by util.ToLinkReference of
// their label) that text can reference.
func prefill(pc parser.Context, defs map[string]parser.LinkDefinition, text []byte) {
	if len(defs) < prefillAll {
		for _, d := range defs {
			pc.AddLinkDefinition(d)
		}
		return
	}
	for l := range refLabels(text) {
		if d, ok := defs[l]; ok {
			pc.AddLinkDefinition(d)
		}
	}
}

// refLabels returns a superset of the normalized labels the inline parser
// can look up in text. A label holds no unescaped bracket, so the only span
// that can be one ends at an unescaped `]` and starts at the last unescaped
// `[` before it; each `[` serves one `]`, which keeps the spans disjoint and
// the pass linear. goldmark reads a label from its block's segments, without
// the container markers of continuation lines (a blockquote's `>`), and a
// table cell sees `\|` as `|`: each span is also normalized without them.
func refLabels(text []byte) map[string]struct{} {
	out := map[string]struct{}{}
	add := func(b []byte) {
		if l := util.ToLinkReference(b); l != "" {
			out[l] = struct{}{}
		}
	}
	open := -1
	for i, c := range text {
		if c != '[' && c != ']' || escaped(text, i) {
			continue
		}
		if c == '[' {
			open = i
			continue
		}
		if open < 0 {
			continue
		}
		span := text[open+1 : i]
		open = -1
		add(span)
		if bytes.IndexByte(span, '\n') >= 0 {
			add(stripMarkers(span))
		}
		if bytes.Contains(span, []byte(`\|`)) {
			add(bytes.ReplaceAll(span, []byte(`\|`), []byte("|")))
		}
	}
	return out
}

// escaped reports a byte preceded by an odd run of backslashes.
func escaped(b []byte, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && b[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// stripMarkers removes the leading SP, TAB and `>` of every line after the
// first.
func stripMarkers(span []byte) []byte {
	lines := bytes.Split(span, []byte("\n"))
	for i := 1; i < len(lines); i++ {
		lines[i] = bytes.TrimLeft(lines[i], " \t>")
	}
	return bytes.Join(lines, []byte("\n"))
}
