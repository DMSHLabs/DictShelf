// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDesktopReindexStatusWithoutRouter(t *testing.T) {
	s := newTestServer(t)
	release := make(chan struct{})
	defer close(release)
	if _, started := s.jobs.start(reindexKey, 0, jobStatus{
		Done: 1, Total: 3, Current: "sample.dsl", CurrentDone: 5, CurrentTotal: 10,
	}, func(*job) { <-release }); !started {
		t.Fatal("could not start test reindex job")
	}

	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, newRequest("GET", "/api/reindex", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status %d: %s", recorder.Code, recorder.Body.String())
	}
	var want ReindexStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &want); err != nil {
		t.Fatal(err)
	}

	mux := s.mux
	s.mux = http.NewServeMux()
	defer func() { s.mux = mux }()
	gotValue, err := s.DesktopJSON("", "reindex", "")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gotValue.(ReindexStatus)
	if !ok {
		t.Fatalf("native result has type %T, want ReindexStatus", gotValue)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP and native reindex status differ:\nHTTP: %+v\nnative: %+v", want, got)
	}
}

func TestDesktopReindexStartAndStopWithoutRouter(t *testing.T) {
	s := newTestServer(t)
	id := getDicts(t, s, "/api/dicts")[0].ID
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	sse(t, s, "/api/ingest?dict="+id)
	bumpReader(t, "dsl")

	mux := s.mux
	s.mux = http.NewServeMux()
	defer func() { s.mux = mux }()

	result, err := s.DesktopJSON("", "startReindex", "")
	if err != nil {
		t.Fatal(err)
	}
	started, ok := result.(ReindexStatus)
	if !ok || started.Total != 1 || (!started.Running && started.Done != 1) {
		t.Fatalf("unexpected direct start result: %#v", result)
	}
	s.jobs.wg.Wait()
	if entryOutdated(e.Path) {
		t.Fatal("direct reindex did not update the dictionary")
	}

	result, err = s.DesktopJSON("", "stopReindex", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.(ReindexStatus); !ok {
		t.Fatalf("unexpected direct stop result type: %T", result)
	}
}

func TestReindexStartAdmissionIsSharedWithNativeBridge(t *testing.T) {
	s := newTestServer(t)
	release := make(chan struct{})
	defer func() { close(release); s.jobs.wg.Wait() }()
	if _, started := s.jobs.start(bulkIndexKey, 0, jobStatus{}, func(*job) { <-release }); !started {
		t.Fatal("could not start blocking index job")
	}

	if _, err := s.DesktopJSON("", "startReindex", ""); err == nil {
		t.Fatal("native reindex start bypassed work admission")
	}
	code, _ := reindexCall(t, s, "POST")
	if code != http.StatusConflict {
		t.Fatalf("HTTP reindex start returned %d, want 409", code)
	}
}
