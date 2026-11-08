// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// wireMark is one mark as the wire carries it: which words, and with which
// class. The occurrence number is a separate property and is pinned by the
// painter's own tests; here what matters is that the right words are painted
// apart or together.
type wireMark struct{ class, text string }

// wireMarkRE reads a painted fragment as the wire carries it: a match is a
// <mark>, the words between a near window's terms are a <span>, and both carry
// the occurrence number.
var wireMarkRE = regexp.MustCompile(`<(?:mark|span) class="([^"]*)" data-wu-match="(\d+)">([^<]*)</(?:mark|span)>`)

func hasMark(body string, want wireMark) bool {
	for _, m := range wireMarkRE.FindAllStringSubmatch(body, -1) {
		if m[1] == want.class && m[3] == want.text {
			return true
		}
	}
	return false
}

// The marking rule lives in three places that can drift independently - the
// `hl` parameter, the full-text-only gate, and the rewriter hook that actually
// emits the tag - so it is asserted end to end, on the bytes a client receives.
//
// The fixture is its own dictionary, not the shared one: proving that a
// non-full-text mode does NOT mark requires a body that contains the query
// word, which "casa"/"vivienda" cannot give.
const hlDSL = "#NAME \"HL Test Dict\"\n\n" +
	"vivienda\n\tuna vivienda es una casa\n\n" +
	"casa\n\tvivienda\n"

func newHLServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hl.dsl"), []byte(hlDSL), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedDBDir(t)
	reg, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, reg)
	s := New(reg)
	dicts := getDicts(t, s, "/api/dicts")
	if len(dicts) != 1 {
		t.Fatalf("dicts: %+v", dicts)
	}
	id := dicts[0].ID
	sse(t, s, "/api/ingest?dict="+id+"&fts=1")
	return s, id
}

// The reader pages through the ARTICLE RESULT CACHE, not the live answer
// (`fetchSearchPortion` in index.html), so a strict reading has to keep its
// marks on the cache's pages too. The strip's Next/Previous walks those marks
// (index.html, markList/navGo), and a page drawn without them cannot be walked
// - which is the whole of "Next and Previous work in all three readings".
// The live answer is the other half of the same promise and is covered by
// TestSearchPhraseMarking; this is the cache.
func TestArticleResultCacheMarksStrictReadings(t *testing.T) {
	s, id := newHLServer(t)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		defer s.wildcardPages.mu.Unlock()
		for _, entry := range s.wildcardPages.entries {
			_ = entry.db.Close()
		}
	})

	for _, tc := range []struct {
		name, q, mode, rung string
		marks               []wireMark
		exact               string
	}{
		{"the phrase mode keeps its one span on a paged result", "vivienda es una", "phrase", "phrase",
			nil, `<mark class="wu-hl" data-wu-match="0">vivienda es una</mark>`},
		{"the near mode keeps the words apart on a paged result", "casa una", "near", "near",
			[]wireMark{{"wu-hl", "una"}, {"wu-hl-ex", "casa"}, {"wu-hl-ex2", " vivienda es una "}}, ""},
		{"the words mode marks a paged result too", "vivienda es una", "fts", "words",
			[]wireMark{{"wu-hl", "vivienda"}, {"wu-hl", "es"}, {"wu-hl", "una"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheID := "strict-reading-" + tc.mode + "-0001"
			base := "/api/search?q=" + url.QueryEscape(tc.q) + "&mode=" + tc.mode + "&rung=" + tc.rung +
				"&hl=1&dict=" + id + "&cacheID=" + cacheID
			build := stream(t, s, base+"&cacheAction=build")
			built, total := false, 0
			for _, m := range build {
				if m.T == "cache" && m.Complete {
					built, total = true, m.Total
				}
			}
			if !built {
				t.Fatalf("the cache never completed: %+v", build)
			}
			if total == 0 {
				t.Fatalf("the cache built no ids for a reading that matched: %+v", build)
			}
			page := stream(t, s, base+"&cacheAction=page&start=0&size=100")
			got := bodies(page)
			if !strings.Contains(got, "vivienda") {
				t.Fatalf("the cached page carried no article text - the case would pass vacuously: %+v", page)
			}
			if tc.exact != "" && !strings.Contains(got, tc.exact) {
				t.Errorf("the cached page lost %s: %s", tc.exact, got)
			}
			for _, m := range tc.marks {
				if !hasMark(got, m) {
					t.Errorf("the cached page lost the %s mark on %q: %s", m.class, m.text, got)
				}
			}
		})
	}
}

// A caller that leaves the reading out is asking by MODE, and the page must
// still come back marked: the article walk counts marks, so an unmarked page
// reads as "no matches" and the bottom strip hides. That is not hypothetical -
// the page's rung lookup silently matched nothing for a while
// (`[data-searchDict]` does not select `data-search-dict`), so every navigation
// page was served unmarked while the strip's counts still came from the text.
func TestArticleResultCacheMarksWithoutARung(t *testing.T) {
	s, id := newHLServer(t)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		defer s.wildcardPages.mu.Unlock()
		for _, entry := range s.wildcardPages.entries {
			_ = entry.db.Close()
		}
	})
	const cacheID = "no-rung-reading-0001"
	base := "/api/search?q=" + url.QueryEscape("vivienda es una") + "&mode=phrase&hl=1&dict=" + id + "&cacheID=" + cacheID
	built := false
	for _, m := range stream(t, s, base+"&cacheAction=build") {
		if m.T == "cache" && m.Complete {
			built = true
		}
	}
	if !built {
		t.Fatal("the cache never completed without a rung")
	}
	got := bodies(stream(t, s, base+"&cacheAction=page&start=0&size=100"))
	if !strings.Contains(got, `<mark class="wu-hl" data-wu-match="0">vivienda es una</mark>`) {
		t.Fatalf("a page built without a rung came back unmarked: %s", got)
	}
	if n, capped := countArticleFindHeads(got); n != 1 || capped {
		t.Fatalf("heads on the unmarked page = %d (capped %t), want 1", n, capped)
	}
}

