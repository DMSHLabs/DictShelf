// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// The two readings an archive's index cannot answer by itself.
//
// A Kiwix index has no positions: libzim indexed the articles with
// index_text_without_positions, and the position table in the archive is empty.
// Positions are what a phrase and a distance are made of, so "are these words
// adjacent, in this order" and "are they within ten of each other" cannot be
// ASKED of that index. They can still be ANSWERED - by verifying them in the
// article text of the candidates the index does return, which is what this file
// does.
//
// Two consequences worth stating plainly, because they are properties of the
// answer and not of the implementation:
//
//   - The check is literal: the words are compared as folded text (lowercased,
//     accents removed), so an inflected form does not match. That is what the
//     word "exact" means to a reader, and it is how our own FTS5 index behaves
//     too - it is not stemmed either.
//   - The check is bounded: only maxCheck candidates are read, in the archive's
//     rank order. A phrase in a handful of articles is found completely; a query
//     matching a million articles is answered from the head of that list, and
//     the count is what was checked, not what exists.
//
// Markup inside the text is treated as a separator, so a phrase broken by a tag
// is found when the tag sat between words ("a <i>b</i>") and missed when the
// tag glued them ("a<br>b"). A miss is the safe direction: it never claims a
// phrase the article does not have in words.

package zim

import (
	"bytes"
	"html"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
)

// FullTextPhrase answers with the articles that hold the words of query
// adjacently, in that order, best first.
func (d *Dict) FullTextPhrase(query string, limit, maxCheck int) ([]dict.Result, error) {
	words := glass.FoldTerms(query)
	if len(words) == 0 {
		return nil, nil
	}
	return d.verifiedFullText(query, limit, maxCheck, func(tokens []string) bool {
		return phraseIn(tokens, words)
	})
}

// FullTextNear answers with the articles that hold the words of query within
// distance other words of one another, in any order, best first.
func (d *Dict) FullTextNear(distance int, query string, limit, maxCheck int) ([]dict.Result, error) {
	words := glass.FoldTerms(query)
	if len(words) == 0 {
		return nil, nil
	}
	return d.verifiedFullText(query, limit, maxCheck, func(tokens []string) bool {
		return nearIn(tokens, words, distance)
	})
}

// verifiedFullText is the shared walk: the archive's own ranking decides which
// candidates are looked at, and keep decides which of them answer.
func (d *Dict) verifiedFullText(query string, limit, maxCheck int, keep func(tokens []string) bool) ([]dict.Result, error) {
	window, size, err := d.XapianIndex()
	if err != nil {
		return nil, err
	}
	db, err := glass.OpenSource(window, size)
	if err != nil {
		return nil, err
	}
	res, err := db.Search(query)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if maxCheck <= 0 || maxCheck > len(res.Hits) {
		maxCheck = len(res.Hits)
	}
	var out []dict.Result
	checked := 0
	for _, h := range res.Hits {
		if checked >= maxCheck || len(out) >= limit {
			break
		}
		path, err := db.DocPath(h.DocID)
		if err != nil {
			continue
		}
		name := strings.TrimPrefix(path, "C/")
		i, de, ok := d.c.find(d.c.contentNS(), name)
		if !ok || !d.c.isArticle(de) {
			continue
		}
		if de.isRedirect() {
			if _, de, err = d.c.resolve(i); err != nil {
				continue
			}
		}
		body, err := d.c.content(de)
		if err != nil || len(body) == 0 {
			continue
		}
		checked++
		if !keep(articleTokens(body)) {
			continue
		}
		src, err := d.c.entry(i)
		if err != nil {
			continue
		}
		out = append(out, dict.Result{Headword: src.headword(), Body: d.articleHTML(de, body)})
	}
	return out, nil
}

// articleTokens is the article as folded words, with markup treated as a
// separator. Entities are decoded first, so "&nbsp;" separates words the way
// the space it stands for does.
func articleTokens(body []byte) []string {
	var text bytes.Buffer
	text.Grow(len(body))
	skip := 0
	for i := 0; i < len(body); i++ {
		switch c := body[i]; {
		case c == '<':
			// A tag, or a comment; find its end and read its name to drop the
			// content of the two elements that hold no prose.
			end := bytes.IndexByte(body[i:], '>')
			if end < 0 {
				i = len(body)
				continue
			}
			tag := strings.ToLower(strings.TrimSpace(string(body[i+1 : i+end])))
			name, _, _ := strings.Cut(tag, " ")
			name = strings.Trim(name, "/")
			switch {
			case name == "script" || name == "style":
				skip++
			case strings.HasPrefix(tag, "/") && (name == "script" || name == "style"):
				if skip > 0 {
					skip--
				}
			}
			text.WriteByte(' ')
			i += end
		case skip > 0:
			// inside a script or a style
		default:
			text.WriteByte(c)
		}
	}
	return glass.FoldTerms(html.UnescapeString(text.String()))
}

