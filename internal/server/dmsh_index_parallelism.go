// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// indexSemaphore is resized between indexing jobs. A limit reduction never
// interrupts work already holding a slot; new work waits until usage is below
// the refreshed limit.
type indexSemaphore struct {
	mu      sync.Mutex
	changed *sync.Cond
	limit   int
	active  int
}

func newIndexSemaphore(limit int) *indexSemaphore {
	s := &indexSemaphore{limit: max(1, limit)}
	s.changed = sync.NewCond(&s.mu)
	return s
}

func (s *indexSemaphore) Limit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limit
}

func (s *indexSemaphore) SetLimit(limit int) {
	s.mu.Lock()
	s.limit = max(1, limit)
	s.changed.Broadcast()
	s.mu.Unlock()
}

func (s *indexSemaphore) Acquire() (release func()) {
	s.mu.Lock()
	for s.active >= s.limit {
		s.changed.Wait()
	}
	s.active++
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.active--
			s.changed.Broadcast()
			s.mu.Unlock()
		})
	}
}

var (
	indexLogicalCPUs             atomic.Int32
	indexLimit                   = newIndexSemaphore(indexconst.IndexDictionaryLimit(runtime.NumCPU()))
	parallelDictionaryIndexLimit = newIndexSemaphore(max(1, runtime.NumCPU()-1))
)

func init() {
	indexLogicalCPUs.Store(int32(max(1, runtime.NumCPU())))
}

// refreshIndexParallelism samples Android's current process CPU affinity at
// each indexing job boundary. runtime.NumCPU is a startup snapshot; the app's
// power state separately applies battery-saver and thermal limits.
func refreshIndexParallelism() int {
	logical := runtime.NumCPU()
	if runtime.GOOS == "android" {
		if n := processAllowedCPUs(); n > 0 {
			logical = n
		}
		indexLogicalCPUs.Store(int32(max(1, logical)))
		store.SetIngestParallelismCPUCount(logical)
		SetActiveProcs(max(1, logical-1)) // leave one allowed CPU for Android and the reader
		return logical
	}
	indexLimit.SetLimit(indexconst.IndexDictionaryLimit(logical))
	parallelDictionaryIndexLimit.SetLimit(max(1, logical-1))
	indexLogicalCPUs.Store(int32(logical))
	store.SetIngestParallelismCPUCount(logical)
	return logical
}

// updateIndexParallelismLimits applies the current Go scheduler cap to the
// dictionary queues. A thermal or power-save change can lower these while a
// user-started indexing operation is already running.
func updateIndexParallelismLimits(logical, gomaxprocs int) {
	if logical < 1 || gomaxprocs < 1 {
		return
	}
	effectiveLogical := min(logical, gomaxprocs+1) // GOMAXPROCS already reserves one CPU
	indexLimit.SetLimit(indexconst.IndexDictionaryLimit(effectiveLogical))
	parallelDictionaryIndexLimit.SetLimit(max(1, min(logical-1, gomaxprocs)))
}

func currentIndexCPUCount() int {
	if n := int(indexLogicalCPUs.Load()); n > 0 {
		return n
	}
	return max(1, runtime.NumCPU())
}

func processAllowedCPUs() int {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		value, ok := strings.CutPrefix(line, "Cpus_allowed_list:")
		if !ok {
			continue
		}
		count := 0
		for _, part := range strings.Split(strings.TrimSpace(value), ",") {
			ends := strings.SplitN(part, "-", 2)
			first, err := strconv.Atoi(ends[0])
			if err != nil || first < 0 {
				return 0
			}
			last := first
			if len(ends) == 2 {
				last, err = strconv.Atoi(ends[1])
				if err != nil || last < first {
					return 0
				}
			}
			count += last - first + 1
		}
		return count
	}
	return 0
}

func currentIndexLimit() *indexSemaphore {
	if indexconst.CurrentMode() == indexconst.ModeParallelDictionariesSingleWorker {
		return parallelDictionaryIndexLimit
	}
	return indexLimit
}

func indexLaneForContext(ctx context.Context) *indexSemaphore {
	if indexconst.IsAutomaticMode(indexconst.CurrentMode()) && indexconst.IngestMode(ctx) == indexconst.ModeParallelDictionariesSingleWorker {
		return parallelDictionaryIndexLimit
	}
	return currentIndexLimit()
}
