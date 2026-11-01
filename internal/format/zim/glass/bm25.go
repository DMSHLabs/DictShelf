// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import "math"

// Ranking: Xapian's own BM25, because the index was built for it.
//
// Reading a Kiwix index without Xapian's ranking puts long documents on top:
// a "Liste von Jazzmusikern" article that repeats a word forty times beats the
// article that is about it, and in the German Wikipedia it did - the three
// results for "Jazzmusiker" were the two lists and one article. The missing
// piece is the length normalisation, and the index carries everything it needs:
// each document's length (the postlist in rank.go) and the collection
// statistics (the version file, already parsed into Stats).
//
// The formula is Xapian's default BM25Weight, k1 = 1, k2 = 0, k3 = 1, b = 0.5,
// min_normlen = 0.5 (include/xapian/weight.h; the arithmetic is
// weight/bm25weight.cc). Two parts of that implementation are deliberately not
// reproduced here, and neither can change an order:
//
//   - the k3 factor, which scales a term by its frequency IN THE QUERY - and a
//     query here is a bag of distinct words, so every term's wqf is 1 and the
//     factor is 1 for all of them;
//   - the scheme's overall factor, a constant multiplying every term's weight
//     (it is what makes BM25Weight and BM25Weight1 differ) - a constant scale
//     cannot reorder anything.
//
// What is reproduced is everything that reorders, including the softening of
// the IDF for a term that indexes more than half the collection, which Xapian
// added deliberately rather than truncating such a weight to zero.
const (
	bm25K1         = 1.0
	bm25B          = 0.5
	bm25MinNormLen = 0.5
)

// bm25TermWeight is the per-term factor (BM25Weight::init).
func bm25TermWeight(docCount, termFreq uint32) float64 {
	if termFreq == 0 || docCount == 0 {
		return 0
	}
	tw := (float64(docCount) - float64(termFreq) + 0.5) / (float64(termFreq) + 0.5)
	if tw < 2 {
		tw = tw*0.5 + 1
	}
	return math.Log(tw) * (bm25K1 + 1)
}

// bm25LengthFactor turns a document length into the multiple of the average
// length, which is what the formula normalises by.
func bm25LengthFactor(docCount uint32, totalDocLen uint64) float64 {
	if docCount == 0 || totalDocLen == 0 {
		return 0
	}
	return float64(docCount) / float64(totalDocLen)
}

// bm25Part is one term's contribution to one document (BM25Weight::get_sumpart).
// The per-document "extra" term that Xapian adds is 2*k2*querylen/(1+normlen),
// which is zero at the default k2 = 0, so a score is exactly the sum of these
// over the query's terms.
//
// A document the length list does not know arrives with a length of zero, and
// is scored as the shortest possible document rather than as an average one:
// that is what Xapian's own min_normlen clamp does with a zero length, and
// guessing an average would hide a corrupt list behind plausible numbers.
func bm25Part(termWeight float64, wdf, docLen uint32, lenFactor float64) float64 {
	norm := float64(docLen) * lenFactor
	if norm < bm25MinNormLen {
		norm = bm25MinNormLen
	}
	denom := bm25K1*((1-bm25B)+bm25B*norm) + float64(wdf)
	if denom <= 0 {
		return 0
	}
	return termWeight * float64(wdf) / denom
}
