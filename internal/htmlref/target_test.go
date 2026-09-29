// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package htmlref

import "testing"

func TestEncTarget(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"run", "run"},
		{"long run", "long run"},
		{"C#", "C%23"},
		{"100%", "100%25"},
		{"%41", "%2541"},
		{"@home", "%40home"},
		{"a@b", "a@b"},
		{"a\tb\x7f", "a%09b%7F"},
		{"вода́ & 水?", "вода́ & 水?"},
		{"#@%", "%23@%25"},
	}
	for _, tt := range tests {
		if got := EncTarget(tt.in); got != tt.want {
			t.Errorf("EncTarget(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := EntryHref("C#"); got != "entry://C%23" {
		t.Errorf("EntryHref = %q", got)
	}
}
