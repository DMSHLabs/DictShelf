// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"strings"
	"unicode"
)

// SectionLabel applies the shared Browse/Wildcards naming rule to one page.
// initial and lastInitial are source-specific script labels; used is scoped to
// one dictionary and prevents repeated page labels within that dictionary.
func SectionLabel(previousLast, first, last, initial, lastInitial string, used map[string]bool) string {
	label := initial
	if previousLast != "" && first != "" {
		if prefix := distinguishingSectionPrefix(previousLast, first); prefix != "" {
			label = prefix
		}
	}
	if label == "" {
		label = initial
	}
	suffix := ""
	if lastInitial != "" && lastInitial != initial {
		suffix = "-" + lastInitial
	}
	if used != nil && used[label+suffix] {
		label = uniqueSectionPrefix(first, initial, suffix, used)
	}
	label += suffix
	if used != nil {
		used[label] = true
	}
	return label
}

// distinguishingSectionPrefix keeps the first-word character rule, but when
// whole words match it includes the first complete differing word.
func distinguishingSectionPrefix(previous, current string) string {
	previousWords, currentWords := strings.Fields(previous), strings.Fields(current)
	sharedWords := 0
	for sharedWords < len(previousWords) && sharedWords < len(currentWords) &&
		strings.EqualFold(previousWords[sharedWords], currentWords[sharedWords]) {
		sharedWords++
	}
	if sharedWords > 0 && sharedWords < len(currentWords) {
		label := []rune(strings.Join(currentWords[:sharedWords+1], " "))
		label[0] = unicode.ToUpper(label[0])
		return string(label)
	}
	if sharedWords == 0 && len(previousWords) > 0 && len(currentWords) > 0 {
		previousRunes, currentRunes := []rune(previousWords[0]), []rune(currentWords[0])
		common := 0
		for common < len(previousRunes) && common < len(currentRunes) &&
			unicode.ToLower(previousRunes[common]) == unicode.ToLower(currentRunes[common]) {
			common++
		}
		if common < len(currentRunes) {
			currentRunes[0] = unicode.ToUpper(currentRunes[0])
			return string(currentRunes[:common+1])
		}
		if len(currentRunes) > 0 {
			currentRunes[0] = unicode.ToUpper(currentRunes[0])
			return string(currentRunes)
		}
	}
	return commonSectionPrefix(previous, current)
}

func commonSectionPrefix(previous, current string) string {
	previousRunes, currentRunes := []rune(previous), []rune(current)
	common := 0
	for common < len(previousRunes) && common < len(currentRunes) &&
		unicode.ToLower(previousRunes[common]) == unicode.ToLower(currentRunes[common]) {
		common++
	}
	if common == 0 {
		return ""
	}
	currentRunes[0] = unicode.ToUpper(currentRunes[0])
	return string(currentRunes[:common])
}

func uniqueSectionPrefix(word, initial, suffix string, used map[string]bool) string {
	runes := []rune(Fold(word))
	if len(runes) == 0 {
		runes = []rune(word)
	}
	if len(runes) == 0 {
		return initial
	}
	runes[0] = unicode.ToUpper(runes[0])
	for n := 1; n <= len(runes); n++ {
		label := string(runes[:n])
		if label != initial && !used[label+suffix] {
			return label
		}
	}
	return string(runes)
}
