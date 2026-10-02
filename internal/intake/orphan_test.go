// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// hangingSite answers HEAD at once and holds every GET open until the test
// ends: a download that is always "in progress".
func hangingSite(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "1000")
		if r.Method == http.MethodHead {
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return srv
}

func withOrphanAfter(t *testing.T, d time.Duration) {
	t.Helper()
	prev := orphanAfter
	orphanAfter = d
	t.Cleanup(func() { orphanAfter = prev })
}

func waitState(t *testing.T, m *Manager, want string) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		var st Job
		if m.cur != nil {
			st = m.cur.pub.copy() // read without touching seen
		}
		m.mu.Unlock()
		if st.State == want {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job never reached %q", want)
	return Job{}
}

// The defect: a download whose watcher died - an app process killed while the
// server it started lives on - held the one import slot, and every link opened
// afterwards was refused as "an import is already running".
func TestAbandonedDownloadIsReplaced(t *testing.T) {
	srv := hangingSite(t)
	dest := t.TempDir()
	m := &Manager{}
	defer func() { m.Cancel(); m.Wait() }()

	withOrphanAfter(t, time.Hour)
	first, err := m.BeginURL(dest, srv.URL+"/a.slob", loopback())
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, m, StateDownloading)
	// Watched (or just started): refused, as before.
	if _, err := m.BeginURL(dest, srv.URL+"/b.slob", loopback()); !errors.Is(err, ErrBusy) {
		t.Fatalf("second import while the first is watched: err = %v, want ErrBusy", err)
	}

	// Nobody has asked about it for longer than the grace: replaced.
	withOrphanAfter(t, 0)
	second, err := m.BeginURL(dest, srv.URL+"/b.slob", loopback())
	if err != nil {
		t.Fatalf("replacing an abandoned download: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("the abandoned job was not replaced")
	}
	// A status poll is what keeps a job from being taken for abandoned.
	withOrphanAfter(t, time.Second)
	waitState(t, m, StateDownloading)
	m.Status()
	if _, err := m.BeginURL(dest, srv.URL+"/c.slob", loopback()); !errors.Is(err, ErrBusy) {
		t.Fatalf("a polled download was replaced: err = %v", err)
	}
}

// A confirmed import is the user's decision: never replaced, however quiet.
func TestConfirmedDownloadIsNotReplaced(t *testing.T) {
	srv := hangingSite(t)
	dest := t.TempDir()
	m := &Manager{}
	defer func() { m.Cancel(); m.Wait() }()
	withOrphanAfter(t, 0)
	// Pasted links are a collection: ready without downloading, then the
	// confirmation starts the (hanging) download.
	if _, err := m.BeginURL(dest, srv.URL+"/a.slob "+srv.URL+"/b.slob", loopback()); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if st := m.Status(); st.State != StateReady {
		t.Fatalf("state = %q (%s)", st.State, st.Error)
	}
	if _, err := m.Confirm(dest, []int{0}, Options{Keep: true}); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, StateDownloading)
	if _, err := m.BeginURL(dest, srv.URL+"/c.slob", loopback()); !errors.Is(err, ErrBusy) {
		t.Fatalf("a confirmed download was replaced: err = %v", err)
	}
}

func TestCancelIDCancelsOnlyItsOwnJob(t *testing.T) {
	srv := hangingSite(t)
	dest := t.TempDir()
	m := &Manager{}
	defer func() { m.Cancel(); m.Wait() }()
	withOrphanAfter(t, 0)
	first, err := m.BeginURL(dest, srv.URL+"/a.slob", loopback())
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.BeginURL(dest, srv.URL+"/b.slob", loopback())
	if err != nil {
		t.Fatal(err)
	}
	if m.CancelID(first.ID) {
		t.Fatal("a stale id cancelled the job that replaced it")
	}
	if st := m.Status(); st.ID != second.ID {
		t.Fatalf("current job = %q, want %q", st.ID, second.ID)
	}
	if !m.CancelID(second.ID) {
		t.Fatal("the job's own id did not cancel it")
	}
	if st := m.Status(); st.ID != "" {
		t.Fatalf("job still current: %+v", st)
	}
}
