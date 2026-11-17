// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"unicode"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"golang.org/x/text/unicode/norm"
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
	// WDF is how many times the query's terms occur in the document, added up
	// over them. It is not what orders results - Score is - but it is the cheap
	// "how often" behind them, and the one number that survives from a list read
	// without ranking.
	WDF uint32
	// DocLen is the document's length as the index records it, filled while
	// scoring and zero when the length list could not be read.
	DocLen uint32
	// Score is Xapian's own BM25 for this document and this query (bm25.go).
	// Larger is better. Two hits with the same score are ordered by DocID, so
	// the answer is stable.
	Score float64
}

// Result is a search's answer. Hits are ranked, best first (bm25.go), and
// callers should keep that order: Score is Xapian's own BM25, and re-sorting by
// document id or by how often a word occurs throws away the ranking.
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
//
// Hits come back ranked, best first, by Xapian's own BM25 (bm25.go) - the
// ranking the index was built for, and the one that keeps a long article that
// merely repeats a word from outranking the article that is about it.
func (db *DB) Search(query string) (Result, error) {
	return db.SearchContext(context.Background(), query)
}

// SearchContext searches the same index and ranking as Search, stopping its walks
// when the caller cancels. It returns no partial answer on cancellation.
func (db *DB) SearchContext(ctx context.Context, query string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
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

	// Rarest word first: the accumulator stays as small as the query allows, and
	// each later list is intersected against it.
	sort.SliceStable(asked, func(i, j int) bool { return asked[i].freq < asked[j].freq })
	lists := make([][]Hit, len(asked))
	var acc []Hit
	for i, w := range asked {
		var hits []Hit
		var empty bool
		var err error
		if i > 0 && len(acc) <= int(w.freq)/sparsePostingRatio {
			hits, err = db.selectedWordHits(ctx, w.terms, acc)
			empty = true
			// A damaged selected chunk must retain the collecting reader's
			// partial-answer/error handling. Cancellation never retries.
			if err != nil && ctx.Err() == nil {
				hits, empty, err = db.wordHits(ctx, w.terms)
			}
		} else {
			hits, empty, err = db.wordHits(ctx, w.terms)
		}
		if err != nil {
			return Result{}, err
		}
		if !empty {
			res.Complete = false
		}
		lists[i] = hits
		if i == 0 {
			// A copy, not the list itself: the intersection writes through its first
			// argument, and every word's list is needed again for scoring.
			acc = append([]Hit(nil), hits...)
		} else {
			acc, err = intersectContext(ctx, acc, hits)
			if err != nil {
				return Result{}, err
			}
		}
		if len(acc) == 0 {
			res.Hits = nil
			return res, nil
		}
	}
	// Rank by Xapian's own BM25, which needs every document's length - the one
	// extra pass, walked in step with the hits (bm25.go, rank.go).
	if err := db.assignDocLengthsContext(ctx, acc); err != nil {
		return Result{}, err
	}
	lenFactor := bm25LengthFactor(db.Stats.DocCount, db.Stats.TotalDocLen)
	for i := range asked {
		weight := bm25TermWeight(db.Stats.DocCount, asked[i].freq)
		// A merge, not a lookup per hit: both sides are ascending, so the whole
		// pass is O(hits + candidates) with nothing allocated.
		j := 0
		for k := range acc {
			if k&1023 == 0 {
				if err := ctx.Err(); err != nil {
					return Result{}, err
				}
			}
			for j < len(lists[i]) && lists[i][j].DocID < acc[k].DocID {
				j++
				if j&1023 == 0 {
					if err := ctx.Err(); err != nil {
						return Result{}, err
					}
				}
			}
			if j < len(lists[i]) && lists[i][j].DocID == acc[k].DocID {
				acc[k].Score += bm25Part(weight, lists[i][j].WDF, acc[k].DocLen, lenFactor)
			}
		}
	}
	// Best first, and by document id within a tie so the answer is stable.
	if err := sortHits(ctx, acc, func(i, j int) bool {
		if acc[i].Score != acc[j].Score {
			return acc[i].Score > acc[j].Score
		}
		return acc[i].DocID < acc[j].DocID
	}); err != nil {
		return Result{}, err
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
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
func (db *DB) wordHits(ctx context.Context, terms []string) (hits []Hit, empty bool, err error) {
	empty = true
	for _, term := range terms {
		t, err := db.lookupContext(ctx, term)
		if err != nil {
			return nil, false, fmt.Errorf("glass: %q: %w", term, err)
		}
		if len(t.Postings) == 0 {
			continue
		}
		if !t.Complete {
			empty = false
		}
		for i, p := range t.Postings {
			if i&1023 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, false, err
				}
			}
			hits = append(hits, Hit{DocID: p.DocID, WDF: p.WDF})
		}
		if len(terms) == 1 {
			break
		}
	}
	if len(hits) > 1 {
		if err := sortHits(ctx, hits, func(i, j int) bool { return hits[i].DocID < hits[j].DocID }); err != nil {
			return nil, false, err
		}
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

// FoldTerms converts a query into the set of folded terms the index holds,
// preserving first appearance and omitting duplicates. Runs in Xapian's
// unbroken CJK ranges become alternating unigrams and bigrams; ordinary words
// remain whole. What it does not do is stem - that needs the index's language,
// so it happens inside Search.
//
// The folding is dict.Fold, which is libzim's own recipe but for one class of
// marks: libzim removes every Unicode mark class (ICU's [:M:] is Mn, Mc and Me)
// while Fold removes non-spacing marks only. The two agree for the Latin,
// Cyrillic and Greek marks that carry accents - including the breve of the
// Russian "Р в„–", which both take off - so a Russian or English archive needs
// nothing more, but a spacing mark survives Fold and would not have survived the
// index build, and an archive in such a script (Tamil vowel signs, for one) can
// mismatch.
//
// CJK indexes use Xapian's FLAG_CJK_NGRAM (libzim/src/writer/xapianWorker.cpp),
// so a run of Chinese, Japanese or Korean yields unigrams and bigrams rather
// than word tokens. FoldText emits those terms in position order and keeps
// repeats, which lets strict article-text readings verify the same sequence.
//
// A document is a sequence and a query is a set, and the difference is not
// cosmetic: a strict reading asks where the query's words stand relative to one
// another, so a check that sees each word once cannot answer it. The article
// below says "Р С”Р В°Р Р…Р В°Р Т‘РЎРѓР С”Р С‘Р в„– Р Т‘Р В¶Р В°Р В·Р С•Р Р†РЎвЂ№Р в„– Р С”Р В»Р В°РЎР‚Р Р…Р ВµРЎвЂљР С‘РЎРѓРЎвЂљ" in its first sentence and names
// the profession "Р С”Р В»Р В°РЎР‚Р Р…Р ВµРЎвЂљР С‘РЎРѓРЎвЂљ" in its infobox a moment later - de-duplicated,
// the word survives only where it stands earlier, and the phrase that is
// plainly in the text verifies as absent (found on a live Wikipedia article,
// 2026-11-02, after the same class of bug had already emptied the reading once).
func FoldText(text string) []string {
	// libzim normalizes to NFD, removes marks and returns to NFC. dict.Fold
	// performs the same accent removal for the marks it supports but leaves
	// NFD output; NFC is essential for Hangul, whose decomposed Jamo are outside
	// Xapian's unbroken-script ranges.
	runes := []rune(norm.NFC.String(dict.Fold(text)))
	var out []string
	for i := 0; i < len(runes); {
		if isUnbrokenScript(runes[i]) && xapianWordChar(runes[i]) {
			start := i
			for i < len(runes) && isUnbrokenScript(runes[i]) && xapianWordChar(runes[i]) {
				i++
			}
			for j := start; j < i; j++ {
				out = append(out, string(runes[j]))
				if j+1 < i {
					out = append(out, string(runes[j:j+2]))
				}
			}
			continue
		}
		if notWord(runes[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < len(runes) && !notWord(runes[i]) && !(isUnbrokenScript(runes[i]) && xapianWordChar(runes[i])) {
			i++
		}
		out = append(out, string(runes[start:i]))
	}
	return out
}

// isUnbrokenScript mirrors Xapian's CJK n-gram ranges (queryparser/word-breaker.cc).
// Xapian combines these code points only when Unicode classifies them as
// word characters; punctuation in the ranges remains a separator.
func isUnbrokenScript(r rune) bool {
	return r >= 0x2E80 && r <= 0x2EFF ||
		r >= 0x3000 && r <= 0x9FFF ||
		r >= 0xA700 && r <= 0xA71F ||
		r >= 0xAC00 && r <= 0xD7AF ||
		r >= 0xF900 && r <= 0xFAFF ||
		r >= 0xFE30 && r <= 0xFE4F ||
		r >= 0xFF00 && r <= 0xFFEF ||
		r >= 0x20000 && r <= 0x2A6DF ||
		r >= 0x2F800 && r <= 0x2FA1F
}

// xapianWordChar mirrors Xapian::Unicode::is_wordchar for CJK n-gram runs.
func xapianWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || unicode.Is(unicode.Categories["Pc"], r)
}

// notWord preserves the existing word split for scripts with explicit word
// breaks. CJK runs use xapianWordChar instead.
func notWord(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// FoldTerms is FoldText as a SET, in first-appearance order: the terms a query
// asks for. A query is a set, so a word written twice asks for it once - the
// index is positionless and cannot tell the two apart anyway.
func FoldTerms(query string) []string {
	var out []string
	seen := map[string]bool{}
	for _, word := range FoldText(query) {
		if seen[word] {
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
	out, _ := intersectContext(context.Background(), a, b)
	return out
}

func intersectContext(ctx context.Context, a, b []Hit) ([]Hit, error) {
	out := a[:0]
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if (i+j)&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
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
	return out, ctx.Err()
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

// selectedWordHits preserves the union and summed frequencies of wordHits,
// while selecting only documents surviving the preceding intersections.
func (db *DB) selectedWordHits(ctx context.Context, terms []string, candidates []Hit) ([]Hit, error) {
	var hits []Hit
	for _, term := range terms {
		selected, err := db.selectedPostings(ctx, term, chunkTermKey(term), candidates)
		if err != nil {
			return nil, err
		}
		hits = append(hits, selected...)
	}
	if len(hits) > 1 {
		if err := sortHits(ctx, hits, func(i, j int) bool { return hits[i].DocID < hits[j].DocID }); err != nil {
			return nil, err
		}
		hits = coalesce(hits)
	}
	return hits, nil
}
