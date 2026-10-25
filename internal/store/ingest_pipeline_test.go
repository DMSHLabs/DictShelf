// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

// This reader reuses its headword slice on every Next. The ingest reader must
// own queued entries before it advances the source, even when workers finish
// out of order.
type pipelineReader struct {
	meta       dict.Meta
	count, pos int
	words      [2]string
	cancel     context.CancelFunc
	badAt      int
}

func (r *pipelineReader) Meta() dict.Meta { return r.meta }
func (r *pipelineReader) Close() error    { return nil }
func (r *pipelineReader) Next() (dict.Entry, error) {
	if r.pos > r.count {
		return dict.Entry{}, io.EOF
	}
	if r.pos == r.count {
		r.pos++
		return dict.Entry{Headwords: []string{"redirect"}, LinkTo: "word-000"}, nil
	}
	i := r.pos
	r.pos++
	if r.cancel != nil && i == 32 {
		r.cancel()
	}
	r.words[0] = fmt.Sprintf("word-%03d", i)
	r.words[1] = fmt.Sprintf("alias-%03d", i)
	body := "<p>article</p>"
	if i == 0 {
		body = "<p>" + strings.Repeat("large article ", 100000) + "</p>"
	}
	e := dict.Entry{Headwords: r.words[:], Body: body}
	if r.badAt > 0 && i == r.badAt {
		e.Kind = dict.BodyKind(99)
	}
	return e, nil
}

func TestIngestPipelineConcurrentOrderAndAliases(t *testing.T) {
	const dictionaries, entries = 2, 80
	paths := make([]string, dictionaries)
	errCh := make(chan error, dictionaries)
	var wg sync.WaitGroup
	for i := 0; i < dictionaries; i++ {
		paths[i] = filepath.Join(t.TempDir(), "text.db")
		r := &pipelineReader{meta: dict.Meta{Name: fmt.Sprintf("Pipeline %d", i), Format: "mdx", EntryCount: entries + 1}, count: entries}
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			_, err := IngestPlanContext(context.Background(), r, path, Plan{Contains: true, FullText: true}, nil)
			errCh <- err
		}(paths[i])
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range paths {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := s.db.Query("SELECT id, w FROM entry ORDER BY id")
		if err != nil {
			s.Close()
			t.Fatal(err)
		}
		index := 0
		for rows.Next() {
			var id int
			var word string
			if err := rows.Scan(&id, &word); err != nil {
				t.Fatal(err)
			}
			if id != index+1 || word != fmt.Sprintf("word-%03d", index) {
				t.Errorf("row %d = (%d, %q)", index, id, word)
			}
			index++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if index != entries {
			t.Errorf("entries = %d, want %d", index, entries)
		}
		for _, alias := range []string{"alias-079", "redirect"} {
			hits, err := s.Exact(alias, 10)
			if err != nil || len(hits) != 1 {
				t.Errorf("Exact(%q) = %v, %v", alias, hits, err)
			}
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	assertIngestPoolIdle(t)
}

func TestIngestPipelineCancellationReleasesQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "text.db")
	r := &pipelineReader{meta: dict.Meta{Name: "Canceled", Format: "mdx", EntryCount: 1001}, count: 1000, cancel: cancel}
	_, err := IngestPlanContext(ctx, r, path, Plan{FullText: true}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ingest error = %v, want canceled", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("canceled ingest published its database")
	}
	assertIngestPoolIdle(t)
}

func TestIngestPipelineWithoutProcessorLane(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	path := filepath.Join(t.TempDir(), "text.db")
	r := &pipelineReader{meta: dict.Meta{Name: "Single core", Format: "mdx", EntryCount: 5}, count: 4}
	if _, err := IngestPlanContext(context.Background(), r, path, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := s.Meta().EntryCount; got != 4 {
		t.Fatalf("entries = %d, want 4", got)
	}
	assertIngestPoolIdle(t)
}

func TestIngestPipelinePreparationErrorReleasesQueues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "text.db")
	r := &pipelineReader{meta: dict.Meta{Name: "Broken", Format: "mdx", EntryCount: 201}, count: 200, badAt: 20}
	_, err := IngestPlanContext(context.Background(), r, path, Plan{FullText: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported body kind") {
		t.Fatalf("ingest error = %v, want unsupported body kind", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("failed ingest published its database")
	}
	assertIngestPoolIdle(t)
}

func assertIngestPoolIdle(t *testing.T) {
	t.Helper()
	sharedIngestPool.mu.Lock()
	defer sharedIngestPool.mu.Unlock()
	if len(sharedIngestPool.sessions) != 0 || sharedIngestPool.usedBytes != 0 || sharedIngestPool.running != 0 {
		t.Errorf("pool retains sessions=%d bytes=%d running=%d", len(sharedIngestPool.sessions), sharedIngestPool.usedBytes, sharedIngestPool.running)
	}
}
