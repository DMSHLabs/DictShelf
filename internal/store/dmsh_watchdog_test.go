// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/watchdog"
)

// The watchdog's own tests pin the mechanism; these two pin the wiring: a real
// store loop that stops moving is reported with the dictionary and the record,
// and a real store loop that is merely slow is not reported and is not cut
// short. Both shorten the silence rather than wait out five minutes, and neither
// changes what the loop returns - a reported loop is not ended.

// shortenSilence makes the watchdog report after a silence a test can wait for.
func shortenSilence(t *testing.T, silence time.Duration) {
	t.Helper()
	watchdog.Default.SetSilence(silence)
	t.Cleanup(func() { watchdog.Default.SetSilence(watchdog.StallSilence) })
}

// blockingWriter takes allowed writes and then never returns: a loop that emits
// its records and stops moving while it is inside the sink.
type blockingWriter struct {
	allowed int
	writes  int
	reached chan struct{}
	release chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.allowed {
		close(w.reached)
	}
	if w.writes > w.allowed {
		<-w.release
	}
	return len(p), nil
}

// pacedWriter takes its time over every write and finishes: a slow loop, which
// is not a stalled one.
type pacedWriter struct {
	pause  time.Duration
	writes int
}

func (w *pacedWriter) Write(p []byte) (int, error) {
	time.Sleep(w.pause)
	w.writes++
	return len(p), nil
}

// waitForLogLine waits for a diagnostic to reach the system log. The watchdog
// writes from a goroutine of its own, on a poll of its own, so a test that read
// the log once would be racing it.
func waitForLogLine(t *testing.T, token string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		b, err := IndexLog()
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, token) {
				return line
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %q line reached the system log within %s:\n%s", token, within, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func logLineContaining(t *testing.T, token string) string {
	t.Helper()
	b, err := IndexLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, token) {
			return line
		}
	}
	return ""
}

// An export that stops moving is reported, naming the dictionary, the prepared
// database and the headword it stopped on. The loop is blocked inside the sink,
// so what is asserted is the report's content and never an elapsed time.
func TestAStalledExportIsReported(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	shortenSilence(t, 300*time.Millisecond)
	s := browseStore(t, []string{"apple", "banana", "cherry", "damson", "elder"})
	sink := &blockingWriter{allowed: 3, reached: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- s.ExportHeadwords(context.Background(), sink, func(int64) {}) }()
	t.Cleanup(func() {
		close(sink.release)
		if err := <-done; err != nil {
			t.Errorf("the export did not finish after the sink was released: %v", err)
		}
	})
	<-sink.reached

	line := waitForLogLine(t, "loop_stall", 10*time.Second)
	for _, want := range []string{`surface="headword-export"`, `name="Browse Fixture"`, "browse.text.db", "size_bytes=", "record=3"} {
		if !strings.Contains(line, want) {
			t.Errorf("the report does not carry %q:\n%s", want, line)
		}
	}
}

// A slow export is not reported and loses nothing: the loop takes many times
// the silence over the whole run, and what the watchdog watches is the movement
// between records, not the total. This is the test the removed clocks would
// fail - they would have ended the export and called the row "timed out".
func TestASlowExportIsNotReportedAndNotTruncated(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	shortenSilence(t, 200*time.Millisecond)
	words := make([]string, 40)
	for i := range words {
		words[i] = fmt.Sprintf("word%02d", i)
	}
	s := browseStore(t, words)
	sink := &pacedWriter{pause: 25 * time.Millisecond}
	if err := s.ExportHeadwords(context.Background(), sink, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	if sink.writes != len(words) {
		t.Fatalf("the export wrote %d headwords of %d", sink.writes, len(words))
	}
	// A second of work under a 200ms silence, and the report would have been
	// written by now if the watchdog were a clock.
	time.Sleep(400 * time.Millisecond)
	if line := logLineContaining(t, "loop_stall"); line != "" {
		t.Fatalf("a slow loop was reported: %s", line)
	}
}
