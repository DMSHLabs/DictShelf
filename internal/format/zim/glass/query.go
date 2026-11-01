// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// ErrNoStemmer says the index was built with a stemmer this reader does not
// carry, so its terms cannot be reproduced and the archive cannot be searched
// from here. It is not a failure of the archive: Xapian stems more languages
// than this package does (see stem.go), and an archive in one of them is
// perfectly searchable by Kiwix itself.
var ErrNoStemmer = errors.New("glass: the index is stemmed for a language this reader cannot stem")

// Hit is one article that contains every word of a query.
type Hit struct {
	DocID uint32
	// WDF is how many times the query's terms occur in the document, which is
	// enough to order results without reimplementing Xapian's ranking.
	WDF uint32
}

// Result is a search's answer.
type Result struct {
	Hits []Hit
	// Terms are the index terms the query was asked for, after stemming. They
	// are what the archive's own index holds, not the words the user typed.
	Terms []string
	// Skipped are the query's words that were left out as stopwords. The index
	// does not hold them anywhere by construction, so requiring one could only
	// return nothing.
	Skipped []string
	// Missing are the terms the index does not hold. If there is one, Hits is
	// empty: every term of a query has to be present in a document for it to
	// match.
	Missing []string
	// Complete says whether every term's posting list was read to its end. A
	// list that could not be is a subset of the answer, not the answer.
	Complete bool
}

// indexMeta is what the index says about how it was built, read once and kept.
type indexMeta struct {
	language string
	stem     Stemmer
	stops    map[string]bool
}

// Search finds the articles holding every word of the query.
//
// The words are folded the way libzim wrote them into the index - lowercased
// with combining marks removed, which is the recipe Xapian's term generator
// applies and the one dict.Fold already implements - and then stemmed with the
// index's own language, because the index holds stems and nothing else
// (stem.go). A document must contain all of the terms: Xapian's query parser
// runs with OP_AND (src/search.cpp). This index stores no positions, so phrases
// and proximity are not available here; a caller wanting them has to look at
// the article text.
func (db *DB) Search(query string) (Result, error) {
	words := FoldTerms(query)
	if len(words) == 0 {
		return Result{Complete: true}, nil
	}
	meta, err := db.indexMeta()
	if err != nil {
		return Result{}, err
	}
	words, skipped := dropStopwords(words, meta.stops)

	// Each word becomes the term the index holds for it. A word whose stem is
	// itself contributes one term; a word the stemmer changes contributes its
	// stem, and the word itself as a fallback for an index that turns out to
	// hold plain words after all - the lookup that finds nothing costs a B-tree
	// descent and settles the question where guessing could not.
	type wanted struct {
		word  string
		terms []string
		freq  uint32
	}
	asked := make([]wanted, 0, len(words))
	for _, word := range words {
		w := wanted{word: word, terms: termsFor(word, meta.stem)}
		for _, term := range w.terms {
			freq, err := db.TermFreq(term)
			if err != nil {
				return Result{}, err
			}
			if freq > 0 {
				w.freq = freq
				break
			}
		}
		asked = append(asked, w)
	}

	res := Result{Skipped: skipped, Complete: true}
	for _, w := range asked {
		if w.freq == 0 {
			res.Missing = append(res.Missing, w.word)
		} else {
			res.Terms = append(res.Terms, w.terms[0])
		}
	}
	if len(res.Missing) > 0 {
		return res, nil // nothing can contain them all
	}

	// Rarest term first: the accumulator stays as small as the query allows,
	// and each later list is intersected against it.
	sort.SliceStable(asked, func(i, j int) bool { return asked[i].freq < asked[j].freq })
	var acc []Hit
	for i, w := range asked {
		hits, empty, err := db.wordHits(w.terms)
		if err != nil {
			return Result{}, err
		}
		if !empty {
			res.Complete = false
		}
		if i == 0 {
			acc = hits
		} else {
			acc = intersect(acc, hits)
		}
		if len(acc) == 0 {
			res.Hits = nil
			return res, nil
		}
	}
	sort.Slice(acc, func(i, j int) bool { return acc[i].DocID < acc[j].DocID })
	res.Hits = acc
	return res, nil
}

// TermFreq is how many documents the index says hold this term, read from the
// head of its posting list without decoding it. It is the cheap question
// "does this term exist, and how common is it".
func (db *DB) TermFreq(term string) (uint32, error) {
	tag, found, err := db.keyTag(TablePostlist, []byte(term))
	if err != nil || !found {
		return 0, err
	}
	freq, _, err := unpackUint(tag, 0)
	return freq, err
}

