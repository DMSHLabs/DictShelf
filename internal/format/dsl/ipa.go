// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Legacy-font transcriptions (D165).
//
// Old Lingvo dictionaries wrote [t] transcriptions in a custom font whose
// glyphs sat on cp1251 code points: decoded as text, "kæt" reads "k‡t" and
// "ʃɪp" reads "ЏэP". GoldenDict maps those code points back to IPA in every
// [t] zone (goldendict-ng src/dict/dsl_details.cc, ArticleDom), which also
// rewrites modern transcriptions - pinyin "ma3" becomes "maĩ". Here a zone is
// mapped only when it shows the legacy font: a symbol that neither IPA nor
// prose puts in a transcription, or Cyrillic and Latin letters in one zone. A
// Russian-letter transcription ("кэт") is all Cyrillic and stays as written.

// legacyIPAMap is GoldenDict's table, with one correction: Ћ is θ, where
// GoldenDict writes the look-alike Cyrillic Ө (its own commented-out line has
// θ).
var legacyIPAMap = map[rune]string{
	0x2021: "æ", 0x0407: "r", 0x00B0: "k", 0x20AC: "ɔ", 0x0404: "z", 0x040F: "ʃ",
	0x00AB: "t", 0x00AC: "d", 0x2020: "ə", 0x0490: "m", 0x00A7: "f", 0x00AE: "l",
	0x00B1: "g", 0x045E: "e", 0x00AD: "n", 0x00A9: "s", 0x00A6: "w", 0x2026: "ʌ",
	0x0452: "v", 0x0408: "p", 0x040C: "u", 0x0406: "h", 0x00B5: "a", 0x0491: "ɛ",
	0x040A: "ŋ", 0x2030: "ð", 0x0456: "j", 0x00A4: "b", 0x0409: "ʒ", 0x040E: "i",
	0x040B: "θ", 0x00B6: "ʊ", 0x2018: "ɑ", 0x0457: "ɥ", 0x0458: "œ",
	0x0405: "œ̃", 0x0441: "ɲ", 0x0442: "ɔ̃", 0x0443: "ø",
	0x0445: "ɛ̃", 0x0446: "ç", 0x044C: "ɑ̃", 0x044D: "ɪ", 0x044F: "ɒ",
	'0': "β", '1': "ẽ", '2': "ɜ", '3': "ĩ", '4': "õ", '6': "ʎ", '7': "ɣ",
	'8': "ǝ", ':': "ː", '\'': "ˈ", 0x0455: "ǐ", 0x00B7: "ã", 0x00A0: "ʧ",
	0x0402: "i:", 0x0403: "ɑ:", 0x0428: "a", 0x0453: "u:", 0x201A: "ɔ", 0x201E: "ə",
	0x2039: "dʒ",
}

// legacySentinels are the mapped symbols that do not occur in a modern
// transcription. Left out on purpose: `…` `‘` `«` `·` and the soft hyphen,
// which do (an ellipsis, a quote used as a stress mark, a syllable dot).
const legacySentinels = "§©®°±µ¶¤¦¬†‡€‰‚„‹"

// markLegacyIPA flags the text of a [t] zone for mapping when it is written
// in the legacy font.
func (tr *transformer) markLegacyIPA(z *zone) {
	if !isLegacyIPA(tr.zoneText(z)) {
		return
	}
	for i := z.open + 1; i < z.end; i++ {
		if tr.toks[i].kind == tkText {
			tr.toks[i].ipa = true
		}
	}
}

// isLegacyIPA requires the whole zone to be in the font's alphabet: ASCII
// and the code points it reused. A zone holding anything else - real IPA, a
// typographic quote, a Cyrillic letter the font never used - was written in
// Unicode, and mapping it would corrupt it. Within that alphabet a sentinel,
// or a reused Cyrillic code point next to a Latin letter, shows the font.
func isLegacyIPA(s string) bool {
	sentinel, cyr, lat := false, false, false
	for _, r := range s {
		switch {
		case r < utf8.RuneSelf:
			lat = lat || unicode.IsLetter(r)
		case legacyIPAMap[r] == "":
			return false
		case strings.ContainsRune(legacySentinels, r):
			sentinel = true
		case r >= 0x0400 && r <= 0x04FF:
			cyr = true
		}
	}
	return sentinel || cyr && lat
}

// legacyIPA maps one run of legacy-font text to IPA.
func legacyIPA(s string) string {
	var b strings.Builder
	mapped := false
	for i, r := range s {
		m, ok := legacyIPAMap[r]
		switch {
		case ok && !mapped:
			mapped = true
			b.Grow(len(s) + 8)
			b.WriteString(s[:i])
			b.WriteString(m)
		case ok:
			b.WriteString(m)
		case mapped:
			b.WriteRune(r)
		}
	}
	if !mapped {
		return s
	}
	return b.String()
}
