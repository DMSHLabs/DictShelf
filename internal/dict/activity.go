// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import "sync"

// Activity is what a long open or scan of a dictionary is doing now. Keyed by
// the source path the work was given; set by the code doing the work, and
// cleared by store's ingest, which every indexing runs through. For showing
// only: nothing decides on it.
type Activity struct {
	Phase       string // PhaseUnpacking or PhaseIndexing
	Done, Total int64  // in the phase's unit; Total 0 when it is not known
}

const (
	PhaseUnpacking = "unpacking" // bytes of text unpacked
	PhaseIndexing  = "indexing"  // entries indexed
)

var activities = struct {
	sync.Mutex
	m map[string]Activity
}{m: map[string]Activity{}}

// SetActivity records what the work on path is doing now.
func SetActivity(path string, a Activity) {
	activities.Lock()
	activities.m[path] = a
	activities.Unlock()
}

// ClearActivity forgets path's activity: its work is over.
func ClearActivity(path string) {
	activities.Lock()
	delete(activities.m, path)
	activities.Unlock()
}

// ActivityOf is what the work on path is doing, if anything says so.
func ActivityOf(path string) (Activity, bool) {
	activities.Lock()
	defer activities.Unlock()
	a, ok := activities.m[path]
	return a, ok
}
