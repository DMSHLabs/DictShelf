// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package htmlref

import (
	"io"
	"strings"
)

// The escapers every article writer shares, each a Replacer built once (it
// compiles a trie) and run per entry of an ingest. Not html.EscapeString: that
// also rewrites ' and " in text, and prepared databases hold text escaped
// exactly this way - a second spelling would change bytes the reader goldens
// pin while rendering nothing differently.
var (
	textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

// EscapeText escapes s for HTML text content: &, < and >.
func EscapeText(s string) string { return textEscaper.Replace(s) }

// WriteText is EscapeText written straight to w, with no intermediate string.
func WriteText(w io.Writer, s string) (int, error) { return textEscaper.WriteString(w, s) }

// EscapeAttr escapes s for a double-quoted attribute value: EscapeText's three
// and ". Dictionary content that lands in an attribute goes through this - a
// " left as it is closes the attribute and starts another.
func EscapeAttr(s string) string { return attrEscaper.Replace(s) }
