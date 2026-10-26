// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package indexconst

import (
	"context"
	"testing"
)

func TestIndexParallelismFromLogicalProcessors(t *testing.T) {
	for _, tc := range []struct {
		logical, dictionaries, workers int
	}{
		{1, 1, 0},
		{2, 1, 0},
		{4, 1, 1},
		{5, 1, 2},
		{8, 1, 5},
		{24, 5, 13},
	} {
		if got := IndexDictionaryLimit(tc.logical); got != tc.dictionaries {
			t.Errorf("IndexDictionaryLimit(%d) = %d, want %d", tc.logical, got, tc.dictionaries)
		}
		if got := IngestWorkerLimit(tc.logical, tc.dictionaries); got != tc.workers {
			t.Errorf("IngestWorkerLimit(%d, %d) = %d, want %d", tc.logical, tc.dictionaries, got, tc.workers)
		}
	}
}

func TestAuto2UsesSharedIngestPoolForMixedQueues(t *testing.T) {
	defer SetMode("pipeline")
	for _, mode := range []string{"auto", "auto2"} {
		if changed, busy := SetMode(mode); !changed || busy {
			t.Fatalf("SetMode(%q) = (%v, %v)", mode, changed, busy)
		}
		ctx := WithIngestMode(context.Background(), ModeParallelDictionariesSingleWorker)
		got, autoSingle := BeginIngestContext(ctx)
		if got != ModeParallelDictionariesSingleWorker || !autoSingle {
			t.Fatalf("%s single-worker ingest = (%v, %v)", mode, got, autoSingle)
		}
		if workers := IngestWorkerLimitForMode(8, 2); workers != 2 {
			t.Errorf("%s worker limit during mixed queue = %d, want 2", mode, workers)
		}
		EndIngestContext(autoSingle)
		if workers := IngestWorkerLimitForMode(8, 2); workers != 3 {
			t.Errorf("%s worker limit after single task = %d, want 3", mode, workers)
		}
	}
}