// wordHits unions the posting lists of one word's candidate terms. empty
// reports whether every candidate term was read whole.
func (db *DB) wordHits(terms []string) (hits []Hit, empty bool, err error) {
	empty = true
	for _, term := range terms {
		t, err := db.Lookup(term)
		if err != nil {
			return nil, false, fmt.Errorf("glass: %q: %w", term, err)
		}
		if len(t.Postings) == 0 {
			continue
		}
		if !t.Complete {
			empty = false
		}
		for _, p := range t.Postings {
			hits = append(hits, Hit{DocID: p.DocID, WDF: p.WDF})
		}
		if len(terms) == 1 {
			break
		}
	}
	if len(hits) > 1 {
		sort.Slice(hits, func(i, j int) bool { return hits[i].DocID < hits[j].DocID })
		hits = coalesce(hits)
	}
	return hits, empty, nil
}

// coalesce merges neighbouring entries that name the same document, which is
// what a word carried by two of its candidate terms produces.
func coalesce(hits []Hit) []Hit {
	out := hits[:1]
	for _, h := range hits[1:] {
		if last := &out[len(out)-1]; last.DocID == h.DocID {
			last.WDF += h.WDF
			continue
		}
		out = append(out, h)
	}
	return out
}

// termsFor is the terms the index may hold for one folded word: its stem, and
// the word itself when stemming changed it.
func termsFor(word string, stem Stemmer) []string {
	s := stem(word)
	if s == "" || s == word {
		return []string{word}
	}
	return []string{s, word}
}

// dropStopwords removes the words the index deliberately did not index, unless
// that would leave nothing to ask for - asking for nothing has no meaning, and
// an index whose stopwords are the whole query is one Xapian searched with its
// stopper rolled back (queryparser_internal.cc does the same for a group of
// nothing but stopwords).
func dropStopwords(words []string, stops map[string]bool) (kept, skipped []string) {
	if len(stops) == 0 {
		return words, nil
	}
	for _, w := range words {
		if stops[w] {
			skipped = append(skipped, w)
		} else {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		return words, nil
	}
	return kept, skipped
}

// FoldTerms splits a query the way the index was written: on everything that is
// not a letter or a digit, folded (lowercased, accents removed), without empty
// pieces and without duplicates. What it does not do is stem - that needs the
// index's language, so it happens inside Search.
//
// The folding is dict.Fold, which is libzim's own recipe but for one class of
// marks: libzim removes every Unicode mark class (ICU's [:M:] is Mn, Mc and Me)
// while Fold removes non-spacing marks only. The two agree for the Latin,
// Cyrillic and Greek marks that carry accents - including the breve of the
// Russian "й", which both take off - so a Russian or English archive needs
// nothing more, but a spacing mark survives Fold and would not have survived the
// index build, and an archive in such a script (Tamil vowel signs, for one) can
// mismatch.
//
// A CJK archive is a larger case: libzim indexed it with Xapian's FLAG_NGRAMS
// (src/writer/xapianWorker.cpp:46), so what the index holds for a run of Chinese
// or Japanese is n-grams, not words, and splitting a query into words cannot
// match them. Nothing here handles that yet.
func FoldTerms(query string) []string {
	var out []string
	seen := map[string]bool{}
	split := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }
	for _, word := range strings.FieldsFunc(dict.Fold(query), split) {
		if word == "" || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	return out
}

// indexMeta reads the language and stopwords once and keeps them. A reader may
// be used from several goroutines, so the Once is what makes the first search
// safe rather than the caller.
func (db *DB) indexMeta() (*indexMeta, error) {
	db.metaOnce.Do(func() {
		m := &indexMeta{}
		language, err := db.Language()
		if err != nil {
			db.metaErr = err
			return
		}
		m.language = language
		ok := false
		if m.stem, ok = StemmerFor(language); !ok {
			db.metaErr = fmt.Errorf("%w: %q", ErrNoStemmer, language)
			return
		}
		words, err := db.Stopwords()
		if err != nil {
			db.metaErr = err
			return
		}
		m.stops = make(map[string]bool, len(words))
		for _, w := range words {
			for _, f := range FoldTerms(w) {
				m.stops[f] = true
			}
		}
		db.meta = m
	})
	return db.meta, db.metaErr
}

// intersect keeps the documents both lists hold, adding up their frequencies.
// Both lists are ascending by document id, as the index stores them and as
// wordHits leaves them.
func intersect(a, b []Hit) []Hit {
	out := a[:0]
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i].DocID == b[j].DocID:
			out = append(out, Hit{DocID: a[i].DocID, WDF: a[i].WDF + b[j].WDF})
			i++
			j++
		case a[i].DocID < b[j].DocID:
			i++
		default:
			j++
		}
	}
	return out
}

// IndexLanguage reports the language the index was built for, "" when it
// declares none. Exported so a caller can tell the user which archive cannot be
// searched, and why (see ErrNoStemmer).
func (db *DB) IndexLanguage() (string, error) {
	meta, err := db.indexMeta()
	if err != nil {
		return "", err
	}
	return meta.language, nil
}
