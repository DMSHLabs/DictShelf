// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

func TestStreamArticleIDsKeepsBoundedBatchesAndArticleOrder(t *testing.T) {
	entries := make([]dict.Entry, 625)
	for i := range entries {
		entries[i] = h(fmt.Sprintf("token%04d", i), fmt.Sprintf("<p>body %d</p>", i))
	}
	path := filepath.Join(t.TempDir(), "text.db")
	reader := &fakeReader{meta: dict.Meta{Name: "many", Format: "test", Path: "/x"}, entries: entries}
	if _, err := IngestPlan(reader, path, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var ids []int64
	var largest int
	err = s.StreamArticleIDs(context.Background(), "token", "prefix", "", func(batch []int64, total int, done bool) error {
		if len(batch) > largest {
			largest = len(batch)
		}
		ids = append(ids, batch...)
		if done && total != len(ids) {
			t.Fatalf("final count %d, received %d IDs", total, len(ids))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 625 || largest > 300 {
		t.Fatalf("IDs=%d, largest batch=%d", len(ids), largest)
	}
	portion, err := s.ArticlesByIDs(context.Background(), ids[295:395])
	if err != nil {
		t.Fatal(err)
	}
	if len(portion) != 100 || portion[0].Headword != "token0295" || portion[99].Headword != "token0394" {
		t.Fatalf("portion edges: %+v / %+v", portion[0], portion[len(portion)-1])
	}
}

func TestStreamArticleIDsUsesSearchModesWithoutBodies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "text.db")
	reader := &fakeReader{meta: dict.Meta{Name: "modes", Format: "test", Path: "/x"}, entries: []dict.Entry{
		h("Blue heart", "<p>cardiac tissue</p>"), h("heartbroken", "<p>sad</p>"),
		h("work", "<p>labor</p>"), h("work", "<p>second article</p>"), h("café", "<p>coffee</p>"),
	}}
	if _, err := IngestPlan(reader, path, Plan{Contains: true, FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, tc := range []struct{ mode, query, fts, want string }{
		{"prefix", "hea", "", "Blue heart"},
		{"suffix", "art", "", "Blue heart"},
		{"contains", "ear", "", "Blue heart"},
		{"word", "heart", "", "Blue heart"},
		{"wordLemma", "worked\x1fwork", "", "work"},
		{"exact", "work", "", "work"},
		{"exactLemma", "worked\x1fwork", "", "work"},
		{"fts", "cardiac", "\"cardiac\"*", "Blue heart"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var ids []int64
			err := s.StreamArticleIDs(context.Background(), tc.query, tc.mode, tc.fts,
				func(batch []int64, _ int, _ bool) error { ids = append(ids, batch...); return nil })
			if err != nil {
				t.Fatal(err)
			}
			results, err := s.ArticlesByIDs(context.Background(), ids)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) == 0 || results[0].Headword != tc.want {
				t.Fatalf("results = %+v; want %s", results, tc.want)
			}
			if tc.want == "work" && len(results) != 2 {
				t.Fatalf("duplicate headword must retain both articles: %+v", results)
			}
		})
	}
}
