// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package search

import (
	"context"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// archiveFake is a prepared-style backend whose full text comes from its source
// (the shape a Kiwix archive has through internal/format/zim/glass). It counts
// which of the two full-text entry points the search used, because that is the
// whole difference: the rungs are FTS5 expressions and the archive has no
// expression language.
type archiveFake struct {
	fake
	fullTextCalls int
	matchCalls    int
}

func (a *archiveFake) Caps() dict.Caps {
	c := a.fake.Caps()
	c.FTS = true
	return c
}

func (a *archiveFake) FullTextFromArchive() bool { return true }

func (a *archiveFake) FullTextContext(context.Context, string, int) ([]dict.Result, error) {
	a.fullTextCalls++
	return []dict.Result{{Headword: "beta", Body: "<p>beta</p>"}}, nil
}

func (a *archiveFake) FullTextMatch(context.Context, string, int) ([]dict.Result, error) {
	a.matchCalls++
	return nil, nil
}

func TestArchiveFullTextSkipsTheRungs(t *testing.T) {
	af := &archiveFake{fake: fake{name: "archive.zim", words: []string{"beta"}}}
	hits := all(context.Background(), []dict.Dictionary{af}, FullText, "beta gamma", 10)
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	if af.matchCalls != 0 {
		t.Errorf("the FTS5 rungs were asked of an archive-backed handle %d times", af.matchCalls)
	}
	if af.fullTextCalls != 1 {
		t.Errorf("the archive was asked %d times, want once", af.fullTextCalls)
	}
	if len(hits[0].Results) != 1 || hits[0].Err != nil {
		t.Fatalf("hit = %+v", hits[0])
	}
	// Without a rung the hit carries none of the words the query was read as,
	// and the client highlights nothing at all: "words" is what a stemmed index
	// does, and it is what makes the existing marking work.
	if hits[0].Rung != "words" {
		t.Errorf("Rung = %q, want %q", hits[0].Rung, "words")
	}
}
