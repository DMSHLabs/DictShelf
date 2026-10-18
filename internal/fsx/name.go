// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package fsx

import (
	"strings"
	"unicode/utf8"
)

// SafeName is the character pass every folder name made from a dictionary's
// name goes through: control characters dropped (a newline makes a name
// unreadable in every listing), and each character no file system here allows
// in a name - / \ : * ? " < > | - written as repl. Invalid UTF-8 reads as
// U+FFFD, so the result is valid UTF-8. What else a name needs - trimming,
// length, a fallback - depends on where it is used, and the names already on
// disk depend on it, so it is the caller's.
func SafeName(name string, repl byte) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
		case strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteByte(repl)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CutUTF8 is s cut to at most n bytes, at a character boundary: a name cut
// inside a UTF-8 sequence is not a valid file name on every file system.
func CutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// windowsReserved are the device names MS-DOS bequeathed to every Windows
// file system, which are still not usable as file names and still resolve to
// the device WITH any extension attached.
var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// WindowsReserved reports a name Windows resolves to a device, extension or
// not: "CON", "nul.txt", "Com1.mdx".
func WindowsReserved(name string) bool {
	base, _, _ := strings.Cut(name, ".")
	return windowsReserved[strings.ToLower(name)] || windowsReserved[strings.ToLower(base)]
}
