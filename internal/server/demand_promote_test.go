// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "testing"

// PromoteDictionary is Browse's reorder-only request: it must lift a dictionary
// that is already waiting in the running index queue to the front, and it must
// never begin indexing - not when a queue is running and not when none is.
func TestPromoteDictionaryReordersWithoutStarting(t *testing.T) {
	s, e := unpreparedEntry(t)

	// No queue is running: nothing to reorder, and above all nothing started.
	queued, err := s.PromoteDictionary(e.ID)
	if err != nil {
		t.Fatalf("PromoteDictionary: %v", err)
	}
	if queued {
		t.Fatalf("a dictionary that is in no queue read as queued")
	}
	if e.demanded.Load() {
		t.Fatalf("promoting started indexing")
	}
	if prepared(e) {
		t.Fatalf("promoting prepared an index")
	}

	// A queue is running with this dictionary behind two others. The same call
	// is a reorder, and still starts nothing.
	queue := newPriorityDictionaryQueue([]pendingDictionaryIndex{
		{entry: &entry{ID: "first"}},
		{entry: &entry{ID: "second"}},
		{entry: e},
	}, nil)
	s.reg.priorityMu.Lock()
	s.reg.priorityQueue = queue
	s.reg.priorityMu.Unlock()

	queued, err = s.PromoteDictionary(e.ID)
	if err != nil {
		t.Fatalf("PromoteDictionary: %v", err)
	}
	if !queued {
		t.Fatalf("a dictionary waiting in the running queue must report as queued")
	}
	if ids := queue.idsLocked(); len(ids) == 0 || ids[0] != e.ID {
		t.Fatalf("promoted dictionary is not first in the queue: %v", ids)
	}
	if e.demanded.Load() {
		t.Fatalf("promoting started indexing")
	}
	if prepared(e) {
		t.Fatalf("promoting prepared an index")
	}
}
