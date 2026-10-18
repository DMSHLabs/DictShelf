// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/logx"
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
	Stage                     string
	// Failed names each item that could not be done, with why.
	Failed   []string
	Damaged  []string
	Err      string // the job as a whole failed
	Canceled bool
	// StopRequested is a soft stop: finish the current atomic dictionary
	// operation, then leave the remaining queue untouched.
	StopRequested  bool
	Action         string
	Indexes        []string
	ActiveDicts    []string
	ActiveProgress []dictionaryWorkProgress
	QueuedDicts    []string
	Result         any // what a successful job produced, for its watchers
}

type dictionaryWorkProgress struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Feature       string `json:"feature,omitempty"`
	Action        string `json:"action,omitempty"`
	Active        bool   `json:"active"`
	Done          int64  `json:"done"`
	Total         int64  `json:"total"`
	StopRequested bool   `json:"stopRequested,omitempty"`
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
	changed           chan struct{}
	drop              bool // forget the job when it ends (dropWhenDone)
	dictCancels       map[string]context.CancelFunc
	dictStopRequested map[string]bool
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
		defer func() {
			if p := recover(); p != nil {
				logx.System("job=%q panic=%v\n%s", key, p, debug.Stack())
				t.mu.Lock()
				if j.st.Err == "" {
					j.st.Err = fmt.Sprintf("background task failed: %v", p)
				}
				t.mu.Unlock()
			}
		}()
		fn(j)
	}()
	return j.st.copy(), true
}

func (j *job) end(key string) {
	j.cancel()
	j.t.mu.Lock()
	j.st.Running = false
	j.signalLocked()
	errText := j.st.Err
	failures := append([]string(nil), j.st.Failed...)
	if j.drop && j.t.jobs[key] == j {
		delete(j.t.jobs, key)
	}
	j.t.mu.Unlock()
	if errText != "" {
		logx.System("job=%q error=%q", key, errText)
	}
	for _, failure := range failures {
		logx.System("job=%q failure=%q", key, failure)
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

// beginDictionary gives one dictionary operation a child context so it can be
// stopped without canceling the other dictionaries in the same queue.
func (j *job) beginDictionary(id string) (context.Context, context.CancelFunc, bool) {
	j.t.mu.Lock()
	defer j.t.mu.Unlock()
	if j.ctx.Err() != nil || j.dictStopRequested[id] {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(j.ctx)
	if j.dictCancels == nil {
		j.dictCancels = make(map[string]context.CancelFunc)
	}
	j.dictCancels[id] = cancel
	return ctx, cancel, true
}

func (j *job) finishDictionary(id string, cancel context.CancelFunc) {
	cancel()
	j.t.mu.Lock()
	delete(j.dictCancels, id)
	if j.dictStopRequested[id] {
		delete(j.dictStopRequested, id)
	}
	for i := range j.st.ActiveProgress {
		if j.st.ActiveProgress[i].ID == id {
			j.st.ActiveProgress[i].Active = false
			j.st.ActiveProgress[i].StopRequested = false
			break
		}
	}
	j.signalLocked()
	j.t.mu.Unlock()
}

func (j *job) cancelDictionary(id string) bool {
	j.t.mu.Lock()
	defer j.t.mu.Unlock()
	cancel := j.dictCancels[id]
	if !j.st.Running || cancel == nil || j.dictStopRequested[id] {
		return false
	}
	if j.dictStopRequested == nil {
		j.dictStopRequested = make(map[string]bool)
	}
	j.dictStopRequested[id] = true
	for i := range j.st.ActiveProgress {
		if j.st.ActiveProgress[i].ID == id {
			j.st.ActiveProgress[i].StopRequested = true
			break
		}
	}
	cancel()
	j.signalLocked()
	return true
}

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

func (t *jobTable) wait(key string) jobStatus {
	for {
		st, changed, ok := t.watch(key)
		if !ok || !st.Running {
			return st
		}
		<-changed
	}
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

func (t *jobTable) requestStop(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[key]; ok && j.st.Running && !j.st.StopRequested {
		j.st.StopRequested = true
		j.signalLocked()
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
	if st.Failed != nil {
		st.Failed = append([]string{}, st.Failed...)
	}
	if st.Damaged != nil {
		st.Damaged = append([]string{}, st.Damaged...)
	}
	st.Indexes = append([]string(nil), st.Indexes...)
	st.ActiveDicts = append([]string(nil), st.ActiveDicts...)
	st.ActiveProgress = append([]dictionaryWorkProgress(nil), st.ActiveProgress...)
	st.QueuedDicts = append([]string(nil), st.QueuedDicts...)
	return st
}

func trackJobDictionary(j *job, id string, active bool) {
	j.update(func(st *jobStatus) {
		if active {
			for _, existing := range st.ActiveDicts {
				if existing == id {
					return
				}
			}
			st.ActiveDicts = append(st.ActiveDicts, id)
			found := false
			for i := range st.ActiveProgress {
				if st.ActiveProgress[i].ID == id {
					st.ActiveProgress[i].Active = true
					found = true
					break
				}
			}
			if !found {
				st.ActiveProgress = append(st.ActiveProgress, dictionaryWorkProgress{ID: id, Active: true})
			}
			return
		}
		for i, existing := range st.ActiveDicts {
			if existing == id {
				st.ActiveDicts = append(st.ActiveDicts[:i], st.ActiveDicts[i+1:]...)
				for p := range st.ActiveProgress {
					if st.ActiveProgress[p].ID == id {
						st.ActiveProgress[p].Active = false
						break
					}
				}
				return
			}
		}
	})
}

func updateJobDictionaryProgress(j *job, id, name, feature, action string, done, total int64) {
	j.update(func(st *jobStatus) {
		// The compact status line has one current dictionary even when several
		// workers run. Keep its name and counters in the same locked update so
		// progress from another worker cannot be paired with a stale name.
		st.Stage, st.Current, st.CurrentDone, st.CurrentTotal, st.Action = id, name, done, total, action
		if feature != "" {
			st.Indexes = []string{feature}
		}
		for i := range st.ActiveProgress {
			if st.ActiveProgress[i].ID == id {
				st.ActiveProgress[i].Name = name
				st.ActiveProgress[i].Feature = feature
				st.ActiveProgress[i].Action = action
				st.ActiveProgress[i].Done = done
				st.ActiveProgress[i].Total = total
				return
			}
		}
	})
}
