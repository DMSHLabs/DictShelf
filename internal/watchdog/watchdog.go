// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Package watchdog reports a loop that is ours when it stops moving.
//
// indexconst states the requirement beside the constants the search clocks
// left behind: where the loop is ours - headwords, article ids, archive records
// - "a thread that stops moving for a long while is a bug or a corrupt file,
// and it is reported with the dictionary, its size and the record it stopped
// on"; where the call is opaque (one SQLite MATCH) there is no progress to
// watch, so there is no limit there either, and the reader has Stop.
//
// What this package is NOT is most of the design, and each half of that is a
// decision the fork already made:
//
//   - It is not a clock. Nothing here bounds how long a loop may run: a loop
//     that keeps moving is never reported however long it takes, and a loop
//     that is reported is not ended - the report buys the hang a name, and the
//     reader keeps the Stop they already had. The one duration is StallSilence,
//     and it measures the ABSENCE of movement, which is the thing a deadline
//     cannot tell apart from a slow disk (indexconst, and the 388-dictionary
//     search every clock failed).
//   - It is not a setting. Nothing turns it on, off, up or down: a reader who
//     had to understand a stall threshold would be paying for our internals
//     (D102), and there is nothing to decide - the report is a fact about the
//     file, not a preference.
//   - It is not a second cancellation path. A reported loop keeps running; the
//     reader's Stop, and the fan-out's abandonment of a wedged query, are the
//     only ways work ends early (internal/search, wedge_test.go).
//
// A loop registers a Watch, calls Moved once per record and Done when it
// returns. One goroutine per process looks at every registered watch every
// stallPoll; a watch whose record has not changed for StallSilence is reported
// to the system log once, and again only if it moves and then stops again.
package watchdog

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// StallSilence is how long a loop may go without moving before it is reported.
//
// Five minutes is a long while by the standard of the loops that use this: the
// slowest record a healthy disk produces costs hundreds of milliseconds, and a
// corrupt file or a wedged read produces none at all, so the gap between the
// two is not a matter of tuning. Reporting late costs nothing (the loop is
// still running, and the reader's Stop still works); reporting early would name
// a dictionary that was merely slow, which is the mistake the search clocks
// were made of.
const StallSilence = 5 * time.Minute

// stallPoll is how often the monitor looks at its watches. It bounds how late
// a report can be, and nothing else: the silence is compared against the
// observation, not against this interval.
const stallPoll = 250 * time.Millisecond

// Desc names the loop a Watch belongs to and the dictionary it is reading.
type Desc struct {
	// Surface is which loop this is, in the log's own words. Each caller names
	// its own, and internal/store/dmsh_watchdog.go lists the store's.
	Surface string
	// Name is the dictionary's display name, the one the reader sees.
	Name string
	// Path is the file the loop reads, and Size its size in bytes (0 when the
	// caller could not stat it). The file rather than the dictionary folder:
	// a stall is a fact about the bytes that are not arriving.
	Path string
	Size int64
}

// Watch is one loop's movement. Moved is called from the loop; everything else
// belongs to the monitor.
type Watch struct {
	monitor *Monitor
	desc    Desc
	record  atomic.Int64

	// The monitor's own reading of this watch, touched only under the monitor's
	// mutex: the last record it saw, when it saw it, and whether it has reported
	// the silence since. A zero at means it has not looked yet, so a watch that
	// stalls before the first look is reported one poll later rather than
	// immediately.
	seen     int64
	at       time.Time
	reported bool
}

// Moved records that the loop is on record n. It is one atomic store and
// belongs on the hot path: the loop says where it is and does no timing of its
// own, because a loop that has stopped cannot time anything.
func (w *Watch) Moved(record int64) { w.record.Store(record) }

// Done ends the watch. A loop calls it when it returns, finished or failed: a
// loop that is no longer running cannot be stalled.
func (w *Watch) Done() { w.monitor.remove(w) }

