// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/wuweidict/wudict/internal/indexconst"
)

func TestAutomaticPipelineCost(t *testing.T) {
	for _, tc := range []struct {
		strategy indexconst.Mode
		budget   int
		want     int
	}{
		{indexconst.ModeAuto, 7, 4},
		{indexconst.ModeAuto2, 7, 3},
		{indexconst.ModeAuto2, 2, 2},
	} {
		if got := automaticPipelineCost(tc.strategy, tc.budget); got != tc.want {
			t.Errorf("automaticPipelineCost(%s, %d) = %d, want %d", indexconst.ModeString(tc.strategy), tc.budget, got, tc.want)
		}
	}
}

func TestAuto2HybridQueueRunsTwoLargeDictionaries(t *testing.T) {
	const budget = 7
	const count = 4
	items := make([]pendingDictionaryIndex, count)
	for i := range items {
		items[i] = pendingDictionaryIndex{
			entry: &entry{ID: string(rune('a' + i))},
			mode:  indexconst.ModePipeline,
			cost:  automaticPipelineCost(indexconst.ModeAuto2, budget),
		}
	}
	queue := newPriorityDictionaryQueue(items, nil)
	started := make(chan struct{}, count)
	release := make(chan struct{}, count)
	done := make(chan struct{})
	var active, peak atomic.Int32
	go func() {
		runHybridPriorityDictionaryQueue(queue, budget, nil, func(item pendingDictionaryIndex) {
			n := active.Add(1)
			for previous := peak.Load(); n > previous; previous = peak.Load() {
				if peak.CompareAndSwap(previous, n) {
					break
				}
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
		})
		close(done)
	}()

	waitStarted := func() {
		t.Helper()
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for a pipeline task to start")
		}
	}
	waitStarted()
	waitStarted()
	if got := peak.Load(); got != 2 {
		t.Fatalf("peak concurrent pipelines after filling budget = %d, want 2", got)
	}
	release <- struct{}{}
	release <- struct{}{}
	for range 2 {
		waitStarted()
		release <- struct{}{}
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("hybrid queue did not finish")
	}
	if got := peak.Load(); got != 2 {
		t.Fatalf("peak concurrent pipelines = %d, want 2", got)
	}
}
