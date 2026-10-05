// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"

	"github.com/wuweidict/wudict/internal/logx"
)

// meter is the in-place counter of a long CLI pass: how many items are done,
// of how many when the source declares it, and the bytes written when the
// items are files.
//
//	41000/118000 entries
//	1200/40000 resources · 120 MB
//
// It redraws once per every items, so a pass pays an increment and a compare
// per item. A nil meter - what newMeter returns when stderr is not a terminal -
// does nothing, so callers never test for one.
type meter struct {
	label string // "entries", "entries read", "resources"
	total int    // 0 when unknown: then no denominator
	bytes bool   // show the bytes counted so far
	every int
	n     int
	next  int // the count at which the line is redrawn
	b     int64
}

const (
	// entryEvery redraws an entry counter about once a second on the slowest
	// pass (a -mode clean dump, a few thousand entries a second) and a few
	// hundred times a second on the fastest.
	entryEvery = 1000
	// resourceEvery is lower: one resource can be a large audio file.
	resourceEvery = 16
)

// newMeter starts a counter of total items (0: unknown), nil when stderr is
// not a terminal.
func newMeter(label string, total, every int) *meter {
	if !logx.Interactive() {
		return nil
	}
	every = max(every, 1)
	return &meter{label: label, total: max(total, 0), every: every, next: every}
}

// Add counts n more items done.
func (m *meter) Add(n int) {
	if m == nil {
		return
	}
	m.n += n
	m.tick()
}

// Set records done items, for a pass that knows its own position.
func (m *meter) Set(done int) {
	if m == nil {
		return
	}
	m.n = done
	m.tick()
}

// AddBytes counts b more bytes written, shown when the meter shows bytes.
func (m *meter) AddBytes(b int64) {
	if m != nil {
		m.b += b
	}
}

// Clear erases the counter, before any other line is printed.
func (m *meter) Clear() {
	if m != nil {
		logx.ClearLine()
	}
}

func (m *meter) tick() {
	if m.n < m.next {
		return
	}
	m.next = m.n + m.every
	logx.Progress("%s", m.line())
}

func (m *meter) line() string {
	var b strings.Builder
	// A source whose declared count is short of what it yields loses its
	// denominator rather than showing 105%.
	if m.total > 0 && m.n <= m.total {
		fmt.Fprintf(&b, "  %d/%d %s", m.n, m.total, m.label)
	} else {
		fmt.Fprintf(&b, "  %d %s", m.n, m.label)
	}
	if m.bytes {
		b.WriteString(" · " + logx.Size(m.b))
	}
	return b.String()
}
