// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package htmlref

import "strings"

// A lookup target is a headword, an opaque string - never a URL: it may hold
// spaces, "?", "&" and every script there is. Only what would change how the
// link is READ is encoded, so the target stays legible in the markup:
//
//   - "%", or the reader would decode a literal "%41" in a headword;
//   - "#", or the reader would split the headword at it as a fragment;
//   - C0 controls and DEL, which no attribute or markdown destination carries;
//   - a leading "@", or the reader would take a headword like "@home" for an
//     MDict sub-entry reference (D26), which is decided on the undecoded form.
//
// The reader side is parseRef in index.html and frame.js: split at the first
// literal "#", then percent-decode each half, all or nothing.

const hexDigits = "0123456789ABCDEF"

func encodes(c byte, i int) bool {
	return c == '%' || c == '#' || c < 0x20 || c == 0x7F || c == '@' && i == 0
}

// EncTarget percent-encodes a headword for use after `entry://`.
func EncTarget(s string) string {
	i := 0
	for i < len(s) && !encodes(s[i], i) {
		i++
	}
	if i == len(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	b.WriteString(s[:i])
	for ; i < len(s); i++ {
		c := s[i]
		if encodes(c, i) {
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&15])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// EntryHref is the lookup link to headword: `entry://` + EncTarget.
func EntryHref(headword string) string { return "entry://" + EncTarget(headword) }
