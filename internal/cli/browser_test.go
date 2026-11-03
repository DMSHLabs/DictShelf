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
		// A typed AUTH_TOKEN survives the trip through the query string the
		// server decodes: '+' would arrive as a space, '&' would split the
		// query, '#' would cut it and '%' would start an escape.
		{"key: escaped into the query", "http://127.0.0.1:6888/", "a+b&c#d e%f", "http://127.0.0.1:6888/?k=a%2Bb%26c%23d+e%25f"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := browseURL(tc.url, tc.token); got != tc.want {
				t.Errorf("browseURL(%q, %q) = %q, want %q", tc.url, tc.token, got, tc.want)
			}
		})
	}
}

// browserURL is the address the browser is actually sent to: the address the
// server listens on, with every spelling of the wildcard read as loopback,
// because "http://0.0.0.0:6888/" is an address no browser can connect to.
// A concrete non-loopback bind is left alone - that IS where it listens.
func TestBrowserURLSpellsTheWildcardAsLoopback(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		token string
		want  string
	}{
		{"loopback: as it is", "127.0.0.1:6888", "", "http://127.0.0.1:6888/"},
		{"loopback, keyed", "127.0.0.1:6888", "abc_-9", "http://127.0.0.1:6888/?k=abc_-9"},
		{"ipv4 wildcard", "0.0.0.0:6888", "abc", "http://127.0.0.1:6888/?k=abc"},
		{"ipv6 wildcard", "[::]:6888", "abc", "http://127.0.0.1:6888/?k=abc"},
		{"an empty host is the same wildcard", ":6888", "", "http://127.0.0.1:6888/"},
		{"ipv6 loopback is not a wildcard", "[::1]:6888", "", "http://[::1]:6888/"},
		{"a host name: as it is", "localhost:6888", "", "http://localhost:6888/"},
		{"a LAN address is left alone", "192.168.1.5:6888", "abc", "http://192.168.1.5:6888/?k=abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := browserURL(tc.addr, tc.token); got != tc.want {
				t.Errorf("browserURL(%q, %q) = %q, want %q", tc.addr, tc.token, got, tc.want)
			}
		})
	}
}
