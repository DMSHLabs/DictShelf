// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fold lowercases and strips combining marks for accent/case-insensitive
// matching. Shared by every direct backend's fold index.
//
// Nothing Fold returns is stored: the direct backends rebuild their fold
// indexes on every open, entry_fts folds with SQLite's own tokenizer, and
// idx_entry_w is a raw NOCASE index. So Fold can change without making any
// prepared dictionary outdated.
func Fold(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, ru := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, ru) {
			continue
		}
		b.WriteRune(ru)
	}
	return b.String()
}
