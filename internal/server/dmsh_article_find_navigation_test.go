// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestArticleFindNavigationWalksDictionaryBlocksAndWraps(t *testing.T) {
	contents := [][]int{{1, 1, 1}, {1}, {2}}
	session := &articleFindNavigationSession{
		dictionaries: []string{"first", "second", "favourites"},
		pageSize:     1,
		firstSize:    1,
		pages:        []map[int]articleFindNavigationPage{{}, {}, {}},
	}
	for i := range session.pages {
		session.pages[i] = make(map[int]articleFindNavigationPage)
	}
	load := func(_ context.Context, dictionary, start int) (articleFindNavigationPage, error) {
		blockSize := session.blockLength(start)
		end := min(start+blockSize, len(contents[dictionary]))
		counts := append([]int(nil), contents[dictionary][min(start, len(contents[dictionary])):end]...)
		return articleFindNavigationPage{Start: start, Counts: counts, HasNext: end < len(contents[dictionary])}, nil
	}
	want := []articleFindNavigationTarget{
		{Dictionary: "first", DictionaryIndex: 0, BlockStart: 0, ArticleIndex: 0, MatchIndex: 0},
		{Dictionary: "first", DictionaryIndex: 0, BlockStart: 1, ArticleIndex: 1, MatchIndex: 0},
		{Dictionary: "first", DictionaryIndex: 0, BlockStart: 2, ArticleIndex: 2, MatchIndex: 0},
		{Dictionary: "second", DictionaryIndex: 1, BlockStart: 0, ArticleIndex: 0, MatchIndex: 0},
		{Dictionary: "favourites", DictionaryIndex: 2, BlockStart: 0, ArticleIndex: 0, MatchIndex: 0},
		{Dictionary: "favourites", DictionaryIndex: 2, BlockStart: 0, ArticleIndex: 0, MatchIndex: 1},
	}
	for i, expected := range want {
		target, wrapped, err := session.move(context.Background(), 1, load)
		if err != nil || target == nil || *target != expected {
			t.Fatalf("forward move %d = %+v, %v; want %+v", i+1, target, err, expected)
		}
		if wrapped {
			t.Fatalf("forward move %d wrapped early", i+1)
		}
		session.current = target
	}
	target, wrapped, err := session.move(context.Background(), 1, load)
	wantFirst := want[0]
	if err != nil || target == nil || *target != wantFirst || !wrapped {
		t.Fatalf("forward wrap = %+v, wrapped=%t, err=%v; want first block %+v", target, wrapped, err, wantFirst)
	}
	session.current = target
	target, wrapped, err = session.move(context.Background(), -1, load)
	wantLast := want[len(want)-1]
	if err != nil || target == nil || *target != wantLast || !wrapped {
		t.Fatalf("backward wrap = %+v, wrapped=%t, err=%v; want last match %+v", target, wrapped, err, wantLast)
	}
}

func TestArticleFindNavigationUsesServerSearchAndHighlights(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"first.dsl":  "#NAME \"First\"\n\none\n\tджазовый кларнетист\n\ntwo\n\tджазовый кларнетист\n\nthree\n\tджазовый кларнетист\n",
		"second.dsl": "#NAME \"Second\"\n\nfour\n\tджазовый кларнетист\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	isolatedDBDir(t)
	registry, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, registry)
	closeBackends(t, registry)
	entries := registry.all()
	dictionaryIDs := make(map[string]string, len(entries))
	for _, entry := range entries {
		if err := entry.setFeatures(features{FullText: true}, nil); err != nil {
			t.Fatal(err)
		}
		dictionaryIDs[filepath.Base(entry.Path)] = entry.ID
	}
	server := New(registry)
	session := &articleFindNavigationSession{query: "джазовый кларнетист", mode: "phrase",
		dictionaries: []string{dictionaryIDs["first.dsl"], dictionaryIDs["second.dsl"]},
		pageSize:     1, firstSize: 1, pages: []map[int]articleFindNavigationPage{{}, {}}}
	for i := range session.pages {
		session.pages[i] = make(map[int]articleFindNavigationPage)
	}
	load := func(ctx context.Context, dictionary, start int) (articleFindNavigationPage, error) {
		return server.loadArticleFindNavigationPage(ctx, session, dictionary, start)
	}
	for i, want := range []articleFindNavigationTarget{
		{Dictionary: dictionaryIDs["first.dsl"], DictionaryIndex: 0, BlockStart: 0, ArticleIndex: 0, MatchIndex: 0},
		{Dictionary: dictionaryIDs["first.dsl"], DictionaryIndex: 0, BlockStart: 1, ArticleIndex: 1, MatchIndex: 0},
		{Dictionary: dictionaryIDs["first.dsl"], DictionaryIndex: 0, BlockStart: 2, ArticleIndex: 2, MatchIndex: 0},
		{Dictionary: dictionaryIDs["second.dsl"], DictionaryIndex: 1, BlockStart: 0, ArticleIndex: 0, MatchIndex: 0},
	} {
		target, _, err := session.move(context.Background(), 1, load)
		if err != nil || target == nil || *target != want {
			t.Fatalf("server search move %d = %+v, %v; want %+v", i+1, target, err, want)
		}
		session.current = target
	}
	target, wrapped, err := session.move(context.Background(), 1, load)
	if err != nil || target == nil || target.DictionaryIndex != 0 || target.BlockStart != 0 || target.ArticleIndex != 0 || !wrapped {
		t.Fatalf("server search wrap = %+v, wrapped=%t, err=%v; want first dictionary block", target, wrapped, err)
	}
	gotoResult, err := server.gotoArticleFindNavigation(context.Background(), session, 4)
	if err != nil || !gotoResult.Found || gotoResult.Target.DictionaryIndex != 1 || gotoResult.Position != 4 {
		t.Fatalf("server goto = %+v, err=%v; want fourth occurrence in second dictionary", gotoResult, err)
	}
}
