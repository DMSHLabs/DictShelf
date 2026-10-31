// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"runtime"
	"sync/atomic"

	"github.com/DMSHLabs/DictShelf/internal/search"
)

// searchWorkersBaseline is the fan-out the platform chose at startup - what
// SetWorkers was given in NewRuntime. Zero means "the library's own default",
// which is the desktop case: MaxProcs there declines to cap anything.
var searchWorkersBaseline int

// searchThreadsPref is the saved "maximum search threads" value, 0 meaning
// automatic. Kept here rather than read from prefs on every call so the power
// state can re-derive the fan-out without touching disk.
var searchThreadsPref atomic.Int32

// searchFanoutCeiling is the widest fan-out this machine may actually use,
// whatever the setting asks for. Three bounds, all of them ceilings rather than
// targets:
//
//   - one CPU less than the process may run on, so the platform and the reader
//     keep a core;
//   - the Go scheduler's current allowance, which is what carries Android's
//     battery-saver and thermal demotions (applyProcs lowers GOMAXPROCS);
//   - the platform baseline when one exists - Android's "half the cores, at
//     most four" - so a user setting can narrow the fan-out but never widen it
//     past the policy the phone was given.
func searchFanoutCeiling() int {
	logical := currentIndexCPUCount()
	if logical < 1 {
		logical = runtime.NumCPU()
	}
	ceiling := max(1, logical-1)
	if g := runtime.GOMAXPROCS(0); g >= 1 && g < ceiling {
		ceiling = g
	}
	if searchWorkersBaseline > 0 && searchWorkersBaseline < ceiling {
		ceiling = searchWorkersBaseline
	}
	return ceiling
}

// applySearchParallelismPrefs records the saved SearchThreads and applies it.
func (s *Server) applySearchParallelismPrefs() {
	if s == nil || s.reg == nil {
		return
	}
	var n int
	if ui := s.reg.prefs.UI(); ui != nil {
		n = ui.SearchThreads
	}
	searchThreadsPref.Store(int32(n))
	applySearchWorkers()
}

// defaultSearchThreads is the width automatic mode asks for. Measured over a
// 110-dictionary Wildcards build on a 24-core desktop, widening the fan-out past
// this changed the run time by nothing that survived the noise: the wall clock
// is set by the slowest dictionary and by how the work distributes, not by the
// cap. Five is where the curve flattens, so that is what "automatic" means -
// still narrowed by the ceiling below, which is what keeps a phone at four.
const defaultSearchThreads = 5

// applySearchWorkers sizes the fan-out from the saved setting, clamped to what
// this machine may use. Called again whenever the power state moves, so a
// battery-saver or thermal demotion reaches the fan-out too.
func applySearchWorkers() {
	n := int(searchThreadsPref.Load())
	if n <= 0 {
		n = defaultSearchThreads
	}
	search.SetWorkers(min(n, searchFanoutCeiling()))
}
