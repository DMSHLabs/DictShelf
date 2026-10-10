// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import "testing"

func TestDesktopWindowURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://0.0.0.0:6888/?k=abc", "http://127.0.0.1:6888/?k=abc"},
		{"http://192.168.1.8:9090/", "http://127.0.0.1:9090/"},
		{"https://example.com/", ""},
		{"http://user:secret@localhost:6888/", ""},
		{"http://localhost/", ""},
	}
	for _, tc := range tests {
		got, err := desktopWindowURL(tc.in)
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Errorf("desktopWindowURL(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}
