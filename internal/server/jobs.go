// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"sync"
	"time"
)

// The server's long-running work - rebuilding outdated dictionaries, a lemma
// download, a change to one dictionary's indexes - runs as jobs: keyed, at
// most one per key, owned by the server rather than by the request that
// started it. A page that closes, reloads or rotates (Android recreates the
// activity) loses nothing: the work goes on, and the next page finds it by its
// key - in a poll, in the dictionary list, or by watching it again. Every job
// holds the active-power reference while it runs (power.go), so the Android
// shell keeps its foreground service up for all of them alike.
//
// The import (internal/intake) is not one of these: it is a conversation -
// download, show what the archive holds, wait for the user's choice, install
// - and intake.Manager is that conversation's own state machine.

// jobStatus is a job as a poll or a stream reports it. Which fields mean
// something depends on the kind of job; the rest stay zero.
type jobStatus struct {
	Running bool
	// Done of Total: dictionaries for a rebuild, bytes for a download,
	// entries for an index change.
	Done, Total int64
	// Current is the item in hand, with its own progress.
	Current                   string
	CurrentDone, CurrentTotal int64
	// Failed names each item that could not be done, with why.
	Failed   []string
	Err      string // the job as a whole failed
	Canceled bool
	Result   any // what a successful job produced, for its watchers
}

// jobTable is every job, running or last run, by key. The zero value is
// usable.
type jobTable struct {
	mu   sync.Mutex
	jobs map[string]*job
	// wg counts running jobs. Nothing in the server waits on it - a job is
	// meant to outlive its request - but a test must, or it removes a
	// t.TempDir() a job is still writing into.
	wg sync.WaitGroup
}

type job struct {
	t      *jobTable
	st     jobStatus
	ctx    context.Context
	cancel context.CancelFunc
	// changed is closed, and replaced, on every update: what a watcher waits on.
	changed chan struct{}
	drop    bool // forget the job when it ends (dropWhenDone)
}

// start runs fn as the job key, unless one is running under that key: then it
// returns that job's status and false - the caller follows the running one.
// A job that has ended is replaced, which is how a retry clears an old error.
// timeout bounds the job's context; 0 is none.
func (t *jobTable) start(key string, timeout time.Duration, init jobStatus, fn func(j *job)) (jobStatus, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.jobs == nil {
		t.jobs = map[string]*job{}
	}
	if j, ok := t.jobs[key]; ok && j.st.Running {
		return j.st.copy(), false
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	init.Running = true
	j := &job{t: t, st: init, ctx: ctx, cancel: cancel, changed: make(chan struct{})}
	t.jobs[key] = j
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer HoldActiveProcs()()
		defer j.end(key)
		fn(j)
	}()
	return j.st.copy(), true
}

func (j *job) end(key string) {
	j.cancel()
	j.t.mu.Lock()
	defer j.t.mu.Unlock()
	j.st.Running = false
	j.signalLocked()
	if j.drop && j.t.jobs[key] == j {
		delete(j.t.jobs, key)
	}
}

// update changes the job's status and wakes its watchers.
func (j *job) update(f func(*jobStatus)) {
	j.t.mu.Lock()
	defer j.t.mu.Unlock()
	f(&j.st)
	j.signalLocked()
}

func (j *job) signalLocked() {
	close(j.changed)
	j.changed = make(chan struct{})
}

// canceled reports that the job was asked to stop, or ran out of time.
func (j *job) canceled() bool { return j.ctx.Err() != nil }

// dropWhenDone forgets the job once it ends: its outcome is then a fact on
// disk (an installed file), not one a page loaded tomorrow should be told.
func (j *job) dropWhenDone() {
	j.t.mu.Lock()
	j.drop = true
	j.t.mu.Unlock()
}

// status is the job under key, running or last run.
func (t *jobTable) status(key string) (jobStatus, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[key]; ok {
		return j.st.copy(), true
	}
	return jobStatus{}, false
}

// watch is status plus a channel closed at the job's next change.
func (t *jobTable) watch(key string) (jobStatus, <-chan struct{}, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[key]; ok {
		return j.st.copy(), j.changed, true
	}
	return jobStatus{}, nil, false
}

// cancel asks the job under key to stop. A job checks between its items, so
// what is in hand finishes.
func (t *jobTable) cancel(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[key]; ok && j.st.Running {
		j.cancel()
	}
}

// forget removes an ended job under key; a running one stays.
func (t *jobTable) forget(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[key]; ok && !j.st.Running {
		delete(t.jobs, key)
	}
}

func (st jobStatus) copy() jobStatus {
	st.Failed = append([]string(nil), st.Failed...)
	return st
}
