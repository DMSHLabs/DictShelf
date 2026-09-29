// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Suffixes (spec R2.2, R2.4). A plain ".md" is claimed by content (Sniff).
const (
	Ext      = ".wudict.md"
	ExtGz    = ".wudict.md.gz"
	ExtDz    = ".wudict.md.dz" // dictzip is gzip with an index; read, never written
	ExtPlain = ".md"
)

// maxMarkdown bounds what a compressed file may unpack to (R2.4). A variable
// so the tests can reach it without writing gigabytes.
var maxMarkdown int64 = 1 << 30

func hasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func compressed(path string) bool { return hasSuffixFold(path, ExtGz) || hasSuffixFold(path, ExtDz) }

// decode applies R2.1: a leading BOM removed, CRLF and lone CR turned into LF,
// invalid UTF-8 and NUL replaced by U+FFFD. Input that needs none of it is
// returned as is.
func decode(b []byte) []byte {
	out, _ := decodeBlock(trimBOM(b))
	return out
}

// decodeBlock is decode without the BOM: what applies anywhere in a text. It
// reports whether anything changed; unchanged input is returned as is.
func decodeBlock(b []byte) ([]byte, bool) {
	clean := utf8.Valid(b)
	for i := 0; clean && i < len(b); i++ {
		clean = b[i] != '\r' && b[i] != 0
	}
	if clean {
		return b, false
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		switch c := b[i]; {
		case c == '\r':
			out = append(out, '\n')
			i++
			if i < len(b) && b[i] == '\n' {
				i++
			}
		case c == 0:
			out = utf8.AppendRune(out, utf8.RuneError)
			i++
		case c < utf8.RuneSelf:
			out = append(out, c)
			i++
		default:
			r, n := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && n == 1 {
				out = utf8.AppendRune(out, utf8.RuneError)
			} else {
				out = append(out, b[i:i+n]...)
			}
			i += n
		}
	}
	return out, true
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// Error is a file this reader does not read (spec §8: E-format, E-version).
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return "wudict markdown: " + e.Code + ": " + e.Msg }

func formatErr(format string, args ...any) error {
	return &Error{Code: "E-format", Msg: fmt.Sprintf(format, args...)}
}

func versionErr(format string, args ...any) error {
	return &Error{Code: "E-version", Msg: fmt.Sprintf(format, args...)}
}
