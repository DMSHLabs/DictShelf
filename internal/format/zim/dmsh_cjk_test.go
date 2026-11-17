// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
)

// These real-archive checks pin queries to the n-gram terms Xapian indexes for
// the CJK corpora available on the maintainer machine.
func TestRealCJKNgramSearch(t *testing.T) {
	dir := os.Getenv("WUDICT_TEST_ZIM_DIR")
	if dir == "" {
		t.Skip("WUDICT_TEST_ZIM_DIR not set")
	}
	tests := []struct {
		archive string
		query   string
		terms   []string
	}{
		{
			archive: "wikipedia_yue_all_maxi_2026-09.zim",
			query:   "中華人民共和國",
			terms:   []string{"中", "中華", "華", "華人", "人", "人民", "民", "民共", "共", "共和", "和", "和國", "國"},
		},
		{
			archive: "wikipedia_zh_all_nopic_2026-07.zim",
			query:   "中华人民共和国",
			terms:   []string{"中", "中华", "华", "华人", "人", "人民", "民", "民共", "共", "共和", "和", "和国", "国"},
		},
		{
			archive: "wiktionary_ja_all_nopic_2026-10.zim",
			query:   "日本語",
			terms:   []string{"日", "日本", "本", "本語", "語"},
		},
	}
	checked := 0
	for _, tc := range tests {
		path := filepath.Join(dir, tc.archive)
		if _, err := os.Stat(path); err != nil {
			t.Logf("skip %s: %v", tc.archive, err)
			continue
		}
		checked++
		t.Run(tc.archive, func(t *testing.T) {
			d, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			window, size, err := d.XapianIndex()
			if err != nil {
				t.Fatal(err)
			}
			db, err := glass.OpenSource(window, size)
			if err != nil {
				t.Fatal(err)
			}

			res, err := db.Search(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Complete || len(res.Missing) != 0 || len(res.Hits) == 0 {
				t.Fatalf("Search(%q): complete=%v missing=%v hits=%d", tc.query, res.Complete, res.Missing, len(res.Hits))
			}
			got := make(map[string]bool, len(res.Terms))
			for _, term := range res.Terms {
				got[term] = true
			}
			for _, term := range tc.terms {
				if !got[term] {
					t.Errorf("query terms %q do not include Xapian n-gram %q; got %v", tc.query, term, res.Terms)
				}
				freq, err := db.TermFreq(term)
				if err != nil {
					t.Fatal(err)
				}
				if freq == 0 {
					t.Errorf("Xapian term %q is absent from %s", term, tc.archive)
				}
			}

			articles, err := d.FullText(tc.query, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(articles) == 0 {
				t.Fatalf("FullText(%q) returned no article", tc.query)
			}
			t.Logf("query terms=%d, matching docs=%d, returned article=%q", len(res.Terms), len(res.Hits), articles[0].Headword)

			phrases, err := d.FullTextPhrase(tc.query, 1, 1000)
			if err != nil {
				t.Fatal(err)
			}
			if len(phrases) == 0 {
				t.Fatalf("FullTextPhrase(%q) did not verify the CJK text", tc.query)
			}
			t.Logf("phrase result=%q", phrases[0].Headword)
		})
	}
	if checked == 0 {
		t.Skip("no configured CJK ZIM archives found")
	}
}
