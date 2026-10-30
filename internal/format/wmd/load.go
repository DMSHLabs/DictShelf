// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// Suffixes (spec R2.2, R2.4). A plain ".md" is claimed by content (Sniff).
const (
	Ext      = ".wudict.md"
	ExtGz    = ".wudict.md.gz"
	ExtDz    = ".wudict.md.dz" // dictzip is gzip with an index; read, never written
	ExtPlain = ".md"
)

// maxMarkdown is what any compressed file may unpack to, whatever its own
// size: the floor R2.4 sets every reader. A variable so the tests can reach it
// without writing gigabytes.
var maxMarkdown int64 = 1 << 30

// unpackRatio is how many times its own size a compressed file may unpack to,
// where that is more than maxMarkdown (R2.4). Markup gzips 3 to 10 times - a
// 166 MB Spanish Wiktionary unpacks 7.0 times - and a gzip bomb 100 times and
// more: 32 leaves room above the first and stays well under the second. A
// variable for the tests.
var unpackRatio int64 = 32

// textMax is the most decoded text this build addresses: its offsets are int,
// so 2 GiB on a 32-bit build. 1<<62 on a 64-bit one keeps limit+1 finite.
const textMax = min(math.MaxInt, 1<<62)

// unpackBound is what a compressed file of packed bytes may unpack to.
func unpackBound(packed int64) int64 {
	b := maxMarkdown
	if packed > 0 && unpackRatio > 0 {
		if packed > textMax/unpackRatio {
			return textMax
		}
		b = max(b, packed*unpackRatio)
	}
	return min(b, textMax)
}

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
// Both are decided by the file's bytes and this reader's rules, so every one
// is dict.ErrUnreadable; a text past its bound (R2.4) is dict.ErrTooLarge too.
type Error struct {
	Code string
	Msg  string

	tooLarge bool
}

func (e *Error) Error() string { return "wudict markdown: " + e.Code + ": " + e.Msg }

func (e *Error) Unwrap() []error {
	if e.tooLarge {
		return []error{dict.ErrUnreadable, dict.ErrTooLarge}
	}
	return []error{dict.ErrUnreadable}
}

func formatErr(format string, args ...any) error {
	return &Error{Code: "E-format", Msg: fmt.Sprintf(format, args...)}
}

// overErr is the E-format of decoded text past limit; packed is the size of
// the compressed file it came from, 0 for a plain file.
func overErr(limit, packed int64) error {
	e := &Error{Code: "E-format", tooLarge: true}
	if packed == 0 || limit >= textMax {
		e.Msg = fmt.Sprintf("text larger than %s, the most this build reads", logx.Size(limit))
	} else {
		e.Msg = fmt.Sprintf("unpacks to more than %s, the limit for a compressed file of %s",
			logx.Size(limit), logx.Size(packed))
	}
	return e
}

func versionErr(format string, args ...any) error {
	return &Error{Code: "E-version", Msg: fmt.Sprintf(format, args...)}
}
