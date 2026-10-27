// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package indexconst

import (
	"context"
	"sync"
	"sync/atomic"
)

type Mode uint32

const (
	ModePipeline Mode = iota
	ModeParallelDictionariesSingleWorker
	ModeAuto
	ModeAdaptive
)

const ModeAuto2 = ModeAdaptive // Legacy name for existing callers and tests.

var activeMode atomic.Uint32
var modeMu sync.Mutex
var activeIngestScans int
var activeAutoSingles atomic.Int32
var activePipelineOverrides atomic.Int32

type ingestModeKey struct{}

func WithIngestMode(ctx context.Context, mode Mode) context.Context {
	return context.WithValue(ctx, ingestModeKey{}, mode)
}

func IngestMode(ctx context.Context) Mode {
	if ctx != nil {
		if mode, ok := ctx.Value(ingestModeKey{}).(Mode); ok && !IsAutomaticMode(mode) {
			return mode
		}
	}
	if mode := CurrentMode(); !IsAutomaticMode(mode) {
		return mode
	}
	return ModePipeline
}

func IsAutomaticMode(mode Mode) bool {
	return mode == ModeAuto || mode == ModeAdaptive
}

func ModeString(mode Mode) string {
	switch mode {
	case ModeParallelDictionariesSingleWorker:
		return "parallel-dictionaries-single-worker"
	case ModeAuto:
		return "auto"
	case ModeAdaptive:
		return "adaptive"
	default:
		return "pipeline"
	}
}

func CurrentMode() Mode { return Mode(activeMode.Load()) }

func ModeName() string { return ModeString(CurrentMode()) }

func BeginIngest() Mode {
	modeMu.Lock()
	activeIngestScans++
	mode := CurrentMode()
	modeMu.Unlock()
	return mode
}

func BeginIngestContext(ctx context.Context) (Mode, bool) {
	modeMu.Lock()
	activeIngestScans++
	mode := IngestMode(ctx)
	autoSingle := IsAutomaticMode(CurrentMode()) && mode == ModeParallelDictionariesSingleWorker
	if autoSingle {
		activeAutoSingles.Add(1)
	}
	if CurrentMode() == ModeParallelDictionariesSingleWorker && mode == ModePipeline {
		activePipelineOverrides.Add(1)
	}
	modeMu.Unlock()
	return mode, autoSingle
}

func EndIngestContext(mode Mode, autoSingle bool) {
	modeMu.Lock()
	if autoSingle {
		activeAutoSingles.Add(-1)
	}
	if CurrentMode() == ModeParallelDictionariesSingleWorker && mode == ModePipeline {
		activePipelineOverrides.Add(-1)
	}
	activeIngestScans--
	modeMu.Unlock()
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
	case "auto":
		activeMode.Store(uint32(ModeAuto))
		return true, false
	case "adaptive", "auto2":
		activeMode.Store(uint32(ModeAdaptive))
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
	if CurrentMode() == ModeParallelDictionariesSingleWorker && activePipelineOverrides.Load() == 0 {
		return 0
	}
	if IsAutomaticMode(CurrentMode()) {
		return max(0, IngestWorkerLimit(logical, activeDictionaries)-int(activeAutoSingles.Load()))
	}
	return IngestWorkerLimit(logical, activeDictionaries)
}
