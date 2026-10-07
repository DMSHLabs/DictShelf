// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import "testing"

// browseURL is what keeps an unkeyed launch out of File Explorer: keyURL
// answers "" for "no access key required" - the default configuration - and
// that empty address, handed to the shell, opens This PC instead of a browser.
func TestBrowseURLFallsBackToThePlainURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		token string
		want  string
	}{
		{"no key: the URL is opened as it is", "http://127.0.0.1:6888/", "", "http://127.0.0.1:6888/"},
		{"key: appended to the URL", "http://127.0.0.1:6888/", "s3cret", "http://127.0.0.1:6888/?k=s3cret"},
		{"key: appended after an existing query", "http://127.0.0.1:6888/?q=x", "s3cret", "http://127.0.0.1:6888/?q=x&k=s3cret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := browseURL(tc.url, tc.token); got != tc.want {
				t.Errorf("browseURL(%q, %q) = %q, want %q", tc.url, tc.token, got, tc.want)
			}
		})
	}
}
