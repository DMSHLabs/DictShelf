// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Preparing a dictionary runs in parallel only while a person waits on it
// (D182). Otherwise it keeps to one core: INDEX_WORKERS promises that a
// background preparation takes one core, so that the machine stays usable.
// The server marks a source for as long as someone waits on it; a command
// line run waits on everything it prepares, so there the default is urgent.
// The work reads the mark at every boundary it can change course at, both
// ways.

var urgent = struct {
	sync.Mutex
	m map[string]int
}{m: map[string]int{}}

var urgentDefault atomic.Bool

func init() { urgentDefault.Store(true) }

// SetUrgentDefault sets whether an unmarked source is urgent: true on the
// command line, false in the server, which marks what a person waits on. It
// returns the default it replaced.
func SetUrgentDefault(v bool) (was bool) { return urgentDefault.Swap(v) }

// MarkUrgent marks path as waited on until the returned release is called
// (more than once is harmless). Marks nest. A folder's mark also covers the
// main file its preparation reads (MainFile).
func MarkUrgent(path string) (release func()) {
	keys := []string{urgentKey(path)}
	if m := MainFile(path); m != path {
		keys = append(keys, urgentKey(m))
	}
	urgent.Lock()
	for _, k := range keys {
		urgent.m[k]++
	}
	urgent.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			urgent.Lock()
			for _, k := range keys {
				if urgent.m[k]--; urgent.m[k] <= 0 {
					delete(urgent.m, k)
				}
			}
			urgent.Unlock()
		})
	}
}

// Urgent reports whether a person waits on preparing path.
func Urgent(path string) bool {
	if urgentDefault.Load() {
		return true
	}
	k := urgentKey(path)
	urgent.Lock()
	n := urgent.m[k]
	urgent.Unlock()
	return n > 0
}

// urgentKey is path as store's text locks key it: absolute and clean.
func urgentKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// selfPrepared holds the suffixes of the formats that prepare themselves
// inside Open (DSL, BGL, wudict markdown): opening one unprepared is a
// preparation, and a person waiting on that open waits on it.
var selfPrepared = map[string]bool{}

// RegisterSelfPrepared records that the format of ext prepares itself inside
// Open.
func RegisterSelfPrepared(ext string) { selfPrepared[strings.ToLower(ext)] = true }

// SelfPrepares reports whether opening path may prepare it.
func SelfPrepares(path string) bool {
	_, ok := matchKey(selfPrepared, path)
	return ok
}
