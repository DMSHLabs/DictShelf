// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/indexconst"
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
	// The strip's own session: "fts" is the walk, resultMode is the reading.
	stripParams := url.Values{"cacheAction": {"navStart"}, "cacheID": {"abcdef0123456789"},
		"dict": {dictionaryIDs["first.dsl"]}, "q": {"джазовый кларнетист"}, "mode": {"fts"},
		"resultMode": {"phrase"}, "groupID": {cacheID}, "cacheCount": {"2"}, "cacheOrders": {"0"},
		"ignoreDiacritics": {"1"}, "rungs": {"phrase"}, "pageSize": {"1"}, "firstSize": {"1"}}
	stripResult, err := server.DesktopJSONContext(context.Background(), "", "articleFindNavigation", stripParams.Encode())
	if err != nil {
		t.Fatal(err)
	}
	stripBody := stripResult.(map[string]any)["body"].(map[string]any)
	if stripBody["found"] != true {
		t.Fatalf("the strip's full-text session found nothing: %+v", stripBody)
	}
	if target, _ := stripBody["target"].(map[string]any); target == nil || target["reading"] != "phrase" {
		t.Fatalf("the strip's target = %+v; want the phrase reading", target)
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
	// The strip's shape: "fts" is the walk, resultMode is the reading that ran.
	fullTextSession := &articleFindNavigationSession{query: "джазовый кларнетист", mode: "fts",
		resultQuery: "джазовый кларнетист", resultMode: "phrase", rungs: []string{"phrase"},
		dictionaries: []string{dictionaryIDs["first.dsl"]},
		pageSize:     1, firstSize: 1, pages: []map[int]articleFindNavigationPage{make(map[int]articleFindNavigationPage)}}
	phrasePage, err := server.loadArticleFindNavigationPage(context.Background(), fullTextSession, 0, 0)
	if err != nil || len(phrasePage.Counts) != 1 || phrasePage.Counts[0] != 1 {
		t.Fatalf("full-text phrase page = %+v, err=%v; want one painted phrase", phrasePage, err)
	}
	// A full-text page counts the heads the answer was painted with; it ships no
	// ranges, because the client no longer paints them.
	if len(phrasePage.Occurrences[0]) != 0 {
		t.Fatalf("full-text phrase page carries occurrences: %+v", phrasePage.Occurrences[0])
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
	// The page addresses its matches the way the PAGE can resolve them: a path to
	// a text node and UTF-16 offsets inside that node. Never a block index - that
	// is the server's own model, and making the page reproduce it is what drifted.
	occurrence := page.Occurrences[0][0]
	if len(occurrence.Spans) != 1 || len(occurrence.Spans[0].Path) == 0 ||
		occurrence.Spans[0].End-occurrence.Spans[0].Start != 10 || occurrence.Text != "кларнетист" {
		t.Fatalf("literal match = %+v; want one node span of 10 UTF-16 units for кларнетист", occurrence)
	}
	literalTarget := articleSession.target(0, 0, 0, 0)
	if len(literalTarget.Occurrences) != 1 || literalTarget.Occurrences[0].Text != occurrence.Text {
		t.Fatalf("literal match target = %+v; want the page's own occurrence", literalTarget)
	}
}

func sameArticleFindTarget(actual, expected *articleFindNavigationTarget) bool {
	return actual != nil && expected != nil && actual.Dictionary == expected.Dictionary &&
		actual.DictionaryIndex == expected.DictionaryIndex && actual.BlockStart == expected.BlockStart &&
		actual.ArticleIndex == expected.ArticleIndex && actual.MatchIndex == expected.MatchIndex
}

// A full-text target names its occurrence by number, not by range: the answer
// is already painted, and the number indexes the heads the page counted. The
// strip's session says "fts" - the walk itself - and the reading travels in
// resultMode, so the dialog's literal trymodes of the same names are untouched.
func TestArticleFindFullTextTargetCarriesOccurrenceNumbers(t *testing.T) {
	session := &articleFindNavigationSession{
		mode: "fts", resultMode: "fts", rungs: []string{"phrase"}, dictionaries: []string{"dict"},
		pages: []map[int]articleFindNavigationPage{{0: {Counts: []int{2}}}},
	}
	target := session.target(0, 0, 0, 1)
	if target.ExpectedMatches != 2 || target.MatchIndex != 1 || target.Reading != "phrase" {
		t.Fatalf("full-text target = %+v; want the second occurrence of two", target)
	}
	if len(target.Occurrences) != 0 {
		t.Fatalf("full-text target ships ranges: %+v", target.Occurrences)
	}
	session.rungs[0] = "near"
	if nearTarget := session.target(0, 0, 0, 0); nearTarget.Reading != "near" {
		t.Fatalf("full-text near target reading = %q; want the reading answered by the dictionary", nearTarget.Reading)
	}
}

// The reader's position travels as a SNIPPET of the article's text, and the
// server finds it in its own model of the same article - so the gesture does not
// depend on the page's tree having the server's node indices (a frame's parser
// moves head content, and every top-level index shifts). The snippet also
// decides which occurrence is next: the first after it for Next, the last before
// it for Previous, and a snippet the search cannot find starts at the ends.
func TestArticleFindAnchorLocatesItsSnippet(t *testing.T) {
	dir := t.TempDir()
	source := "#NAME \"Anchor\"\n\nalpha\n\talpha beta alpha gamma alpha\n"
	if err := os.WriteFile(filepath.Join(dir, "anchor.dsl"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedDBDir(t)
	registry, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, registry)
	closeBackends(t, registry)
	entries := registry.all()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want one", len(entries))
	}
	if err := entries[0].setFeatures(features{FullText: true}, nil); err != nil {
		t.Fatal(err)
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
	id := entries[0].ID
	session := &articleFindNavigationSession{query: "alpha", mode: "contains",
		resultQuery: "alpha", resultMode: "exact", dictionaries: []string{id},
		pageSize: 1, firstSize: 1, pages: []map[int]articleFindNavigationPage{make(map[int]articleFindNavigationPage)}}
	page, err := server.loadArticleFindNavigationPage(context.Background(), session, 0, 0)
	if err != nil || len(page.Counts) != 1 || page.Counts[0] != 3 {
		t.Fatalf("page = %+v, err=%v; want three matches in the one article", page, err)
	}
	for _, tc := range []struct {
		name, anchor string
		direction    int
		position     int
	}{
		{"next lands after the anchor", "gamma alpha", 1, 3},
		{"previous lands before the anchor", "gamma alpha", -1, 2},
		{"an unfindable anchor starts Next at the top", "no such text", 1, 1},
		{"an unfindable anchor starts Previous at the end", "no such text", -1, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session.current = nil
			response, err := server.startArticleFindNavigationFromAnchor(context.Background(), session, id, 0, tc.anchor, tc.direction)
			if err != nil || !response.Found || response.Position != tc.position {
				t.Fatalf("anchor %q (%d) = %+v, err=%v; want position %d", tc.anchor, tc.direction, response, err, tc.position)
			}
		})
	}
}

// The literal list is bounded the way the marks are, and a bounded list makes the
// total a LOWER BOUND: an article that matches a short needle more times than
// indexconst.ArticleMarkLimit paints and counts the first that many, records that it did, and
// from then on reports its total as provisional - "Complete" false, which is the
// page's "+". A reader may see fewer matches than exist; being told an exact
// number that is not one would be worse.
func TestArticleFindCapBoundsALiteralList(t *testing.T) {
	dir := t.TempDir()
	source := "#NAME \"Cap\"\n\nbig\n\t" + strings.Repeat("a ", indexconst.ArticleMarkLimit+50) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "cap.dsl"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedDBDir(t)
	registry, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, registry)
	entries := registry.all()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want one", len(entries))
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
	session := &articleFindNavigationSession{query: "a", mode: "contains",
		resultQuery: "big", resultMode: "exact", dictionaries: []string{entries[0].ID},
		pageSize: 1, firstSize: 1, pages: []map[int]articleFindNavigationPage{make(map[int]articleFindNavigationPage)}}
	page, err := server.loadArticleFindNavigationPage(context.Background(), session, 0, 0)
	if err != nil || len(page.Counts) != 1 {
		t.Fatalf("page = %+v, err=%v; want one article", page, err)
	}
	if page.Counts[0] != indexconst.ArticleMarkLimit || len(page.Occurrences[0]) != indexconst.ArticleMarkLimit {
		t.Fatalf("count = %d, occurrences = %d; want the cap %d",
			page.Counts[0], len(page.Occurrences[0]), indexconst.ArticleMarkLimit)
	}
	if !session.capped {
		t.Fatal("the session did not record that an article was cut")
	}
	session.complete = true
	if session.completeAnswer() {
		t.Fatal("a capped session claimed its total was exact")
	}
}

// The page cache keys on the origin, and the walk's own page loads must use the
// spelling the reader's page requests carry - otherwise the same block is
// rendered twice, once for the walk and once for the client's prefetch.
func TestNavigationOriginIsThePagesOwnSpelling(t *testing.T) {
	cases := map[string]string{
		"https://dictshelf.localhost": "https://dictshelf.localhost",
		"http://127.0.0.1:8080":       "http://127.0.0.1:8080",
		"":                            "",
		"https://example.com/":        "",
		"https://example.com/path":    "",
		"https://user@example.com":    "",
		"/relative":                   "",
		"dictshelf.localhost":         "",
		"file:///tmp/x":               "",
	}
	for raw, want := range cases {
		if got := navigationOrigin(raw); got != want {
			t.Errorf("navigationOrigin(%q) = %q, want %q", raw, got, want)
		}
	}
}
