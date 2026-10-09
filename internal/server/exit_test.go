// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopExitOnlyFromLocalPage(t *testing.T) {
	s := New(nil)
	called := make(chan struct{}, 1)
	s.DesktopExit = func() { called <- struct{}{} }
	for _, tc := range []struct {
		name, host, remote, origin string
		want                       int
	}{
		{"local page", "127.0.0.1:6888", "127.0.0.1:1234", "http://127.0.0.1:6888", http.StatusAccepted},
		{"remote browser", "127.0.0.1:6888", "192.0.2.7:1234", "http://127.0.0.1:6888", http.StatusForbidden},
		{"rebound host", "evil.example:6888", "127.0.0.1:1234", "http://evil.example:6888", http.StatusMisdirectedRequest},
		{"foreign origin", "127.0.0.1:6888", "127.0.0.1:1234", "https://evil.example", http.StatusForbidden},
		{"missing origin", "127.0.0.1:6888", "127.0.0.1:1234", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/exit", nil)
			r.RemoteAddr = tc.remote
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want == http.StatusAccepted {
				<-called
			} else {
				select {
				case <-called:
					t.Fatal("rejected request stopped the server")
				default:
				}
			}
		})
	}
}

func TestDesktopExitUnavailableWithoutOwner(t *testing.T) {
	s := New(nil)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6888/api/exit", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Origin", "http://127.0.0.1:6888")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}
