// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"math"
	"testing"
)

// The two numbers Xapian's BM25 turns on: its IDF, and the length
// normalisation. The expected values are the formula worked out by hand from
// weight/bm25weight.cc, not from this implementation.
func TestBM25TermWeight(t *testing.T) {
	// One term in ten of a thousand documents: tw = (1000-10+0.5)/(10+0.5),
	// termweight = ln(tw) * (k1 + 1).
	want := math.Log(990.5/10.5) * 2
	if got := bm25TermWeight(1000, 10); math.Abs(got-want) > 1e-9 {
		t.Errorf("bm25TermWeight(1000, 10) = %v, want %v", got, want)
	}
	// A term in every document: the raw ratio would be negative, so Xapian
	// softens it and the weight stays small but POSITIVE rather than becoming
	// zero - a term that cannot affect a ranking at all is worse than one that
	// barely does.
	got := bm25TermWeight(100, 100)
	if got <= 0 {
		t.Errorf("a term in every document got weight %v; Xapian softens rather than zeroes it", got)
	}
	if got > bm25TermWeight(100, 50) {
		t.Errorf("a universal term outranked a specific one: %v", got)
	}
	if bm25TermWeight(0, 0) != 0 || bm25TermWeight(100, 0) != 0 {
		t.Error("a term nothing holds must have no weight at all")
	}
}

// Length normalisation is the whole reason this ranking exists: with it, a
// short article that is about the query beats a long list that merely repeats
// it - which is what the German Wikipedia showed before it was implemented.
func TestBM25ShortDocumentScoresHigher(t *testing.T) {
	const docs, total = 1000, 100000 // average length 100
	factor := bm25LengthFactor(docs, total)
	if math.Abs(factor-0.01) > 1e-12 {
		t.Fatalf("length factor = %v, want 0.01", factor)
	}
	weight := bm25TermWeight(docs, 50)
	short := bm25Part(weight, 3, 100, factor)  // average length
	long := bm25Part(weight, 3, 1000, factor)  // ten times the average
	huge := bm25Part(weight, 3, 10000, factor) // a hundred times
	if !(short > long && long > huge) {
		t.Errorf("scores do not fall with length: %v, %v, %v", short, long, huge)
	}
	// More occurrences in the same document never score lower, and a long
	// document can overtake a short one once it holds enough of the term -
	// otherwise a long article could never rank for anything. How many is
	// enough is Xapian's own saturation, and it is steep: with k1 = 1, three
	// occurrences in an average document already score 0.75 of the term's
	// maximum, so a document ten times as long needs around seventeen to pass
	// it. That steepness is the length prior doing its job, not a bug - it is
	// what put the Jazzmusiker article above the list.
	if bm25Part(weight, 20, 1000, factor) <= long {
		t.Error("more occurrences in the same document scored lower")
	}
	if bm25Part(weight, 30, 1000, factor) <= short {
		t.Error("a long document holding the term often enough cannot outrank a short one")
	}
	// A missing length (zero) is clamped to the shortest document, not treated
	// as an average one: guessing would hide a corrupt list behind plausible
	// numbers.
	for _, len := range []uint32{0} {
		if got, want := bm25Part(weight, 1, len, factor), bm25Part(weight, 1, uint32(bm25MinNormLen/factor), factor); math.Abs(got-want) > 1e-12 {
			t.Errorf("a zero length scored %v, want the clamp at %v", got, want)
		}
	}
	if bm25LengthFactor(0, 0) != 0 {
		t.Error("an empty collection has no average length")
	}
}
