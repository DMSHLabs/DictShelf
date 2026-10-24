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

// HeadwordWordMatcher requires every query token to occur as a complete token
// in the headword, in any position.
func HeadwordWordMatcher(query string) func(string) bool {
	return headwordAlternativeMatcher(query, headwordWordMatcherSingle)
}

// HeadwordSuffixMatcher requires every query token to end a complete headword
// token, in any position.
func HeadwordSuffixMatcher(query string) func(string) bool {
	return headwordTokenMatcher(query, strings.HasSuffix)
}

// HeadwordContainsMatcher requires every query token to occur within a
// complete headword token, in any position.
func HeadwordContainsMatcher(query string) func(string) bool {
	return headwordTokenMatcher(query, strings.Contains)
}

func headwordTokenMatcher(query string, match func(string, string) bool) func(string) bool {
	terms := headwordTokens(query)
	return func(word string) bool {
		if len(terms) == 0 {
			return false
		}
		words := headwordTokens(word)
		for _, term := range terms {
			found := false
			for _, token := range words {
				if match(token, term) {
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

// HeadwordExactMatcher compares the complete displayed headword using the
// same case- and accent-folding rules as the prepared search indexes.
func HeadwordExactMatcher(query string) func(string) bool {
	return headwordAlternativeMatcher(query, func(value string) func(string) bool {
		needle := Fold(strings.TrimSpace(value))
		return func(word string) bool { return needle != "" && Fold(word) == needle }
	})
}

const headwordAlternativeSeparator = "\x1f"

func headwordAlternativeMatcher(query string, makeMatcher func(string) func(string) bool) func(string) bool {
	parts := strings.Split(query, headwordAlternativeSeparator)
	matchers := make([]func(string) bool, 0, len(parts))
	for _, part := range parts {
		matchers = append(matchers, makeMatcher(part))
	}
	return func(word string) bool {
		for _, matches := range matchers {
			if matches(word) {
				return true
			}
		}
		return false
	}
}

func headwordWordMatcherSingle(query string) func(string) bool {
	terms := headwordTokens(query)
	return func(word string) bool {
		if len(terms) == 0 {
			return false
		}
		words := headwordTokens(word)
		for _, term := range terms {
			found := false
			for _, token := range words {
				if token == term {
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
