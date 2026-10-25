// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package indexconst

import "testing"

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
