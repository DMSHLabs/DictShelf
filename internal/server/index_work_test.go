// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIndexWorkContinuesAfterSourceFailure(t *testing.T) {
	s, _ := newPrefsServer(t)
	defer s.jobs.wg.Wait()
	entries := s.reg.all()
	if err := os.Remove(entries[0].Path); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal([]indexOperation{
		{entries[0].ID, "fts", "recreate"},
		{entries[1].ID, "fts", "recreate"},
	})
	if _, err := s.DesktopJSON("http://127.0.0.1:6888", "startIndexWork", string(body)); err != nil {
		t.Fatal(err)
	}
	s.jobs.wait(bulkIndexKey)
	result := s.indexWorkStatus(bulkIndexKey)
	if result.Running || result.Done != 2 || result.Total != 2 || len(result.Failed) != 1 || result.Error == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !s.currentFeatures(entries[1]).FullText {
		t.Fatal("valid dictionary after the failing source was not indexed")
	}
}

func TestIndexWorkSurvivesRequestAndStopsBetweenOperations(t *testing.T) {
	s, _ := newPrefsServer(t)
	waitNewDictionaryIndexing(t, s.reg)
	s.reg.jobs.wg.Wait()
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
	if result.Running || !result.Canceled || result.Done != 0 || result.Error != "" {
		t.Fatalf("completion: %+v", result)
	}
	if s.currentFeatures(first).Contains {
		t.Fatal("stopped operation unexpectedly created its index")
	}
	if s.currentFeatures(entries[1]).FullText {
		t.Fatal("next operation ran after Stop")
	}
}

func TestIndexWorkCompletesWholeQueue(t *testing.T) {
	s, _ := newPrefsServer(t)
	waitNewDictionaryIndexing(t, s.reg)
	s.reg.jobs.wg.Wait()
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

func TestIndexWorkContinuesAfterOperationFailure(t *testing.T) {
	s, _ := newPrefsServer(t)
	waitNewDictionaryIndexing(t, s.reg)
	s.reg.jobs.wg.Wait()
	defer s.jobs.wg.Wait()
	entries := s.reg.all()
	if len(entries) < 2 {
		t.Fatal("fixture needs two dictionaries")
	}
	broken := entries[0]
	broken.Path = "missing-index-work-source.mdx"
	ops := []indexOperation{{broken.ID, "base", "create"}, {entries[1].ID, "fts", "create"}}
	body, _ := json.Marshal(ops)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest("POST", "/api/index-work", strings.NewReader(string(body))))
	if rec.Code != 202 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	s.jobs.wait(bulkIndexKey)
	result := s.indexWorkStatus()
	if result.Running || result.Done != 2 || len(result.Failed) != 1 || !strings.Contains(result.Failed[0], broken.probeName()) || result.Error == "" {
		t.Fatalf("completion: %+v", result)
	}
	if !s.currentFeatures(entries[1]).FullText {
		t.Fatal("operation after failure did not run")
	}
}
