// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package watchdog

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// The mechanism tests below register through watch, not Start, and drive check
// with a clock of their own: a supervisor reading the same watch against the
// wall clock would put two timelines in one watch. The supervisor itself is
// exercised end to end by the store's own tests, which wait for the line to
// reach the system log.

// fakeReader is a loop that is ours with a reader behind it that stops: it
// emits its records and then blocks forever, the way a corrupt file or a wedged
// read does. It is the shape every watched loop has - a record per iteration,
// and the read underneath it.
type fakeReader struct {
	watch    *Watch
	records  int
	blocked  chan struct{}
	finished chan struct{}
	done     chan struct{}
}

func (r *fakeReader) run() {
	defer close(r.done)
	for i := 1; i <= r.records; i++ {
		r.watch.Moved(int64(i))
	}
	close(r.finished)
	<-r.blocked
}

func newFakeReader(w *Watch, records int) *fakeReader {
	return &fakeReader{watch: w, records: records, blocked: make(chan struct{}), finished: make(chan struct{}), done: make(chan struct{})}
}

// captureLog points the system log at a temporary directory and returns what
// has been written to it. The directory is resolved once, before the resolver
// is installed: a resolver that called t.TempDir() would panic the day a report
// arrives after the test has finished.
func captureLog(t *testing.T) func() string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "system")
	logx.SetSystemLogRoot(func() string { return dir })
	return func() string {
		t.Helper()
		b, err := logx.SystemSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
}

// A loop that stops moving is reported, and the report names the dictionary, the
// file, its size and the record it stopped on. The clock is the test's own - the
// silence is compared against the times handed to check - so nothing here waits
// for a wall clock, and the diagnostic is asserted rather than an elapsed time.
func TestAStalledLoopIsReported(t *testing.T) {
	log := captureLog(t)
	m := New(5 * time.Minute)
	w := m.watch(Desc{Surface: "article-ids", Name: "Викисловарь", Path: "/dicts/wiktionary.zim", Size: 2_147_483_648})
	reader := newFakeReader(w, 3)
	go reader.run()
	<-reader.finished

	now := time.Now()
	if stalls := m.check(now); len(stalls) != 0 {
		t.Fatalf("the first look reported a watch it had not seen move: %+v", stalls)
	}
	if stalls := m.check(now.Add(4 * time.Minute)); len(stalls) != 0 {
		t.Fatalf("a loop reported after less than the silence: %+v", stalls)
	}
	stalls := m.check(now.Add(6 * time.Minute))
	if len(stalls) != 1 {
		t.Fatalf("a loop that stopped moving for six minutes was not reported: %+v", stalls)
	}
	s := stalls[0]
	if s.desc.Surface != "article-ids" || s.desc.Name != "Викисловарь" || s.desc.Path != "/dicts/wiktionary.zim" || s.desc.Size != 2_147_483_648 {
		t.Errorf("the report does not name the loop and the dictionary: %+v", s.desc)
	}
	if s.record != 3 {
		t.Errorf("record = %d, want the last one the loop reached (3)", s.record)
	}
	s.report()
	line := log()
	for _, want := range []string{"loop_stall", `surface="article-ids"`, "Викисловарь", "/dicts/wiktionary.zim", "size_bytes=2147483648", "record=3"} {
		if !strings.Contains(line, want) {
			t.Errorf("the diagnostic does not carry %q:\n%s", want, line)
		}
	}
	close(reader.blocked)
	<-reader.done
}

// A loop that keeps moving is not reported, however long it takes: the records
// arrive with long pauses between them, and the total runtime is many times the
// silence. This is the test a deadline would fail, which is the point of it.
func TestASlowLoopIsNotReported(t *testing.T) {
	m := New(30 * time.Second)
	w := m.watch(Desc{Surface: "browse-headwords", Name: "Slow", Path: "/dicts/slow.text.db", Size: 4096})
	now := time.Now()
	for i := int64(1); i <= 100; i++ {
		w.Moved(i)
		if stalls := m.check(now); len(stalls) != 0 {
			t.Fatalf("record %d was reported while the loop was moving: %+v", i, stalls)
		}
		now = now.Add(29 * time.Second) // a long pause, and still less than the silence
	}
	// 100 records at 29s each: 48 minutes of silence-free work under a 30s
	// silence, and not one report.
	if stalls := m.check(now); len(stalls) != 0 {
		t.Fatalf("a loop that never stopped moving was reported: %+v", stalls)
	}
	w.Done()
}

// A loop that is finished is not a loop that has stopped: nothing is reported
// for a watch that has been taken off, even if it is never looked at again.
func TestAFinishedLoopIsNotReported(t *testing.T) {
	m := New(time.Second)
	w := m.watch(Desc{Surface: "entry-scan", Name: "Done", Path: "/dicts/done.text.db"})
	w.Moved(1)
	now := time.Now()
	m.check(now)
	w.Done()
	if stalls := m.check(now.Add(time.Hour)); len(stalls) != 0 {
		t.Fatalf("a finished loop was reported: %+v", stalls)
	}
}

// A watch reports a stall once, and reports again only if the loop moved and
// stopped again: a hang is named, and a named hang does not fill the log.
func TestAStallIsReportedOncePerStop(t *testing.T) {
	m := New(time.Minute)
	w := m.watch(Desc{Surface: "headword-export", Name: "Twice", Path: "/dicts/twice.text.db"})
	w.Moved(1)
	now := time.Now()
	m.check(now)
	if stalls := m.check(now.Add(2 * time.Minute)); len(stalls) != 1 {
		t.Fatalf("the stall was not reported: %+v", stalls)
	}
	if stalls := m.check(now.Add(3 * time.Minute)); len(stalls) != 0 {
		t.Fatalf("the same stall was reported twice: %+v", stalls)
	}
	// It moves again, and stops again: that is a second stall, and a second
	// report.
	w.Moved(2)
	m.check(now.Add(4 * time.Minute))
	if stalls := m.check(now.Add(5 * time.Minute)); len(stalls) != 1 {
		t.Fatalf("a second stall was not reported: %+v", stalls)
	}
}
