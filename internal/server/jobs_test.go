// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"testing"
	"time"
)

// startBlocked starts a job that runs until release is closed.
func startBlocked(t *testing.T, jt *jobTable, key string) (release chan struct{}, started bool) {
	t.Helper()
	release = make(chan struct{})
	_, started = jt.start(key, 0, jobStatus{Total: 2}, func(j *job) {
		select {
		case <-release:
		case <-j.ctx.Done():
		}
	})
	return release, started
}

func waitEnded(t *testing.T, jt *jobTable, key string) jobStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, _ := jt.status(key); !st.Running {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not end", key)
	return jobStatus{}
}

// The table's rules, without work behind them.
func TestJobTableRules(t *testing.T) {
	var jt jobTable
	t.Cleanup(jt.wg.Wait)

	rel, ok := startBlocked(t, &jt, "a")
	if !ok {
		t.Fatal("did not start when idle")
	}
	if _, again := startBlocked(t, &jt, "a"); again {
		t.Error("two jobs under one key")
	}
	if _, other := startBlocked(t, &jt, "b"); !other {
		t.Error("another key is not blocked by this one")
	}
	jt.cancel("b")
	if st := waitEnded(t, &jt, "b"); st.Running {
		t.Error("cancel did not end the job")
	}
	close(rel)
	waitEnded(t, &jt, "a")
	if _, restarted := startBlocked(t, &jt, "a"); !restarted {
		t.Error("an ended job is not replaced by a new one")
	}
	if st, _ := jt.status("a"); !st.Running {
		t.Error("the new job does not run")
	}
	jt.cancel("a")
	waitEnded(t, &jt, "a")

	jt.cancel("nothing") // no job: nothing happens
	if _, ok := jt.status("nothing"); ok {
		t.Error("cancel invented a job")
	}
}

// A new job does not inherit an old one's cancel, and status hands out copies.
func TestJobTableFreshStateAndCopies(t *testing.T) {
	var jt jobTable
	t.Cleanup(jt.wg.Wait)
	rel, _ := startBlocked(t, &jt, "a")
	jt.cancel("a")
	waitEnded(t, &jt, "a")
	close(rel)

	done := make(chan bool, 1)
	jt.start("a", 0, jobStatus{}, func(j *job) {
		j.update(func(st *jobStatus) { st.Failed = append(st.Failed, "x: y") })
		done <- j.canceled()
	})
	if <-done {
		t.Error("the new job starts canceled")
	}
	st := waitEnded(t, &jt, "a")
	st.Failed[0] = "changed"
	if again, _ := jt.status("a"); again.Failed[0] != "x: y" {
		t.Error("status shares Failed with the job")
	}
}

// A watcher is woken by an update; a job that drops itself is gone when it
// ends; a timeout cancels.
func TestJobTableWatchDropTimeout(t *testing.T) {
	var jt jobTable
	t.Cleanup(jt.wg.Wait)
	step := make(chan struct{})
	jt.start("w", 0, jobStatus{}, func(j *job) {
		<-step
		j.update(func(st *jobStatus) { st.Done = 1 })
		j.dropWhenDone()
		<-step
	})
	_, next, _ := jt.watch("w")
	step <- struct{}{}
	select {
	case <-next:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher was not woken")
	}
	if st, _, _ := jt.watch("w"); st.Done != 1 {
		t.Errorf("Done = %d", st.Done)
	}
	step <- struct{}{}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := jt.status("w"); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a dropped job is still listed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	jt.start("t", 10*time.Millisecond, jobStatus{}, func(j *job) { <-j.ctx.Done() })
	waitEnded(t, &jt, "t")
}
