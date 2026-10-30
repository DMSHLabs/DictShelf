// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

func TestIndexLogDownload(t *testing.T) {
	s := newTestServer(t)
	s.Version = "wudict2-test-build"
	store.IndexDiagnostic("failure source=%q error=%q", "broken.dsl", "disk full")
	logx.Warn("resource unreadable: permission denied")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, newRequest("GET", "/api/system-log", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"Build: DictShelf-test-build", "broken.dsl", "disk full", "resource unreadable", "System Log"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("log must download without caching")
	}
	if !regexp.MustCompile(`filename="DictShelf_SystemLog_\d{4}_\d{2}_\d{2}-\d{2}-\d{2}\.log"`).MatchString(w.Header().Get("Content-Disposition")) {
		t.Fatal("missing dated filename")
	}
}

func TestBuildIdentityUsesProductName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "wudict2-v1.01.0-29-gc29ef6e6-dirty", want: "DictShelf-v1.01.0-29-gc29ef6e6-dirty"},
		{input: "v1.2.3", want: "DictShelf-v1.2.3"},
		{input: "DictShelf-v1.2.3", want: "DictShelf-v1.2.3"},
		{input: "", want: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, _, _ := buildIdentity(test.input)
			if got != test.want {
				t.Fatalf("buildIdentity(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
