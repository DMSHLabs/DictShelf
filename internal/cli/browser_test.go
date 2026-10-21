// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"net/url"
	"testing"
)

// The link handed to the platform opener is never empty - on Windows an empty
// target opens Explorer - never a wildcard address no browser can reach, and
// carries the key exactly when one is required.
func TestBrowserURL(t *testing.T) {
	for _, tc := range []struct{ addr, token, want string }{
		{"127.0.0.1:6888", "", "http://127.0.0.1:6888/"}, // the default install
		{"127.0.0.1:6888", "abc_-9", "http://127.0.0.1:6888/?k=abc_-9"},
		{"0.0.0.0:6888", "abc", "http://127.0.0.1:6888/?k=abc"},
		{"[::]:6888", "abc", "http://127.0.0.1:6888/?k=abc"},
		{":6888", "", "http://127.0.0.1:6888/"},
		{"[::1]:6888", "", "http://[::1]:6888/"},
		{"localhost:6888", "", "http://localhost:6888/"},
		{"192.168.1.5:6888", "abc", "http://192.168.1.5:6888/?k=abc"},
	} {
		if got := browserURL(tc.addr, tc.token); got != tc.want {
			t.Errorf("browserURL(%q, %q) = %q, want %q", tc.addr, tc.token, got, tc.want)
		}
	}
}

// keyURL is empty without a key (the banner's "no key line" signal), and a
// typed AUTH_TOKEN survives the trip through the query string the server
// decodes - '+', '&', '#', '%' and spaces included.
func TestKeyURL(t *testing.T) {
	if got := keyURL("http://127.0.0.1:6888/", ""); got != "" {
		t.Errorf("keyURL without a token = %q, want empty", got)
	}
	if got := keyURL("http://h/?a=1", "x"); got != "http://h/?a=1&k=x" {
		t.Errorf("keyURL on a query = %q", got)
	}
	for _, tok := range []string{"a+b", "a&k=evil", "a#b", "100%", "two words", "ключ"} {
		u, err := url.Parse(keyURL("http://127.0.0.1:6888/", tok))
		if err != nil {
			t.Fatalf("token %q: %v", tok, err)
		}
		if got := u.Query()["k"]; len(got) != 1 || got[0] != tok || u.Fragment != "" {
			t.Errorf("token %q came back as k=%q fragment=%q", tok, got, u.Fragment)
		}
	}
}
