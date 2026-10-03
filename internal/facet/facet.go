// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package facet derives the handful of GROUPS a dictionary belongs to, from
// what is already known about it: its declared language, its file and folder
// names, and its own title. Nothing here opens a dictionary, reads an index or
// touches the disk - Derive is a pure function over strings the caller already
// has, which is why it can run inside the /api/dicts fan-out at no cost.
//
// The groups exist for one purpose: to let the dictionary picker offer "every
// English dictionary" or "the encyclopedias" as a single choice, instead of a
// flat list of a hundred names. They are a CONVENIENCE built out of evidence,
// not a classification the app believes in. Two rules follow from that, and
// both are load-bearing:
//
//  1. Derive, never persist. Nothing here is written to state.json or to a
//     library folder, so a better rule tomorrow reclassifies everything on the
//     next load and no user ever has to undo a stale verdict.
//
//  2. Never invent a value from absence. A dictionary that says nothing about
//     its language joins no language group; it does not become English. The
//     search path may assume English for lemmatization because that guess is
//     invisible and validated against a real headword index (see
//     lang.Resolve), but a group label is SHOWN, and one visibly misfiled
//     dictionary discredits every correctly filed one.
//
// A group is a (facet, value) pair. A dictionary may hold several - an Oxford
// English-Russian encyclopedia is in four. The client hides only a value that
// holds every dictionary: it is "All dictionaries" under another name (D162).
package facet

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/lang"
)

// Group is one (facet, value) membership, carrying its own labels so the
// client holds no taxonomy at all: it renders what it is given and never has
// to know that "lang" means language or that "en-ru" is a pair of them
// (D102 - none of these internal ids reach a user's eye).
//
// FO is the facet's rank in the picker. It travels per row because a row is
// the only thing this API sends: the facets that exist are not known until the
// last dictionary has resolved, so there is no header line to put an order in.
type Group struct {
	F  string `json:"f"`  // facet id: lang | pair | a groups.ini section, lower-cased
	FL string `json:"fl"` // facet label, as the picker heads the group
	FO int    `json:"fo"` // facet rank, ascending
	V  string `json:"v"`  // value id, unique within the facet
	VL string `json:"vl"` // value label, as the picker names the choice
}

// Input is everything Derive is allowed to look at.
//
// Path must already be the path language conventions are written on - for a
// prepared dictionary that is the library FOLDER, not the text.db inside it
// (the server's langPath does that reduction).
type Input struct {
	Name     string   // the dictionary's own title
	Path     string   // source path, or library folder
	Roots    []string // configured dictionary directories, to bound the folder walk
	Declared string   // what the format declares for the HEADWORD language
	Contents string   // what it declares for the ARTICLE language (DSL, BGL)
	Rules    *Rules   // the groups.ini in effect; nil is the embedded default
	File     string   // the file (or library folder) name as on disk; "" takes Path's base
}

const (
	// The user's own groups.ini sections - any the built-in list does not
	// have - rank first, above Language, in file order: they are what the
	// user came to the picker for. The built-in sections follow Language pair.
	foMine  = -maxSections
	foLang  = 1
	foPair  = 2
	foRules = 3
)

// Derive returns the groups a dictionary belongs to, in facet order. nil when
// nothing could be established, which is a normal answer.
func Derive(in Input) []Group {
	var out []Group
	if code := lang.Resolve(in.Declared, in.Path, in.Roots, in.Name); code != "" {
		out = append(out, Group{F: "lang", FL: "Language", FO: foLang, V: code, VL: lang.Name(code)})
	}
	if v, l := languagePair(in); v != "" {
		out = append(out, Group{F: "pair", FL: "Language pair", FO: foPair, V: v, VL: l})
	}
	rules := in.Rules
	if rules == nil {
		rules = Default()
	}
	// The title and the file name: a collection named by its folder or file
	// ("oald9.mdx", "Wikipedia_es") says what it is there as often as in its
	// title.
	file := in.File
	if file == "" && in.Path != "" {
		file = filepath.Base(in.Path)
	}
	out = append(out, rules.match(in.Name, file)...)
	slices.SortStableFunc(out, func(a, b Group) int { return a.FO - b.FO })
	return out
}

// languagePair names the two languages a dictionary joins, from an EXPLICIT
// pair only: two declared fields, or two language tokens joined the way a
// title, a file name or a folder name joins them ("English-Russian", "es-es",
// "En-Ru/", "英汉"). A single language hint says nothing about this - "Oxford
// English Dictionary" and "Oxford English-Russian" both resolve to English -
// so a dictionary with one hint and no pair joins no group here.
//
// Not a mono/bi "Type" facet (D149): "Bilingual" would gather every pair into
// one choice nobody searches with; a pair is the unit a reader actually
// picks by - the dictionaries that explain English in Russian. The pair is
// UNDIRECTED: en-ru and ru-en are one value. Together they are the set a
// reader of that pair wants, and a query only matches the headwords of the
// side it is written in anyway.
func languagePair(in Input) (string, string) {
	a, b := lang.FromDeclared(in.Declared), lang.FromDeclared(in.Contents)
	if a == "" || b == "" {
		a, b = pair(in.Name)
	}
	if a == "" || b == "" {
		a, b = pair(dict.Name(in.Path))
	}
	if a == "" || b == "" {
		a, b = folderPair(in.Path, in.Roots)
	}
	switch {
	case a == "" || b == "":
		return "", ""
	case a == b:
		// es-es, spa-spa, "English-English", 英英: a pair that names the same
		// language twice is the standard way a monolingual dictionary is
		// labelled, and it is stated, not inferred.
		return a, "Monolingual " + lang.Name(a)
	}
	if a > b {
		a, b = b, a // the id is order-free: en-ru and ru-en are one group
	}
	na, nb := lang.Name(a), lang.Name(b)
	if strings.ToLower(nb) < strings.ToLower(na) {
		na, nb = nb, na
	}
	return a + "-" + b, na + " ↔ " + nb
}

