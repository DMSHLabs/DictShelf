// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package indexconst

import (
	"sync"
	"sync/atomic"
)

type Mode uint32

const (
	ModePipeline Mode = iota
	ModeParallelDictionariesSingleWorker
)

var activeMode atomic.Uint32
var modeMu sync.Mutex
var activeIngestScans int

func CurrentMode() Mode { return Mode(activeMode.Load()) }

func ModeName() string {
	if CurrentMode() == ModeParallelDictionariesSingleWorker {
		return "parallel-dictionaries-single-worker"
	}
	return "pipeline"
}

func BeginIngest() Mode {
	modeMu.Lock()
	activeIngestScans++
	mode := CurrentMode()
	modeMu.Unlock()
	return mode
}

func EndIngest() {
	modeMu.Lock()
	activeIngestScans--
	modeMu.Unlock()
}

func SetMode(name string) (changed, busy bool) {
	modeMu.Lock()
	defer modeMu.Unlock()
	if activeIngestScans != 0 {
		return false, true
	}
	switch name {
	case "pipeline":
		activeMode.Store(uint32(ModePipeline))
		return true, false
	case "parallel-dictionaries-single-worker":
		activeMode.Store(uint32(ModeParallelDictionariesSingleWorker))
		return true, false
	default:
		return false, false
	}
}

// IndexDictionaryLimit leaves one logical processor for the rest of the app
// when possible and budgets a reader, writer, and two preparation workers per
// dictionary. Small machines still admit one dictionary.
func IndexDictionaryLimit(logical int) int {
	usable := max(1, logical-1)
	return max(1, usable/4)
}

// IndexDictionaryLimitForMode returns the dictionary-job limit used by the
// active comparative run. IndexDictionaryLimit retains the baseline formula.
func IndexDictionaryLimitForMode(logical int) int {
	if CurrentMode() == ModeParallelDictionariesSingleWorker {
		return max(1, logical-1)
	}
	return IndexDictionaryLimit(logical)
}

// IngestWorkerLimit shares the processors left after active readers and
// writers among all dictionaries. The writer prepares work itself when no
// processor lane remains, which also covers single-core devices.
func IngestWorkerLimit(logical, activeDictionaries int) int {
	usable := max(1, logical-1)
	return max(0, usable-2*max(0, activeDictionaries))
}

// IngestWorkerLimitForMode disables shared entry preparation for the current
// comparative run so each dictionary prepares its own batches inline.
func IngestWorkerLimitForMode(logical, activeDictionaries int) int {
	if CurrentMode() == ModeParallelDictionariesSingleWorker {
		return 0
	}
	return IngestWorkerLimit(logical, activeDictionaries)
}
