// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// The two readings an archive's index cannot answer by itself.
//
// A Kiwix index has no positions: libzim indexed the articles with
// index_text_without_positions, and the position table in the archive is empty.
// Positions are what a phrase and a distance are made of, so "are these words
// adjacent, in this order" and "are they within the near window" cannot be
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
	"errors"
	"fmt"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
	"github.com/DMSHLabs/DictShelf/internal/htmlref"
)

// ErrCheckBound is returned by a strict reading that stopped at its bound with
// candidates left unread. It is an answer and a limit at once: the walk has
// already produced everything it found, and the caller decides what to say about
// the part it did not look at. Reporting a bounded list as the whole answer is
// the one thing this must not do - the bound is invisible to the reader
// otherwise, and a phrase that is in an unread article would be called absent.
var ErrCheckBound = errors.New("the strict reading reached its bound with candidates unread")

// FullTextPhrase answers with the articles that hold the words of query
// adjacently, in that order, best first.
//
// It returns ErrCheckBound, wrapped around the results it did find, when the
// bound stopped the walk. The collecting result has nowhere to carry that, so
// callers that must report it - the search's streaming path - read the
// streaming form instead.
func (d *Dict) FullTextPhrase(query string, limit, maxCheck int) ([]dict.Result, error) {
	words := glass.FoldText(query)
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
	words := glass.FoldText(query)
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
	res, err := d.searchIndex(db, query)
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
	var bound error
	records := d.watch("archive-records")
	defer records.Done()
	for _, h := range res.Hits {
		if checked >= maxCheck || len(out) >= limit {
			if checked >= maxCheck && checked < len(res.Hits) {
				bound = fmt.Errorf("%w: %d candidates, %d read", ErrCheckBound, len(res.Hits), checked)
			}
			break
		}
		records.Moved(int64(h.DocID))
		path, err := db.DocPath(h.DocID)
		if err != nil {
			continue
		}
		name := strings.TrimPrefix(path, "C/")
		i, de, ok := d.c.find(d.c.contentNS(), name)
		if !ok || !d.c.isArticle(i, de) {
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
	return out, bound
}

// articleTokens is the article as folded words, with markup treated as a
// separator.
//
// The markup is stripped by htmlref.PlainText, which is the same function the
// article index of OUR own databases is built with (the index column is
// StripHTML(body)): the two answers to "what does this article say" have to
// agree, or the reading verified here would disagree with the one a prepared
// database gives for the same article. This package used to carry its own
// stripper, and it was wrong in a way that made the strict readings almost
// empty: it counted every </script> as an opening tag, so the skip it kept
// never ended and the whole of the article after the first script or style
// block was dropped. Phrase and near search then matched only what the head of
// the document holds - the title - which is why single words worked and a real
// phrase found nothing (found on a live Wikipedia article, 2026-11-02).
//
// The title is part of what this returns, because the archive's own index
// indexes it too: a phrase whose words stand in the title has to verify here
// the same way it verifies there.
func articleTokens(body []byte) []string {
	return glass.FoldText(htmlref.PlainText(string(body)))
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
	// The query as a SEQUENCE, not as a set: these readings ask where the words
	// stand relative to one another, and a word the reader typed twice is two
	// words standing somewhere.
	words := glass.FoldText(query)
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
	res, err := d.searchIndex(db, query)
	if err != nil {
		return err
	}
	checked := 0
	records := d.watch("archive-records")
	defer records.Done()
	for _, h := range res.Hits {
		if keep != nil && maxCheck > 0 && checked >= maxCheck {
			// The bound, and the honest half of it: the caller is told that the
			// list it received is a head of the answer. It used to return nil
			// here, exactly like a finished walk.
			return fmt.Errorf("%w: %d candidates, %d read", ErrCheckBound, len(res.Hits), checked)
		}
		records.Moved(int64(h.DocID))
		path, err := db.DocPath(h.DocID)
		if err != nil {
			continue
		}
		name := strings.TrimPrefix(path, "C/")
		i, de, ok := d.c.find(d.c.contentNS(), name)
		if !ok || !d.c.isArticle(i, de) {
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
