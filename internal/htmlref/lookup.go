// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package htmlref

import "strings"

// Cross-reference links: `entry://word[#frag]` is the only form wudict writes
// (D153). `bword:` is accepted on input and nowhere emitted: it is Babylon
// Ltd.'s proprietary scheme, from its BGL glossaries and Babylon Builder in the
// late 1990s, with no public specification - only the behaviour open-source
// dictionary readers reverse-engineered and preserved. Dictionaries still carry
// it, so every output path passes their links through CanonRef.

// CanonRef returns the canonical spelling of a cross-reference and whether it
// differs from ref. `bword:` with or without `//` (scheme case ignored) becomes
// `entry://`; a headword starting with "@" (an MDict sub-entry, D26) takes the
// slash-less `entry:@…` instead, because behind `//` the browser parses "@" as
// the userinfo delimiter and drops it. Anything else is returned unchanged.
func CanonRef(ref string) (string, bool) {
	var rest string
	switch {
	case hasPrefixFold(ref, "bword:"):
		rest = strings.TrimPrefix(ref[len("bword:"):], "//")
	case hasPrefixFold(ref, "entry://@"):
		rest = ref[len("entry://"):]
	default:
		return ref, false
	}
	if strings.HasPrefix(rest, "@") {
		return "entry:" + rest, true
	}
	return "entry://" + rest, true
}

// CanonLinks applies CanonRef to every reference in doc.
func CanonLinks(doc string) string {
	if !containsASCIIFold(doc, "bword:") && !containsASCIIFold(doc, "entry://@") {
		return doc
	}
	return Rewriter{URL: func(r Ref) string {
		s, _ := CanonRef(r.URL)
		return s
	}}.Rewrite(doc)
}

func hasPrefixFold(s, p string) bool {
	return len(s) >= len(p) && strings.EqualFold(s[:len(p)], p)
}

// containsASCIIFold reports whether s contains the ASCII string sub, ignoring case.
func containsASCIIFold(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}
