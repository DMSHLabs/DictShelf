// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"strings"
	"unicode"
)

// MatchHeadwordTerms uses the same token-prefix meaning as Browse's find box.
func MatchHeadwordTerms(word, query string) bool {
	return HeadwordTermMatcher(query)(word)
}

// HeadwordTermMatcher folds the query once for a full headword-index scan.
func HeadwordTermMatcher(query string) func(string) bool {
	terms := headwordTokens(query)
	return func(word string) bool {
		if len(terms) == 0 {
			return false
		}
		words := headwordTokens(word)
		for _, term := range terms {
			found := false
			for _, token := range words {
				if strings.HasPrefix(token, term) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
}

func headwordTokens(s string) []string {
	var tokens []string
	var token strings.Builder
	flush := func() {
		if token.Len() > 0 {
			tokens = append(tokens, Fold(token.String()))
			token.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			token.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}
