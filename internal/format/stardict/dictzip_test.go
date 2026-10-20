// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package stardict

import (
	"bytes"
	"compress/flate"
	"io"
	"sync"
	"testing"
)

// barrierReaderAt makes two cache misses reach the compressed chunk read
// before either can finish inflating and populate the cache.
type barrierReaderAt struct {
	data  []byte
	mu    sync.Mutex
	reads int
	once  sync.Once
	ready chan struct{}
}

func (r *barrierReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.mu.Lock()
	r.reads++
	if r.reads == 2 {
		r.once.Do(func() { close(r.ready) })
	}
	r.mu.Unlock()
	<-r.ready
	if off < 0 || off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestDictzipConcurrentChunkMissAddsOneCacheKey(t *testing.T) {
	plain := []byte("shared dictzip chunk")
	var compressed bytes.Buffer
	w, err := flate.NewWriter(&compressed, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	ra := &barrierReaderAt{data: compressed.Bytes(), ready: make(chan struct{})}
	d := &dzReader{
		ra: ra, chunkLen: len(plain), offsets: []int64{0},
		sizes: []int{compressed.Len()}, cache: make(map[int][]byte),
	}
	results := make(chan []byte, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			got, err := d.chunk(0)
			results <- got
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("chunk: %v", err)
		}
		if got := <-results; !bytes.Equal(got, plain) {
			t.Fatalf("chunk = %q, want %q", got, plain)
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.cacheKeys) != 1 || d.cacheKeys[0] != 0 {
		t.Fatalf("cache FIFO = %v, want [0]", d.cacheKeys)
	}
}