// ArticleLang is the language the dictionary's ARTICLE BODIES are written in -
// the second half of a pair - as an ISO 639-1 code, or "" when nothing says.
// It is the default voice for reading a selection aloud; the client refines it
// per selection by script, choosing between this and the headword language,
// because a bilingual article quotes both.
//
// Evidence, first hit wins: the declared contents language; the second code
// of a pair in the title ("Oxford Russian-English"), then in the file name
// ("eng-eus", "en_fr"), then in an enclosing folder name up to the configured
// root. A dictionary with no pair anywhere is taken to be monolingual, so its
// headword language answers. Unlike a group label this value is never shown -
// it picks a voice, and the reader overrides a wrong one with a tap - which is
// why the monolingual assumption is acceptable here and not in Derive.
func ArticleLang(in Input) string {
	if c := lang.FromDeclared(in.Contents); c != "" {
		return c
	}
	if _, b := pair(in.Name); b != "" {
		return b
	}
	if _, b := pair(dict.Name(in.Path)); b != "" {
		return b
	}
	if _, b := folderPair(in.Path, in.Roots); b != "" {
		return b
	}
	return lang.Resolve(in.Declared, in.Path, in.Roots, in.Name)
}

// folderPair is the pair the nearest enclosing folder is named as
// ("En-Ru/apresyan.dsl"). The walk stops at the configured root it is
// under, root included, exactly as the headword-language folder walk does; a
// path under no root has only its own folder read. Strings only - no disk.
func folderPair(path string, roots []string) (string, string) {
	if path == "" {
		return "", ""
	}
	dir := filepath.Clean(filepath.Dir(path))
	inside := false
	for _, r := range roots {
		if r == "" {
			continue
		}
		r = filepath.Clean(r)
		if dir == r || strings.HasPrefix(dir, r+string(filepath.Separator)) {
			inside = true
			break
		}
	}
	for {
		if a, b := pair(filepath.Base(dir)); b != "" {
			return a, b
		}
		if !inside || slices.ContainsFunc(roots, func(r string) bool {
			return r != "" && filepath.Clean(r) == dir
		}) {
			return "", ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// pairRe finds two letter tokens joined by one of the separators a language
// pair is written with. The tokens are deliberately unanchored to the whole
// string - a title puts the pair anywhere ("Oxford Russian-English Dictionary",
// "Collins (En-Es) 2nd ed.") - and both sides must resolve to a language, which
// is what keeps "Anglo-Saxon", "Latin-American" and "e-book" out.
var pairRe = regexp.MustCompile(`(?i)(\p{L}{2,})[ \t]*[-\x{2013}\x{2014}>_/][ \t]*(\p{L}{2,})`)

func pair(s string) (string, string) {
	for _, m := range pairRe.FindAllStringSubmatch(s, -1) {
		a, b := lang.Normalize(m[1]), lang.Normalize(m[2])
		if a != "" && b != "" {
			return a, b
		}
	}
	return hanPair(s)
}

// hanPair reads the pair a Chinese or Japanese title writes with no separator
// at all: one character per language, "英汉" (English-Chinese), "汉英", "俄汉",
// "英和" (English-Japanese), "英英" (monolingual). Two characters that both
// name a language are not yet a pair - "语和汉语" would read as one - so the
// pair must be followed by the word the title is naming: 词典/辞典/字典/辞書,
// optionally sized (大/中/小), or 双解 ("bilingual explanations"). That is how
// "牛津高阶英汉双解词典" and "新英和中辞典" are written, and it is what keeps a
// sentence out.
//
// Only characters that are unambiguous in that position are in the table. 西
// (Spanish) is not: 西汉 is also the Western Han dynasty. 和 names Japanese
// only against 英, the one pairing it is used in; elsewhere it is "and".
var hanPairRe = regexp.MustCompile(`([英汉漢中俄日法德韩韓意葡拉和])([英汉漢中俄日法德韩韓意葡拉和])(?:[大中小]?(?:词典|詞典|辞典|辭典|字典|辞書|辭書)|双解|雙解)`)

var hanLang = map[string]string{
	"英": "en", "汉": "zh", "漢": "zh", "中": "zh", "俄": "ru", "日": "ja", "法": "fr",
	"德": "de", "韩": "ko", "韓": "ko", "意": "it", "葡": "pt", "拉": "la", "和": "ja",
}

func hanPair(s string) (string, string) {
	for _, m := range hanPairRe.FindAllStringSubmatch(s, -1) {
		if (m[1] == "和" || m[2] == "和") && m[1]+m[2] != "英和" && m[1]+m[2] != "和英" {
			continue
		}
		return hanLang[m[1]], hanLang[m[2]]
	}
	return "", ""
}
