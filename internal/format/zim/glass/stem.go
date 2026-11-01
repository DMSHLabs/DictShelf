// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"strings"

	snowballRuntime "github.com/blevesearch/snowballstem"
	"github.com/blevesearch/snowballstem/arabic"
	"github.com/blevesearch/snowballstem/danish"
	"github.com/blevesearch/snowballstem/dutch"
	"github.com/blevesearch/snowballstem/english"
	"github.com/blevesearch/snowballstem/finnish"
	"github.com/blevesearch/snowballstem/french"
	"github.com/blevesearch/snowballstem/german"
	"github.com/blevesearch/snowballstem/hungarian"
	"github.com/blevesearch/snowballstem/irish"
	"github.com/blevesearch/snowballstem/italian"
	"github.com/blevesearch/snowballstem/norwegian"
	"github.com/blevesearch/snowballstem/porter"
	"github.com/blevesearch/snowballstem/portuguese"
	"github.com/blevesearch/snowballstem/romanian"
	"github.com/blevesearch/snowballstem/russian"
	"github.com/blevesearch/snowballstem/spanish"
	"github.com/blevesearch/snowballstem/swedish"
	"github.com/blevesearch/snowballstem/tamil"
	"github.com/blevesearch/snowballstem/turkish"
)

// A Kiwix full-text index does not hold the words of an article - it holds their
// stems. libzim builds it with Xapian's TermGenerator set to STEM_ALL, so every
// term in the postlist table is a Snowball stem and the unstemmed form is not
// there at all (src/writer/xapianWorker.cpp:49-58). A query has to be stemmed
// the same way or it asks the index for words it never held: in a Russian
// archive "джазовый" is 0 documents while "джазов", what its stemmer makes of
// it, is 162.
//
// The stemmer is not a choice - it follows from the index metadata. libzim runs
// the declared language through ICU, which turns an ISO 639-2 code into the
// two-letter ISO 639-1 one ("rus" into "ru"), and hands that to Xapian::Stem
// (src/writer/xapianIndexer.cpp:44; src/tools.cpp, getXapianStemmer). Xapian
// knows those codes and English names and nothing else, so a language outside
// its list gets an index with no stemmer at all: the words are stored plain,
// and searching them plain is right. That is why an index declaring Japanese or
// Thai is searched unstemmed rather than refused.
//
// The algorithms below are Snowball's own, compiled to Go by the Snowball
// compiler - the same sources Xapian generates languages/*.sbl from, under the
// Snowball licence (THIRD-PARTY-NOTICES.md). Xapian 1.4.32 can also stem
// Armenian, Basque, Catalan, Indonesian, Lithuanian and Nepali, plus four
// legacy variants (earlyenglish, german2, kraaij_pohlmann, lovins). This module
// carries none of those, so StemmerFor reports them as a language it cannot
// serve, and a caller must not search such an index with a stemmer picked by
// guess: it would find nothing and look like an empty archive.

// Stemmer turns one folded word into the form the index stores. It holds no
// state - each call builds its own Snowball environment - so one may be shared
// between goroutines.
type Stemmer func(word string) string

// snowball adapts one generated stemmer to Stemmer. The generated code is a
// function over an environment, so the environment carries the word in and the
// stem out.
func snowball(stem func(*snowballRuntime.Env) bool) Stemmer {
	return func(word string) string {
		env := snowballRuntime.NewEnv(word)
		stem(env)
		return env.Current()
	}
}

// stemmers is keyed by the two-letter ISO 639-1 code that ICU and Xapian agree
// on. Norwegian is here under all three codes Xapian accepts for it - no, nb
// (Bokmål) and nn (Nynorsk) are one algorithm there (languages/sbl-dispatch.h).
var stemmers = map[string]Stemmer{
	"ar": snowball(arabic.Stem),
	"da": snowball(danish.Stem),
	"de": snowball(german.Stem),
	"en": snowball(english.Stem),
	"es": snowball(spanish.Stem),
	"fi": snowball(finnish.Stem),
	"fr": snowball(french.Stem),
	"ga": snowball(irish.Stem),
	"hu": snowball(hungarian.Stem),
	"it": snowball(italian.Stem),
	"nb": snowball(norwegian.Stem),
	"nl": snowball(dutch.Stem),
	"nn": snowball(norwegian.Stem),
	"no": snowball(norwegian.Stem),
	"pt": snowball(portuguese.Stem),
	"ro": snowball(romanian.Stem),
	"ru": snowball(russian.Stem),
	"sv": snowball(swedish.Stem),
	"ta": snowball(tamil.Stem),
	"tr": snowball(turkish.Stem),

	// Xapian's second English stemmer, the original Porter one. It has no code
	// of its own - "en" is Porter2, which Xapian calls "english" - so it is
	// reachable by name only.
	porterCode: snowball(porter.Stem),
}