// phraseIn reports whether the words appear consecutively, in that order.
func phraseIn(tokens, words []string) bool {
	if len(words) == 0 || len(tokens) < len(words) {
		return false
	}
	for i := 0; i+len(words) <= len(tokens); i++ {
		match := true
		for j, w := range words {
			if tokens[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// nearIn reports whether every pair of the words occurs with at most distance
// tokens between them, in any order - the rule FTS5's NEAR applies, so the two
// kinds of dictionary answer the same question the same way.
func nearIn(tokens, words []string, distance int) bool {
	if len(words) == 0 {
		return false
	}
	if len(words) == 1 {
		return containsToken(tokens, words[0])
	}
	at := map[string][]int{}
	for i, t := range tokens {
		at[t] = append(at[t], i)
	}
	for i := 0; i < len(words); i++ {
		if len(at[words[i]]) == 0 {
			return false
		}
		for j := i + 1; j < len(words); j++ {
			if !pairWithin(at[words[i]], at[words[j]], distance) {
				return false
			}
		}
	}
	return true
}

// pairWithin reports whether some occurrence of each word lies within distance
// tokens of an occurrence of the other. Both positions are ascending, so one
// walk of the shorter list answers it.
func pairWithin(a, b []int, distance int) bool {
	span := distance + 1
	for _, pa := range a {
		for _, pb := range b {
			d := pa - pb
			if d < 0 {
				d = -d
			}
			if d <= span {
				return true
			}
			if pb > pa+span {
				break
			}
		}
	}
	return false
}

func containsToken(tokens []string, word string) bool {
	for _, t := range tokens {
		if t == word {
			return true
		}
	}
	return false
}

// Reading is which reading of a query a caller wants from the archive.
type Reading int

const (
	// ReadingWords is what the index holds: the words, anywhere in the article.
	ReadingWords Reading = iota
	// ReadingPhrase is the words adjacent, in order.
	ReadingPhrase
	// ReadingNear is the words within ReadingNearDistance of one another.
	ReadingNear
)

// ReadingNearDistance is how many other words may sit between two words of a
// "near" reading. It is FTS5's default, and the number our own index answers
// with, so both kinds of dictionary mean the same thing by the word.
const ReadingNearDistance = 10

// StreamHeadwords answers a query with the headwords of the articles that
// satisfy the reading, best first, reading no article that the reading does not
// need to look at.
//
// maxCheck bounds the articles the strict readings are allowed to read (the
// words reading ignores it: the index already holds that answer). It is the
// honest half of the promise - a phrase is verified in the candidates this
// costs, and a caller that hits the bound knows its list is a head of the
// answer rather than the answer.
func (d *Dict) StreamHeadwords(query string, reading Reading, maxCheck int, emit func(headword string) error) error {
	if reading == ReadingWords {
		return d.streamHeadwords(query, 0, nil, emit)
	}
	words := glass.FoldTerms(query)
	if len(words) == 0 {
		return nil
	}
	keep := func(tokens []string) bool { return phraseIn(tokens, words) }
	if reading == ReadingNear {
		keep = func(tokens []string) bool { return nearIn(tokens, words, ReadingNearDistance) }
	}
	return d.streamHeadwords(query, maxCheck, keep, emit)
}

// streamHeadwords is the walk both readings share: the archive's rank order,
// the article read only when keep is given, and the bound applied to the
// articles read rather than to the headwords emitted.
func (d *Dict) streamHeadwords(query string, maxCheck int, keep func(tokens []string) bool, emit func(headword string) error) error {
	window, size, err := d.XapianIndex()
	if err != nil {
		return err
	}
	db, err := glass.OpenSource(window, size)
	if err != nil {
		return err
	}
	res, err := db.Search(query)
	if err != nil {
		return err
	}
	checked := 0
	for _, h := range res.Hits {
		if keep != nil {
			if maxCheck > 0 && checked >= maxCheck {
				return nil
			}
		}
		path, err := db.DocPath(h.DocID)
		if err != nil {
			continue
		}
		name := strings.TrimPrefix(path, "C/")
		i, de, ok := d.c.find(d.c.contentNS(), name)
		if !ok || !d.c.isArticle(de) {
			continue
		}
		if de.isRedirect() {
			if _, de, err = d.c.resolve(i); err != nil {
				continue
			}
		}
		if keep != nil {
			body, err := d.c.content(de)
			if err != nil || len(body) == 0 {
				continue
			}
			checked++
			if !keep(articleTokens(body)) {
				continue
			}
		}
		if err := emit(de.headword()); err != nil {
			return err
		}
	}
	return nil
}

// ReadingHeadwordsFor is the reading a caller asked for by name, as the two
// APIs above take it: the search modes and this package's readings are the same
// three ideas, and agreeing on a name is cheaper than agreeing on a translation.
func ReadingFor(mode string) (Reading, bool) {
	switch mode {
	case "fts", "words":
		return ReadingWords, true
	case "phrase":
		return ReadingPhrase, true
	case "near":
		return ReadingNear, true
	}
	return ReadingWords, false
}
