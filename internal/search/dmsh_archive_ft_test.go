// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package search

import (
	"context"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// archiveFake is a prepared-style backend whose full text comes from its source
// (the shape a Kiwix archive has through internal/format/zim/glass). It counts
// which reading the search asked it for, because that is the whole contract:
// "words" comes from the index, and the two strict readings are verified in the
// article text by the source itself.
type archiveFake struct {
	fake
	wordsCalls  int
	phraseCalls int
	nearCalls   int
	matchCalls  int
}

func (a *archiveFake) Caps() dict.Caps {
	c := a.fake.Caps()
	c.FTS = true
	return c
}

func (a *archiveFake) FullTextFromArchive() bool { return true }

func (a *archiveFake) FullTextContext(context.Context, string, int) ([]dict.Result, error) {
	a.wordsCalls++
	return []dict.Result{{Headword: "beta", Body: "<p>beta</p>"}}, nil
}

func (a *archiveFake) FullTextMatch(context.Context, string, int) ([]dict.Result, error) {
	a.matchCalls++
	return nil, nil
}

func (a *archiveFake) ArchivePhrase(context.Context, string, int, int) ([]dict.Result, error) {
	a.phraseCalls++
	return []dict.Result{{Headword: "phrase", Body: "<p>phrase</p>"}}, nil
}

func (a *archiveFake) ArchiveNear(context.Context, int, string, int, int) ([]dict.Result, error) {
	a.nearCalls++
	return []dict.Result{{Headword: "near", Body: "<p>near</p>"}}, nil
}

// Every full-text mode asks an archive-backed dictionary for its own reading and
// gets it: the phrase and the distance are verified by the source, and the rung
// names which reading answered so the words can be marked. No mode walks a
// ladder - the fake's expression entry point is never called for an archive,
// which has no expression language to give it.
func TestArchiveReadingsPerMode(t *testing.T) {
	for _, tc := range []struct {
		mode      Mode
		wantRung  string
		words     int
		phrase    int
		near      int
		wantFirst string
	}{
		{FullText, "words", 1, 0, 0, "beta"},
		{Phrase, "phrase", 0, 1, 0, "phrase"},
		{Near, "near", 0, 0, 1, "near"},
	} {
		af := &archiveFake{fake: fake{name: "archive.zim", words: []string{"beta"}}}
		hits := all(context.Background(), []dict.Dictionary{af}, tc.mode, "beta gamma", 10)
		if len(hits) != 1 {
			t.Fatalf("%v: hits = %+v", tc.mode, hits)
		}
		h := hits[0]
		if h.Rung != tc.wantRung {
			t.Errorf("%v: Rung = %q, want %q", tc.mode, h.Rung, tc.wantRung)
		}
		if len(h.Results) != 1 || h.Results[0].Headword != tc.wantFirst {
			t.Errorf("%v: results = %+v", tc.mode, h.Results)
		}
		if af.wordsCalls != tc.words || af.phraseCalls != tc.phrase || af.nearCalls != tc.near {
			t.Errorf("%v: readings asked = words %d, phrase %d, near %d; want %d/%d/%d",
				tc.mode, af.wordsCalls, af.phraseCalls, af.nearCalls, tc.words, tc.phrase, tc.near)
		}
		if af.matchCalls != 0 {
			t.Errorf("%v: an FTS5 expression was handed to an archive-backed dictionary %d times",
				tc.mode, af.matchCalls)
		}
	}
}

// A dictionary with an index of ours answers each strict mode with its own
// expression: the phrase is a quoted phrase, the distance is a NEAR, and the
// words mode is the bag of words. That is what makes the mode mean what its
// label says on the dictionaries that can promise it.
func TestStrictModesChooseTheirExpression(t *testing.T) {
	seen := map[Mode]string{}
	for _, mode := range []Mode{Phrase, Near, FullText} {
		f := &recordingFake{fake: fake{name: "db", words: []string{"beta"}}}
		hits := all(context.Background(), []dict.Dictionary{f}, mode, "beta gamma", 5)
		if len(hits) != 1 {
			t.Fatalf("%v: hits = %+v", mode, hits)
		}
		seen[mode] = f.lastMatch
	}
	// The phrase mode lowers the words into ONE quoted phrase - adjacent, in
	// that order - the near mode says NEAR, and the words mode keeps each word
	// in its own quotes as a prefix, which is how a bag of words matches the
	// inflections ftsq cannot know. The modes disagree by construction, which is
	// the point: the mode is the promise.
	if !strings.Contains(seen[Phrase], `"beta gamma"`) || strings.Contains(seen[Phrase], "NEAR") {
		t.Errorf("the phrase reading is not one adjacent phrase: %q", seen[Phrase])
	}
	if !strings.Contains(seen[Near], "NEAR") {
		t.Errorf("the near reading does not use NEAR: %q", seen[Near])
	}
	if !strings.Contains(seen[FullText], `"beta"*`) || strings.Contains(seen[FullText], "NEAR") ||
		strings.Contains(seen[FullText], `"beta gamma"`) {
		t.Errorf("the words reading is not each word on its own: %q", seen[FullText])
	}
	if seen[Phrase] == seen[Near] || seen[Near] == seen[FullText] {
		t.Errorf("two modes asked for the same reading: %q / %q / %q", seen[Phrase], seen[Near], seen[FullText])
	}
}

// recordingFake is a prepared-style backend that remembers the expression the
// search composed for the mode.
type recordingFake struct {
	fake
	lastMatch string
}

func (f *recordingFake) Caps() dict.Caps {
	c := f.fake.Caps()
	c.FTS = true
	return c
}

func (f *recordingFake) FullTextMatch(_ context.Context, match string, limit int) ([]dict.Result, error) {
	f.lastMatch = match
	return []dict.Result{{Headword: "beta", Body: "<p>beta</p>"}}, nil
}
