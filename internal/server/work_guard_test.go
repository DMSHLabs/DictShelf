// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkAdmissionIndividualAndExclusive(t *testing.T) {
	s := New(nil)
	done := make(chan struct{})
	s.jobs.start("ingest:alpha", 0, jobStatus{}, func(j *job) { <-done })
	defer func() { close(done); s.jobs.wg.Wait() }()
	for _, test := range []struct {
		method, path, body string
		allowed            bool
	}{
		{"GET", "/api/ingest?dict=beta&fts=1", "", true},
		{"GET", "/api/ingest?dict=alpha&fts=1", "", false},
		{"POST", "/api/index-work", `[{"dict":"beta","feature":"base","action":"create"}]`, false},
		{"GET", "/api/setup?path=test&save=1", "", false},
		{"POST", "/api/rescan", "{}", false},
		{"DELETE", "/api/library?dict=beta", "", false},
		{"GET", "/api/setup?path=test", "", true},
		{"DELETE", "/api/index-work?id=all", "", true},
		{"GET", "/api/rescan?status=1", "", true},
		{"POST", "/api/index-work?single=1", `[{"dict":"beta","feature":"base","action":"delete"}]`, true},
	} {
		reached := false
		handler := s.withWorkAdmission(func(w http.ResponseWriter, r *http.Request) { reached = true })
		response := httptest.NewRecorder()
		handler(response, newRequest(test.method, test.path, strings.NewReader(test.body)))
		if reached != test.allowed {
			t.Errorf("%s %s: allowed=%v code=%d", test.method, test.path, reached, response.Code)
		}
	}
}

func TestWorkAdmissionReservationClosesStartRace(t *testing.T) {
	s := New(nil)
	entered, release := make(chan struct{}), make(chan struct{})
	handler := s.withWorkAdmission(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release })
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		handler(httptest.NewRecorder(), newRequest("GET", "/api/setup?path=test&save=1", nil))
	}()
	<-entered
	reached := false
	s.withWorkAdmission(func(w http.ResponseWriter, r *http.Request) { reached = true })(httptest.NewRecorder(), newRequest("GET", "/api/ingest?dict=beta&fts=1", nil))
	close(release)
	<-ended
	if reached {
		t.Fatal("individual request entered before exclusive job registration")
	}
}