// Monitor watches the loops registered with it. The zero value is not usable;
// New builds one, and Default is the process-wide instance.
type Monitor struct {
	silence atomic.Int64 // nanoseconds, see SetSilence
	mu      sync.Mutex
	watches map[*Watch]struct{}
	running bool
}

// Default is the monitor the product's loops register with.
var Default = New(StallSilence)

// New returns a monitor that reports a loop after silence without movement.
// A silence of zero or less is StallSilence.
func New(silence time.Duration) *Monitor {
	m := &Monitor{watches: map[*Watch]struct{}{}}
	m.SetSilence(silence)
	return m
}

// SetSilence changes how long a loop may go without moving before it is
// reported. It exists for tests, which cannot wait out five minutes; nothing in
// the product writes it, because the silence is not a setting (see the package
// comment). What a test shortens is the threshold, never the rule: a loop that
// keeps moving is still never reported, whatever its total runtime.
func (m *Monitor) SetSilence(silence time.Duration) {
	if silence <= 0 {
		silence = StallSilence
	}
	m.silence.Store(int64(silence))
}

// Start registers a watch on the default monitor. A loop that is ours calls
// this before its first record and Done when it returns.
func Start(desc Desc) *Watch { return Default.Start(desc) }

// Start registers a watch on this monitor and starts the supervisor if it is
// not already running.
func (m *Monitor) Start(desc Desc) *Watch {
	w := m.watch(desc)
	m.mu.Lock()
	start := !m.running
	m.running = true
	m.mu.Unlock()
	if start {
		go m.supervise()
	}
	return w
}

// watch registers a watch and starts nothing. It is the seam the mechanism
// tests use: they drive check with a clock of their own, and a supervisor
// reading the same watch against the wall clock would put two timelines in one
// watch.
func (m *Monitor) watch(desc Desc) *Watch {
	w := &Watch{monitor: m, desc: desc}
	m.mu.Lock()
	m.watches[w] = struct{}{}
	m.mu.Unlock()
	return w
}

func (m *Monitor) remove(w *Watch) {
	m.mu.Lock()
	delete(m.watches, w)
	m.mu.Unlock()
}

// supervise is the one goroutine: it looks at every registered watch on a fixed
// poll and writes the reports. It exits when the last watch is done, so a
// process that never runs one of these loops never runs this either.
func (m *Monitor) supervise() {
	ticker := time.NewTicker(stallPoll)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.Lock()
		empty := len(m.watches) == 0
		if empty {
			m.running = false
		}
		m.mu.Unlock()
		if empty {
			return
		}
		for _, s := range m.check(time.Now()) {
			s.report()
		}
	}
}

// check returns the watches that have been silent for this monitor's silence,
// and re-arms the ones that moved. The time enters here as an argument and is
// read nowhere else, so a test can hand it a clock of its own instead of
// waiting one out.
func (m *Monitor) check(now time.Time) []stall {
	m.mu.Lock()
	defer m.mu.Unlock()
	silence := time.Duration(m.silence.Load())
	var out []stall
	for w := range m.watches {
		record := w.record.Load()
		if w.at.IsZero() || record != w.seen {
			w.seen, w.at, w.reported = record, now, false
			continue
		}
		silent := now.Sub(w.at)
		if w.reported || silent < silence {
			continue
		}
		w.reported = true
		out = append(out, stall{desc: w.desc, record: record, silent: silent})
	}
	return out
}

// A stall is one loop's silence, ready to be written.
type stall struct {
	desc   Desc
	record int64
	silent time.Duration
}

// report writes the stall as one diagnostic. It names the dictionary, the file
// and the record, and never the clock: silent_ms is how long the loop has been
// still - the observation itself - and not a bound anything was measured
// against. The line is written whether or not the reader turned diagnostics on,
// because a stall is not a measurement (the lines next door are) and a silent
// hang is the thing this exists to end.
func (s stall) report() {
	logx.SystemDiagnostic("loop_stall surface=%q name=%q path=%q size_bytes=%d record=%d silent_ms=%d",
		s.desc.Surface, s.desc.Name, s.desc.Path, s.desc.Size, s.record, s.silent.Milliseconds())
}
