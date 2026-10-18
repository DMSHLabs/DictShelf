// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package fsx

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSafeName(t *testing.T) {
	for _, tc := range []struct {
		in   string
		repl byte
		want string
	}{
		{`a/b\c:d*e?f"g<h>i|j`, '-', "a-b-c-d-e-f-g-h-i-j"},
		{`a/b`, '_', "a_b"},
		{"line\nbreak\x00\x7f", '-', "linebreak"},
		{"bad\xffbyte", '-', "bad�byte"},
		{"  .Café. ", '-', "  .Café. "}, // trimming is the caller's
	} {
		if got := SafeName(tc.in, tc.repl); got != tc.want {
			t.Errorf("SafeName(%q, %q) = %q, want %q", tc.in, tc.repl, got, tc.want)
		}
	}
}

func TestCutUTF8(t *testing.T) {
	long := strings.Repeat("a", 79) + "é" + "tail"
	if got := CutUTF8(long, 80); got != strings.Repeat("a", 79) || !utf8.ValidString(got) {
		t.Errorf("CutUTF8 through é = %q", got)
	}
	if got := CutUTF8("short", 80); got != "short" {
		t.Errorf("CutUTF8(short) = %q", got)
	}
	if got := CutUTF8("日本", 3); got != "日" {
		t.Errorf("CutUTF8 at a boundary = %q", got)
	}
	if got := CutUTF8("日本", 2); got != "" {
		t.Errorf("CutUTF8 inside the first character = %q", got)
	}
}

func TestWindowsReserved(t *testing.T) {
	for in, want := range map[string]bool{
		"CON": true, "nul.txt": true, "Com1.mdx": true, "lpt9": true, "aux.tar.gz": true,
		"console": false, "com10": false, ".con": false, "a.con": false, "": false,
	} {
		if got := WindowsReserved(in); got != want {
			t.Errorf("WindowsReserved(%q) = %v, want %v", in, got, want)
		}
	}
}
