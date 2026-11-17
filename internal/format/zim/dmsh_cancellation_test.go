// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package zim

import (
	"context"
	"errors"
	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRealArchiveCancellation(t *testing.T) {
	dir := os.Getenv("WUDICT_TEST_ZIM_DIR")
	if dir == "" {
		t.Skip("WUDICT_TEST_ZIM_DIR not set")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.zim"))
	if err != nil || len(files) == 0 {
		t.Fatalf("archives: %v", err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			d, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			w, n, err := d.XapianIndex()
			if err != nil {
				t.Fatal(err)
			}
			db, err := glass.OpenSource(w, n)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			visited := 0
			db.Progress = func(uint32) {
				visited++
				if visited == 100 {
					cancel()
				}
			}
			start := time.Now()
			result, err := db.SearchContext(ctx, "год")
			if !errors.Is(err, context.Canceled) || visited != 100 || len(result.Hits) != 0 {
				t.Fatalf("err=%v visited=%d hits=%d", err, visited, len(result.Hits))
			}
			t.Logf("index stopped after %d postings (%s)", visited, time.Since(start))
			ctx2, cancel2 := context.WithCancel(context.Background())
			defer cancel2()
			emitted := 0
			err = d.StreamHeadwordsContext(ctx2, "кларнетист", ReadingWords, 0, func(string) error { emitted++; cancel2(); return nil })
			if !errors.Is(err, context.Canceled) || emitted != 1 {
				t.Fatalf("stream err=%v emitted=%d", err, emitted)
			}
			ctx3, cancel3 := context.WithCancel(context.Background())
			cancel3()
			if r, e := d.FullTextContext(ctx3, "год", 5); !errors.Is(e, context.Canceled) || len(r) != 0 {
				t.Fatalf("full text: %v %v", r, e)
			}
			// A subsequent request on the same dictionary remains usable.
			if r, e := d.FullTextContext(context.Background(), "кларнетист", 1); e != nil || len(r) != 1 {
				t.Fatalf("subsequent search: results=%d err=%v", len(r), e)
			}
		})
	}
}
