// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIndexWorkSurvivesRequestAndStopsBetweenOperations(t *testing.T) {
	s, _ := newPrefsServer(t)
	entries := s.reg.all()
	first := entries[0]
	// Hold the first operation so a simulated page departure and Stop happen
	// while the server owns unfinished work, not after a tiny fixture finishes.
	first.ingestMu.Lock()
	locked := true
	defer func() {
		if locked {
			first.ingestMu.Unlock()
		}
		s.jobs.wg.Wait()
	}()
	ops := []indexOperation{{first.ID, "contains", "create"}, {entries[1].ID, "fts", "create"}}
	body, _ := json.Marshal(ops)
	ctx, cancel := context.WithCancel(context.Background())
	req := newRequest("POST", "/api/index-work", strings.NewReader(string(body))).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for s.indexWorkStatus().Current == "" {
		if time.Now().After(deadline) {
			t.Fatal("job did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if !s.indexWorkStatus().Running {
		t.Fatal("request cancellation stopped the queue")
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest("DELETE", "/api/index-work?id=bulk-indexes", nil))
	if rec.Code != 200 || !s.indexWorkStatus().StopRequested {
		t.Fatalf("stop: %d", rec.Code)
	}
	first.ingestMu.Unlock()
	locked = false
	s.jobs.wait(bulkIndexKey)
	result := s.indexWorkStatus()
	if result.Running || !result.Canceled || result.Done != 1 || result.Error != "" {
		t.Fatalf("completion: %+v", result)
	}
	if !s.currentFeatures(first).Contains {
		t.Fatal("current index was abandoned")
	}
	if s.currentFeatures(entries[1]).FullText {
		t.Fatal("next operation ran after Stop")
	}
}

func TestIndexWorkCompletesWholeQueue(t *testing.T) {
	s, _ := newPrefsServer(t)
	defer s.jobs.wg.Wait()
	ops := []indexOperation{}
	for _, e := range s.reg.all() {
		ops = append(ops, indexOperation{e.ID, "base", "recreate"}, indexOperation{e.ID, "contains", "create"}, indexOperation{e.ID, "fts", "create"})
	}
	body, _ := json.Marshal(ops)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest("POST", "/api/index-work", strings.NewReader(string(body))))
	if rec.Code != 202 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	s.jobs.wait(bulkIndexKey)
	result := s.indexWorkStatus()
	if result.Error != "" || result.Done != int64(len(ops)) {
		t.Fatalf("completion: %+v", result)
	}
	for _, e := range s.reg.all() {
		f := s.currentFeatures(e)
		if !f.Contains || !f.FullText {
			t.Fatalf("missing indexes: %+v", f)
		}
	}
}
