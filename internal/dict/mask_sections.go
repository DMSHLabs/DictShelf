// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"unicode"
	"unicode/utf8"
)

// MaskInitial groups a headword under the same readable initial as Browse.
func MaskInitial(word string) string {
	r, _ := utf8.DecodeRuneInString(word)
	if r >= 0xAC00 && r <= 0xD7A3 {
		return string([]rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")[(r-0xAC00)/588])
	}
	f, _ := utf8.DecodeRuneInString(Fold(string(r)))
	if f == utf8.RuneError {
		f = r
	}
	if f >= 0x1100 && f <= 0x1112 {
		return string([]rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")[f-0x1100])
	}
	switch {
	case unicode.IsDigit(f):
		return "0-9"
	case unicode.Is(unicode.Han, f):
		return "漢"
	case unicode.Is(unicode.Hiragana, f):
		return "あ"
	case unicode.Is(unicode.Katakana, f):
		return "ア"
	case !unicode.IsLetter(f):
		return "#"
	}
	return string(unicode.ToUpper(f))
}

type maskBoundary struct {
	initial, word string
	offset        int
}

// MaskPageSize is the fixed size of one mask result block. The server uses the
// same size to page the matching headwords.
const MaskPageSize = 300

// MaskSections records the first matching headword of each result page.
// The strip labels page starts, not every initial appearing inside a page.
type MaskSections struct {
	boundaries []maskBoundary
	total      int
}

func (m *MaskSections) Add(word string) {
	if m.total%MaskPageSize == 0 {
		m.boundaries = append(m.boundaries, maskBoundary{initial: MaskInitial(word), word: word, offset: m.total})
	}
	m.total++
}

func (m *MaskSections) Finish() []Letter {
	var out []Letter
	used := make(map[string]bool)
	for _, boundary := range m.boundaries {
		label := boundary.initial
		if used[label] {
			label = maskBlockPrefix(boundary.word, boundary.initial, used)
		}
		if len(out) > 0 {
			out[len(out)-1].Count = boundary.offset - out[len(out)-1].Offset
		}
		out = append(out, Letter{Letter: label, Offset: boundary.offset})
		used[label] = true
	}
	if len(out) > 0 {
		out[len(out)-1].Count = m.total - out[len(out)-1].Offset
	}
	return out
}

func maskBlockPrefix(word, initial string, used map[string]bool) string {
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
		if label != initial && !used[label] {
			return label
		}
	}
	return word
}
