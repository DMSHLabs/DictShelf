// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"testing"
)

// These opt-in benchmarks use a read-only real ZIM archive. Set
// WUDICT_TEST_ZIM to compare cold same-cluster misses or repeated reads of the
// largest stored-cluster offset table in that archive. They are intended for
// before/after measurements, not as a timing gate in ordinary test runs.
func openPerfCompareContainer(b *testing.B) (*container, uint32, uint32, uint32) {
	b.Helper()
	path := os.Getenv("WUDICT_TEST_ZIM")
	if path == "" {
		b.Skip("WUDICT_TEST_ZIM not set")
	}
	c, err := openContainer(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = c.Close() })

	var compressed, direct, directBlobs uint32
	haveCompressed, haveDirect := false, false
	for i := uint32(0); i < c.clusterCount; i++ {
		start, end := c.extent(i)
		var head [9]byte
		if _, err := c.f.ReadAt(head[:1], int64(start)); err != nil {
			b.Fatal(err)
		}
		codec, extended := head[0]&0x0f, head[0]&0x10 != 0
		if codec > 1 && !haveCompressed {
			compressed, haveCompressed = i, true
		}
		if codec > 1 || end <= start+1 {
			continue
		}
		w := uint64(4)
		if extended {
			w = 8
		}
		if _, err := c.f.ReadAt(head[:w], int64(start)+1); err != nil {
			b.Fatal(err)
		}
		first := uint64(binary.LittleEndian.Uint32(head[:4]))
		if w == 8 {
			first = binary.LittleEndian.Uint64(head[:8])
		}
		size := end - start - 1
		if first >= w*2 && first <= size && first%w == 0 {
			count := uint32(first/w - 1)
			if !haveDirect || count > directBlobs {
				direct, directBlobs, haveDirect = i, count, true
			}
		}
	}
	if !haveCompressed || !haveDirect {
		b.Fatalf("archive needs compressed and stored clusters, got compressed=%v stored=%v", haveCompressed, haveDirect)
	}
	b.Logf("archive clusters: compressed sample=%d; stored sample=%d with %d blobs", compressed, direct, directBlobs)
	return c, compressed, direct, directBlobs
}

func BenchmarkRealConcurrentClusterMiss(b *testing.B) {
	c, clusterID, _, _ := openPerfCompareContainer(b)
	const workers = 8
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.mu.Lock()
		c.cache = make(map[uint32]*clusterData)
		c.order = nil
		c.cacheBytes = 0
		c.mu.Unlock()

		start := make(chan struct{})
		errs := make(chan error, workers)
		var wg sync.WaitGroup
		for j := 0; j < workers; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := c.cluster(clusterID)
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkRealStoredOffsetTable(b *testing.B) {
	c, _, clusterID, blobs := openPerfCompareContainer(b)
	blobID := blobs - 1
	if _, _, _, err := c.blobSpan(clusterID, blobID); err != nil {
		b.Fatal(fmt.Errorf("warm direct cluster: %w", err))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := c.blobSpan(clusterID, blobID); err != nil {
			b.Fatal(err)
		}
	}
}
