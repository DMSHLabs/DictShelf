// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"unicode"
	"unicode/utf8"

	"github.com/DMSHLabs/DictShelf/internal/indexconst"
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
	initial, word, last string
	offset              int
}

// MaskPageSize is the fixed size of one mask result block. The server uses the
// same size to page the matching headwords.
const MaskPageSize = indexconst.BrowsePageSize

// MaskSections records the first and last matching headword of each result
// page. The strip labels page starts and the last initial reached in the page.
type MaskSections struct {
	boundaries []maskBoundary
	total      int
}

func (m *MaskSections) Add(word string) {
	if m.total%MaskPageSize == 0 {
		m.boundaries = append(m.boundaries, maskBoundary{initial: MaskInitial(word), word: word, offset: m.total})
	}
	if len(m.boundaries) > 0 {
		m.boundaries[len(m.boundaries)-1].last = word
	}
	m.total++
}

func (m *MaskSections) Finish() []Letter {
	var out []Letter
	used := make(map[string]bool)
	for index, boundary := range m.boundaries {
		previousLast := ""
		if index > 0 {
			previousLast = m.boundaries[index-1].last
		}
		lastInitial := ""
		first, _ := utf8.DecodeRuneInString(boundary.word)
		if boundary.last != "" && !unicode.Is(unicode.Han, first) {
			lastInitial = MaskInitial(boundary.last)
		}
		label := SectionLabel(previousLast, boundary.word, boundary.last, boundary.initial, lastInitial, used)
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
