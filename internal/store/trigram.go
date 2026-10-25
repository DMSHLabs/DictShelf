// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"strings"
	"unicode/utf8"
)

// trigramMatch asks FTS5 only for one trigram. The query is a candidate filter;
// callers verify the complete substring against the stored folded headword or
// with their existing matcher. This also works with detail=none trigram tables.
func trigramMatch(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	n := 0
	for i, r := range s {
		if n == 3 {
			break
		}
		if r == '"' {
			b.WriteString(`""`)
		} else {
			_, size := utf8.DecodeRuneInString(s[i:])
			b.WriteString(s[i : i+size])
		}
		n++
	}
	if n < 3 {
		return ""
	}
	b.WriteByte('"')
	return b.String()
}
