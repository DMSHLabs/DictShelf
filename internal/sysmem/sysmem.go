// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package sysmem reads how much memory the machine has and has free, and
// keeps the budget preparing dictionaries may use (D182).
//
// Preparing a large dictionary goes faster with more memory - parallel parse
// workers, decoded text held in memory instead of a temporary file, a page
// cache that holds the headword index - but it runs beside everything else
// the person is doing, and on a phone beside a low-memory killer. So the
// budget is a share of what the system reports free when a preparation
// starts, never more than an eighth of the machine, and under a Go memory
// limit (Android) half of what that limit leaves.
package sysmem

import (
	"math"
	"runtime/debug"
	"runtime/metrics"
	"sync"
	"sync/atomic"
)

const (
	// floorBudget is the least a budget is, on a machine with the RAM for it.
	floorBudget = 256 << 20
	// cap32 bounds the budget of a 32-bit build, whose address space is the
	// tighter limit (as textMax bounds its text, wmd/load.go).
	cap32 = 512 << 20
)

// Total is the machine's physical memory, 0 when it cannot be read. Read once:
// it does not change while the process runs.
var Total = sync.OnceValue(total)

// Available is the memory the system can hand out now without paging, and
// whether it could be read.
func Available() (int64, bool) { return available() }

// configured is the Go memory limit the configuration set (MEMORY_LIMIT), 0
// for none. A GOMEMLIMIT present at process start counts: it is read here
// before anything can relax it.
var configured atomic.Int64

func init() {
	if l := debug.SetMemoryLimit(-1); l < math.MaxInt64 {
		configured.Store(l)
	}
}

// SetConfiguredLimit records the Go memory limit the configuration set. The
// ceiling the server raises when shedding fails (server.relaxMemoryLimit) is
// never recorded: it is an emergency, not room to grow into.
func SetConfiguredLimit(n int64) {
	if n > 0 {
		configured.Store(n)
	}
}

// ConfiguredLimit is the Go memory limit the configuration set, 0 for none.
func ConfiguredLimit() int64 { return configured.Load() }

// Budget is the peak Go heap, in bytes, that preparing dictionaries may use
// now, all preparations together.
func Budget() int64 {
	avail, ok := Available()
	b := machineBudget(Total(), avail, ok)
	if l := configured.Load(); l > 0 {
		b = min(b, limitBudget(l, liveHeap(), avail, ok))
	}
	if math.MaxInt == math.MaxInt32 {
		b = min(b, cap32)
	}
	return b
}

// machineBudget is half of what is available, at most an eighth of the
// machine and at least min(floorBudget, an eighth); the floor alone when the
// available memory cannot be read.
func machineBudget(total, avail int64, ok bool) int64 {
	if total <= 0 {
		return floorBudget
	}
	hi := total / 8
	lo := min(floorBudget, hi)
	if !ok {
		return lo
	}
	return min(max(avail/2, lo), hi)
}

// limitBudget is half of what the limit l leaves above the live heap, between
// l/8 and l/2; then at most half of what the system has available, which may
// take it below l/8 - a preparation then runs at its floor.
func limitBudget(l, live, avail int64, ok bool) int64 {
	b := min(max(l/2-live, l/8), l/2)
	if ok {
		b = min(b, avail/2)
	}
	return max(b, 0)
}

// liveHeap is the heap the last collection found live; 0 before the first
// collection, so a preparation at process start sees the whole limit.
func liveHeap() int64 {
	s := []metrics.Sample{{Name: "/gc/heap/live:bytes"}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return int64(s[0].Value.Uint64())
}

// The pool every preparation draws from: one Budget for the process, so the
// background preparations the processor-derived limit allows and the one a person waits on
// never sum past it. Its unit is peak Go heap bytes; the capacity is taken
// anew whenever nothing is drawn. Nothing waits on it: work it does not admit
// runs at its floor - the one-core shape a preparation has without it.
var pool struct {
	sync.Mutex
	capacity, used int64
}

// An Account is one preparation's draw on the pool. The zero value is ready.
type Account struct {
	mu   sync.Mutex
	held int64
}

// TryAcquire takes n when it fits, and reports whether it did. n <= 0 always
// fits.
func (a *Account) TryAcquire(n int64) bool {
	if n <= 0 {
		return true
	}
	pool.Lock()
	idle := pool.used == 0
	pool.Unlock()
	var fresh int64
	if idle {
		fresh = Budget() // outside the lock: it reads the system
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	pool.Lock()
	defer pool.Unlock()
	if idle && pool.used == 0 {
		pool.capacity = fresh
	}
	if pool.used+n > pool.capacity {
		return false
	}
	pool.used += n
	a.held += n
	return true
}

// Held is what the account holds now.
func (a *Account) Held() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.held
}

// Release returns n of what the account holds.
func (a *Account) Release(n int64) {
	a.mu.Lock()
	n = min(max(n, 0), a.held)
	a.held -= n
	a.mu.Unlock()
	pool.Lock()
	pool.used -= n
	pool.Unlock()
}

// Close returns everything the account holds. Safe to call more than once.
func (a *Account) Close() {
	a.mu.Lock()
	n := a.held
	a.held = 0
	a.mu.Unlock()
	pool.Lock()
	pool.used -= n
	pool.Unlock()
}
