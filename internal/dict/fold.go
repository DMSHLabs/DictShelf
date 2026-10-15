// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// FoldVersion identifies Fold's BEHAVIOUR, and must be bumped in the same
// commit as any change to what Fold returns.
//
// Fold is called at query time and at ingest time. Ingest persists its result
// in the folded headword and alias keys, and in the optional trigram index.
// A change to Fold requires this bump so prepared dictionaries are reported
// outdated. Direct backends rebuild their fold indexes on every open.
const FoldVersion = 1

// Fold lowercases and strips combining marks for accent/case-insensitive
// matching. Shared by every direct backend's fold index.
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

// HasDiacritics reports combining marks, including marks in precomposed text.
func HasDiacritics(s string) bool {
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			return true
		}
	}
	return false
}

// CaseKey keeps diacritics while normalising Unicode spelling and case.
func CaseKey(s string) string { return norm.NFC.String(strings.ToLower(norm.NFC.String(s))) }
