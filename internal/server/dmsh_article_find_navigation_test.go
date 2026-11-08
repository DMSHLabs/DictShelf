// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"net/url"
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
		if err != nil || !sameArticleFindTarget(target, &expected) {
			t.Fatalf("forward move %d = %+v, %v; want %+v", i+1, target, err, expected)
		}
		if wrapped {
			t.Fatalf("forward move %d wrapped early", i+1)
		}
		session.current = target
	}
	target, wrapped, err := session.move(context.Background(), 1, load)
	wantFirst := want[0]
	if err != nil || !sameArticleFindTarget(target, &wantFirst) || !wrapped {
		t.Fatalf("forward wrap = %+v, wrapped=%t, err=%v; want first block %+v", target, wrapped, err, wantFirst)
	}
	session.current = target
	target, wrapped, err = session.move(context.Background(), -1, load)
	wantLast := want[len(want)-1]
	if err != nil || !sameArticleFindTarget(target, &wantLast) || !wrapped {
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
	server.User = UserDir(t.TempDir())
	server.wildcardPages.init(server.wildcardCacheDir())
	t.Cleanup(func() {
		server.wildcardPages.mu.Lock()
		defer server.wildcardPages.mu.Unlock()
		for _, cached := range server.wildcardPages.entries {
			_ = cached.db.Close()
		}
	})
	const cacheID = "article-find-phrase-0001"
	for order, name := range []string{"first.dsl", "second.dsl"} {
		collectLookup(t, server, SearchRequest{Query: "джазовый кларнетист", Mode: "phrase",
			Dictionary: dictionaryIDs[name], CacheAction: "build", CacheID: cacheID,
			CacheCount: 2, CacheOrder: order, IgnoreDiacritics: true, Rung: "phrase"})
	}
	params := url.Values{"cacheAction": {"navStart"}, "cacheID": {"abcdef0123456789"},
		"dict": {dictionaryIDs["second.dsl"]}, "q": {"джазовый кларнетист"}, "mode": {"phrase"},
		"groupID": {cacheID}, "cacheCount": {"2"}, "cacheOrders": {"1"},
		"ignoreDiacritics": {"1"}, "rungs": {"phrase"}, "pageSize": {"1"}, "firstSize": {"1"}}
	fromCache, err := server.DesktopJSONContext(context.Background(), "", "articleFindNavigation", params.Encode())
	if err != nil {
		t.Fatal(err)
	}
	cachedBridge := fromCache.(map[string]any)
	if cachedBridge["status"] != 200 || cachedBridge["body"].(map[string]any)["found"] != true {
		t.Fatalf("article find cache bridge = %+v; want phrase match in second dictionary", cachedBridge)
	}
	bridgeResult, err := server.DesktopJSONContext(context.Background(), "", "articleFindNavigation",
		"cacheAction=navStart&cacheID=abcdef0123456789&dict="+dictionaryIDs["first.dsl"]+"&q=%D0%B4%D0%B6%D0%B0%D0%B7%D0%BE%D0%B2%D1%8B%D0%B9+%D0%BA%D0%BB%D0%B0%D1%80%D0%BD%D0%B5%D1%82%D0%B8%D1%81%D1%82&mode=phrase&pageSize=1&firstSize=1")
	if err != nil {
		t.Fatal(err)
	}
	bridge := bridgeResult.(map[string]any)
	if bridge["status"] != 200 || bridge["body"].(map[string]any)["found"] != true {
		t.Fatalf("desktop navigation bridge = %+v; want first phrase match", bridge)
	}
	session := &articleFindNavigationSession{query: "джазовый кларнетист", mode: "phrase", resultQuery: "джазовый кларнетист", resultMode: "phrase",
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
		if err != nil || !sameArticleFindTarget(target, &want) {
			t.Fatalf("server search move %d = %+v, %v; want %+v", i+1, target, err, want)
		}
		session.current = target
	}
	fullTextSession := &articleFindNavigationSession{query: "джазовый кларнетист", mode: "fts",
		resultQuery: "джазовый кларнетист", resultMode: "fts", rungs: []string{"phrase"},
		dictionaries: []string{dictionaryIDs["first.dsl"]},
		pageSize:     1, firstSize: 1, pages: []map[int]articleFindNavigationPage{make(map[int]articleFindNavigationPage)}}
	phrasePage, err := server.loadArticleFindNavigationPage(context.Background(), fullTextSession, 0, 0)
	if err != nil || len(phrasePage.Counts) != 1 || phrasePage.Counts[0] != 1 || len(phrasePage.Occurrences[0]) != 1 {
		t.Fatalf("full-text phrase page = %+v, err=%v; want one phrase occurrence", phrasePage, err)
	}
	target, wrapped, err := session.move(context.Background(), 1, load)
	if err != nil || target == nil || target.DictionaryIndex != 0 || target.BlockStart != 0 || target.ArticleIndex != 0 || !wrapped {
		t.Fatalf("server search wrap = %+v, wrapped=%t, err=%v; want first dictionary block", target, wrapped, err)
	}
	gotoResult, err := server.gotoArticleFindNavigation(context.Background(), session, 4)
	if err != nil || !gotoResult.Found || gotoResult.Target.DictionaryIndex != 1 || gotoResult.Position != 4 {
		t.Fatalf("server goto = %+v, err=%v; want fourth occurrence in second dictionary", gotoResult, err)
	}
	articleSession := &articleFindNavigationSession{query: "кларнетист", mode: "contains", resultQuery: "one", resultMode: "exact",
		dictionaries: []string{dictionaryIDs["first.dsl"]}, pageSize: 1, firstSize: 1,
		pages: []map[int]articleFindNavigationPage{make(map[int]articleFindNavigationPage)}}
	page, err := server.loadArticleFindNavigationPage(context.Background(), articleSession, 0, 0)
	if err != nil || len(page.Counts) != 1 || page.Counts[0] != 1 || len(page.Occurrences[0]) != 1 {
		t.Fatalf("server-side literal article matches = %+v, err=%v; want one match", page, err)
	}
	literalTarget := articleSession.target(0, 0, 0, 0)
	if literalTarget.Group != 0 || literalTarget.Start != page.Occurrences[0][0].Start || literalTarget.End-literalTarget.Start != 10 {
		t.Fatalf("literal match target = %+v; want the server's UTF-16 range for кларнетист", literalTarget)
	}
}

func sameArticleFindTarget(actual, expected *articleFindNavigationTarget) bool {
	return actual != nil && expected != nil && actual.Dictionary == expected.Dictionary &&
		actual.DictionaryIndex == expected.DictionaryIndex && actual.BlockStart == expected.BlockStart &&
		actual.ArticleIndex == expected.ArticleIndex && actual.MatchIndex == expected.MatchIndex
}

func TestArticleFindFullTextTargetKeepsPhraseOccurrences(t *testing.T) {
	occurrences := []articleFindOccurrence{{Group: 0, Start: 0, End: 21}, {Group: 0, Start: 24, End: 45}}
	session := &articleFindNavigationSession{
		mode: "fts", dictionaries: []string{"dict"},
		pages: []map[int]articleFindNavigationPage{{0: {
			Counts: []int{len(occurrences)}, Occurrences: [][]articleFindOccurrence{occurrences},
		}}},
	}
	target := session.target(0, 0, 0, 1)
	if target.ExpectedMatches != 2 || target.MatchIndex != 1 || len(target.Occurrences) != 2 ||
		target.Occurrences[1] != occurrences[1] {
		t.Fatalf("full-text target = %+v; want the second complete phrase occurrence", target)
	}
}