// xapianStems is every language Xapian 1.4.32 has a stemmer for, as the codes
// it accepts (languages/sbl-dispatch.h). It is what tells apart the two ways a
// language can be missing from stemmers: a language in here was stemmed when
// the index was built and we cannot reproduce it, while one absent here was
// never stemmed and its index is meant to be searched plain.
var xapianStems = map[string]bool{
	"ar": true, "ca": true, "da": true, "de": true, "en": true, "es": true,
	"eu": true, "fi": true, "fr": true, "ga": true, "hu": true, "hy": true,
	"id": true, "it": true, "lt": true, "nb": true, "ne": true, "nl": true,
	"nn": true, "no": true, "pt": true, "ro": true, "ru": true, "sv": true,
	"ta": true, "tr": true,
}

// porter is Xapian's second English stemmer, the original Porter one. It has no
// code of its own - "en" is Porter2, which Xapian calls "english" - so it is
// only reachable by name.
const porterCode = "porter"

// iso639_2 maps the three-letter codes an archive declares to the two-letter
// ones ICU would produce. Both /B and /T forms are here where they differ
// (ger/deu, fre/fra, dut/nld, rum/ron), because an archive may carry either.
// The list covers every language in xapianStems and no more: a language Xapian
// cannot stem needs no mapping, since it resolves to "unstemmed" either way.
var iso639_2 = map[string]string{
	"ara": "ar", "cat": "ca", "dan": "da", "deu": "de", "ger": "de",
	"eng": "en", "spa": "es", "eus": "eu", "baq": "eu", "fin": "fi",
	"fra": "fr", "fre": "fr", "gle": "ga", "hun": "hu", "hye": "hy",
	"arm": "hy", "ind": "id", "ita": "it", "lit": "lt", "nob": "nb",
	"nep": "ne", "nld": "nl", "dut": "nl", "nno": "nn", "nor": "no",
	"por": "pt", "ron": "ro", "rum": "ro", "rus": "ru", "swe": "sv",
	"tam": "ta", "tur": "tr",
}

// langNames maps Xapian's language names, which the archive's own Language tag
// may carry where the index has no language of its own.
var langNames = map[string]string{
	"arabic": "ar", "catalan": "ca", "danish": "da", "dutch": "nl",
	"english": "en", "finnish": "fi", "french": "fr", "german": "de",
	"hungarian": "hu", "indonesian": "id", "irish": "ga", "italian": "it",
	"lithuanian": "lt", "nepali": "ne", "norwegian": "no", "porter": porterCode,
	"portuguese": "pt", "romanian": "ro", "russian": "ru", "spanish": "es",
	"swedish": "sv", "tamil": "ta", "turkish": "tr",
}

// StemmerFor returns the stemmer the index declaring this language was built
// with, and whether we can search that index at all.
//
// Three outcomes, and the caller has to tell them apart:
//
//	stem, true   the language resolves to a stemmer we carry, or to no stemmer
//	             at all, which an empty or non-Xapian language means
//	nil, false   the language is one Xapian stemmed and we cannot - the index
//	             holds stems we cannot produce, so it cannot be searched from
//	             here and the caller must say so rather than return nothing
//
// The identity stemmer is returned for a language Xapian has no algorithm for:
// such an index was built with the stemmer call throwing, was left unstemmed,
// and holds plain words (src/writer/xapianWorker.cpp:49-56 wraps the stemmer in
// a try for exactly that case).
func StemmerFor(language string) (Stemmer, bool) {
	declared := strings.TrimSpace(language)
	if declared == "" {
		return identity, true
	}
	code := languageCode(declared)
	if code == "" {
		// A language we cannot place. Xapian would not have placed it either,
		// since every code it accepts is in the tables above, so its index has
		// no stemmer.
		return identity, true
	}
	if stem, ok := stemmers[code]; ok {
		return stem, true
	}
	if xapianStems[code] {
		return nil, false // Xapian stemmed this index; we cannot reproduce it
	}
	return identity, true
}

// identity is the stemmer of an index built without one.
func identity(word string) string { return word }

// languageCode reduces what an archive declares to a key of stemmers: an ICU
// locale name, an ISO 639-1 or 639-2 code, or a Xapian language name, all of
// which may mean the same language. "" means it is none of those.
func languageCode(language string) string {
	s := strings.ToLower(language)
	// A locale carries a region and sometimes an encoding; ICU keeps only the
	// language, so "ru_RU" and "ru-RU.UTF-8" are both "ru".
	if i := strings.IndexAny(s, "_-."); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return ""
	}
	if _, ok := stemmers[s]; ok {
		return s
	}
	if code, ok := iso639_2[s]; ok {
		return code
	}
	if code, ok := langNames[s]; ok {
		return code
	}
	return ""
}
