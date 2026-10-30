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
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// This reader reuses its headword slice on every Next. The ingest reader must
// own queued entries before it advances the source, even when workers finish
// out of order.
type pipelineReader struct {
	meta       dict.Meta
	count, pos int
	words      [2]string
	cancel     context.CancelFunc
	cancelAt   int
	badAt      int
	firstBody  string
}

type blockedPipelineReader struct {
	*pipelineReader
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (r *blockedPipelineReader) Next() (dict.Entry, error) {
	r.once.Do(func() {
		close(r.started)
		<-r.release
	})
	return r.pipelineReader.Next()
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
	if r.cancel != nil && i == r.cancelAt {
		r.cancel()
	}
	r.words[0] = fmt.Sprintf("word-%03d", i)
	r.words[1] = fmt.Sprintf("alias-%03d", i)
	body := "<p>article</p>"
	if i == 0 {
		body = "<p>" + strings.Repeat("large article ", 100000) + "</p>"
		if r.firstBody != "" {
			body = r.firstBody
		}
	}
	e := dict.Entry{Headwords: r.words[:], Body: body}
	if r.badAt > 0 && i == r.badAt {
		e.Kind = dict.BodyKind(99)
	}
	return e, nil
}

func TestIngestPipelineConcurrentOrderAndAliases(t *testing.T) {
	const dictionaries, entries = 2, 520
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
		for _, alias := range []string{"alias-519", "redirect"} {
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

func TestIngestBlockedDictionaryDoesNotStopAnother(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	slow := &blockedPipelineReader{
		pipelineReader: &pipelineReader{meta: dict.Meta{Name: "Blocked", Format: "mdx", EntryCount: 2}, count: 1},
		started:        started,
		release:        release,
	}
	slowPath := filepath.Join(t.TempDir(), "slow.db")
	fastPath := filepath.Join(t.TempDir(), "fast.db")
	slowDone := make(chan error, 1)
	go func() {
		_, err := IngestPlanContext(context.Background(), slow, slowPath, Plan{}, nil)
		slowDone <- err
	}()
	<-started
	fast := &pipelineReader{meta: dict.Meta{Name: "Fast", Format: "mdx", EntryCount: 521}, count: 520}
	fastDone := make(chan error, 1)
	go func() {
		_, err := IngestPlanContext(context.Background(), fast, fastPath, Plan{Contains: true}, nil)
		fastDone <- err
	}()
	select {
	case err := <-fastDone:
		if err != nil {
			close(release)
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("another dictionary waited for the blocked reader")
	}
	close(release)
	if err := <-slowDone; err != nil {
		t.Fatal(err)
	}
	assertIngestPoolIdle(t)
}

func TestIngestPipelineCancellationReleasesQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "text.db")
	r := &pipelineReader{meta: dict.Meta{Name: "Canceled", Format: "mdx", EntryCount: 1001}, count: 1000, cancel: cancel, cancelAt: 300}
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

func TestIngestPipelineArticleExceedingQueueBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "text.db")
	r := &pipelineReader{
		meta:      dict.Meta{Name: "Oversized", Format: "mdx", EntryCount: 2},
		count:     1,
		firstBody: "<p>" + strings.Repeat("large article ", 700000) + "</p>",
	}
	if _, err := IngestPlanContext(context.Background(), r, path, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Meta().EntryCount; got != 1 {
		t.Errorf("entries = %d, want 1", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	assertIngestPoolIdle(t)
}

func TestIngestPipelinePreparationErrorReleasesQueues(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
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

func TestIngestWriterPrefersReadyResult(t *testing.T) {
	p := &ingestPool{}
	s := &ingestSession{
		queue:   []*ingestWork{{sequence: 2}},
		results: make(chan preparedIngestBatch, 1),
	}
	sessions := []*ingestSession{s}
	p.sessions.Store(&sessions)
	s.results <- preparedIngestBatch{sequence: 1}

	result, ok := p.takeReadyOrInline(s)
	if !ok || result.sequence != 1 {
		t.Fatalf("writer took batch %d, ready=%v; want prepared batch 1", result.sequence, ok)
	}
	if len(s.queue) != 1 {
		t.Fatal("writer prepared queued work while a result was ready")
	}
}

func TestIngestQueuesDoNotShareTheirLock(t *testing.T) {
	previous := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(previous)
	p := &ingestPool{wake: make(chan struct{})}
	busy := &ingestSession{changed: make(chan struct{})}
	other := &ingestSession{ctx: context.Background(), changed: make(chan struct{})}
	p.memCapacity.Store(1 << 20)
	sessions := []*ingestSession{busy, other}
	p.sessions.Store(&sessions)
	p.workers.Store(1)
	busy.mu.Lock()
	done := make(chan error, 1)
	go func() {
		item := &ingestWork{weight: 1}
		if err := p.submit(other, item); err != nil {
			done <- err
			return
		}
		selected, got := p.next()
		if selected != other || got != item {
			done <- errors.New("worker selected another dictionary")
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		busy.mu.Unlock()
		t.Fatal("another dictionary's queue or worker blocked on this dictionary's lock")
	}
	busy.mu.Unlock()
	p.release(other, preparedIngestBatch{weight: 1})
}

func TestIngestWorkerRechecksContendedQueue(t *testing.T) {
	previous := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(previous)
	p := &ingestPool{wake: make(chan struct{})}
	s := &ingestSession{ctx: context.Background(), changed: make(chan struct{})}
	p.memCapacity.Store(1 << 20)
	sessions := []*ingestSession{s}
	p.sessions.Store(&sessions)
	p.workers.Store(1)
	item := &ingestWork{weight: 1}
	if err := p.submit(s, item); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	done := make(chan *ingestWork, 1)
	go func() {
		_, got := p.next()
		done <- got
	}()
	time.Sleep(10 * time.Millisecond)
	s.mu.Unlock()
	select {
	case got := <-done:
		if got != item {
			t.Fatal("worker selected the wrong batch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not revisit a previously locked queue")
	}
	p.release(s, preparedIngestBatch{weight: 1})
}

func assertIngestPoolIdle(t *testing.T) {
	t.Helper()
	current := sharedIngestPool.sessions.Load()
	sessionCount := 0
	if current != nil {
		sessionCount = len(*current)
	}
	running, bytes := sharedIngestPool.running.Load(), sharedIngestPool.usedBytes.Load()
	if sessionCount != 0 || bytes != 0 || running != 0 {
		t.Errorf("pool retains sessions=%d bytes=%d running=%d", sessionCount, bytes, running)
	}
}
