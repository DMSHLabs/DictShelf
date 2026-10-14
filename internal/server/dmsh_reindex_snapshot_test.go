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
