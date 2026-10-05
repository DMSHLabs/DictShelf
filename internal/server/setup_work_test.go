// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestFolderSaveSurvivesPageDeparture(t *testing.T) {
	s, _ := newPrefsServer(t)
	path := s.reg.Dirs()[0]
	s.reg.mu.Lock()
	locked := true
	defer func() {
		if locked {
			s.reg.mu.Unlock()
		}
		s.jobs.wg.Wait()
	}()
	ctx, cancel := context.WithCancel(context.Background())
	request := newRequest("GET", "/api/setup?path="+url.QueryEscape(path)+"&save=1&async=1", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 202 {
		t.Fatalf("start: %d %s", response.Code, response.Body.String())
	}
	cancel()
	if !s.indexWorkStatus(setupWorkKey).Running {
		t.Fatal("save not registered as active work")
	}
	s.reg.mu.Unlock()
	locked = false
	s.jobs.wait(setupWorkKey)
	result := s.indexWorkStatus(setupWorkKey)
	if result.Error != "" || result.Running {
		t.Fatalf("completion: %+v", result)
	}
	if saved, ok := result.Result.(map[string]any); !ok || saved["saved"] != true {
		t.Fatalf("save result: %+v", result.Result)
	}
}