func bodies(hits []streamMsg) string {
	var b strings.Builder
	for _, h := range hits {
		for _, r := range h.Results {
			b.WriteString(r.Body)
		}
	}
	return b.String()
}

func TestSearchHighlight(t *testing.T) {
	s, id := newHLServer(t)

	got := bodies(searchStream(t, s, "/api/search?q=vivi&mode=fts&hl=1&dict="+id))
	if !hasMark(got, wireMark{"wu-hl", "vivienda"}) {
		t.Errorf("fts&hl=1 did not mark the whole token: %s", got)
	}

	for _, q := range []string{
		"/api/search?q=vivi&mode=fts&dict=" + id,             // opt-in: absent means off
		"/api/search?q=vivi&mode=fts&hl=0&dict=" + id,        // explicitly off
		"/api/search?q=vivienda&mode=prefix&hl=1&dict=" + id, // full-text only
	} {
		got := bodies(searchStream(t, s, q))
		if !strings.Contains(got, "vivienda") {
			t.Fatalf("%s returned no body carrying the query word - the case would pass vacuously: %q", q, got)
		}
		if strings.Contains(got, "wu-hl") {
			t.Errorf("%s marked anyway: %s", q, got)
		}
	}
}

// The phrase is the unit. A multi-word full-text query marks ONE span across
// the words it matched, and only a descent to a weaker rung marks them apart -
// which is also the only case the response admits to, via `rung`.
func TestSearchPhraseMarking(t *testing.T) {
	s, id := newHLServer(t)

	/* The MODE is the reading, so each case names the one it is asking for. A
	   mode that relaxed into a weaker reading would answer a question nobody
	   asked, and the test that used to live here pinned exactly that descent:
	   "casa una" falling from a phrase to NEAR on its own. That descent is gone
	   by decision - the reader chooses the reading now - so the marking cases
	   moved to the modes that promise them. */
	for _, tc := range []struct {
		name, q, mode, rung string
		marks               []wireMark
		exact               string
	}{
		{"the phrase mode marks the words as one span", "vivienda es una", "phrase", "phrase",
			nil, `<mark class="wu-hl" data-wu-match="0">vivienda es una</mark>`},
		{"the near mode marks the words apart", "casa una", "near", "near",
			[]wireMark{{"wu-hl", "una"}, {"wu-hl-ex", "casa"}}, ""},
		{"the words mode does not join them either", "vivienda es una", "fts", "words",
			[]wireMark{{"wu-hl", "vivienda"}, {"wu-hl", "es"}}, ""},
		{"one word is one reading", "vivi", "fts", "words",
			nil, `<mark class="wu-hl" data-wu-match="0">vivienda</mark>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits := searchStream(t, s, "/api/search?q="+url.QueryEscape(tc.q)+"&mode="+tc.mode+"&hl=1&dict="+id)
			var rung string
			for _, h := range hits {
				if len(h.Results) > 0 {
					rung = h.Rung
				}
			}
			if rung != tc.rung {
				t.Errorf("q=%q in mode %q answered on rung %q, want %q", tc.q, tc.mode, rung, tc.rung)
			}
			got := bodies(hits)
			if tc.exact != "" && !strings.Contains(got, tc.exact) {
				t.Errorf("q=%q: missing %s in %s", tc.q, tc.exact, got)
			}
			for _, m := range tc.marks {
				if !hasMark(got, m) {
					t.Errorf("q=%q: missing the %s mark on %q in %s", tc.q, m.class, m.text, got)
				}
			}
		})
	}
}

// A page for a dictionary whose list is EMPTY is an empty answer, not a wait.
// The cache row is written when a build finishes even with no ids (a strict
// reading that matched nothing), and a page request must read that row instead
// of polling for a block that will never exist: the favourites slot, empty for
// most readers, made every Find in articles session start with a 30-second wait
// and "article result cache is unavailable".
func TestArticleResultCacheAnswersAnEmptyList(t *testing.T) {
	s, id := newHLServer(t)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		defer s.wildcardPages.mu.Unlock()
		for _, entry := range s.wildcardPages.entries {
			_ = entry.db.Close()
		}
	})
	const cacheID = "e000000000000001"
	base := "/api/search?q=" + url.QueryEscape("zebra quagga") + "&mode=phrase&rung=phrase&hl=1&dict=" + id + "&cacheID=" + cacheID
	built, total := false, -1
	build := stream(t, s, base+"&cacheAction=build")
	for _, m := range build {
		if m.T == "cache" && m.Complete {
			built, total = true, m.Total
		}
	}
	if !built || total != 0 {
		t.Fatalf("the empty build = built %t, total %d; want a completed empty list: %+v", built, total, build)
	}
	started := time.Now()
	page := stream(t, s, base+"&cacheAction=page&start=0&size=5")
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("the empty page waited %v; it must answer at once", elapsed)
	}
	if got := bodies(page); got != "" {
		t.Fatalf("the empty page returned bodies: %q", got)
	}
}
