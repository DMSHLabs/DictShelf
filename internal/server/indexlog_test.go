// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
)

func TestIndexLogDownload(t *testing.T) {
	s := newTestServer(t)
	s.Version = "test-build"
	store.IndexDiagnostic("failure source=%q error=%q", "broken.dsl", "disk full")
	logx.Warn("resource unreadable: permission denied")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, newRequest("GET", "/api/system-log", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"test-build", "broken.dsl", "disk full", "resource unreadable", "System Log"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("log must download without caching")
	}
	if !regexp.MustCompile(`filename="wuDict2_SystemLog_\d{4}_\d{2}_\d{2}-\d{2}-\d{2}\.log"`).MatchString(w.Header().Get("Content-Disposition")) {
		t.Fatal("missing dated filename")
	}
}
