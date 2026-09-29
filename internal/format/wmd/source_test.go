// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDecodeStream: decoding a block at a time equals decoding the whole,
// with CR LF pairs, multi-byte characters, invalid bytes and NUL falling on
// every block boundary.
func TestDecodeStream(t *testing.T) {
	unit := "a\r\né漢\x00\xff😀\rb\n" // 19 bytes: shifts against 256 KiB blocks
	for _, n := range []int{1, 1000, 30000, 50000} {
		in := "\xEF\xBB\xBF" + strings.Repeat(unit, n) + "\r"
		var out bytes.Buffer
		changed, err := decodeStream(strings.NewReader(in), &out, 1<<40)
		if err != nil {
			t.Fatal(err)
		}
		if want := decode([]byte(in)); !bytes.Equal(out.Bytes(), want) || !changed {
			t.Fatalf("n=%d: stream and whole decoding differ (changed %v)", n, changed)
		}
	}
	var out bytes.Buffer
	if changed, err := decodeStream(strings.NewReader("clean é\n"), &out, 1<<40); err != nil || changed || out.String() != "clean é\n" {
		t.Errorf("clean text: %q %v %v", out.String(), changed, err)
	}
	if _, err := decodeStream(strings.NewReader(strings.Repeat("x", 100)), &out, 50); err == nil {
		t.Error("the limit was not enforced")
	}
}

// TestScannerCandidates: the streamed candidate lines are exactly the lines
// that start with `##` and WS or end of line.
func TestScannerCandidates(t *testing.T) {
	text := "# T\nwudict: 1\n\n## a\n##b\n##\n ## no\n##\tc\nx ## y\n## d"
	var want []cand
	off := 0
	for _, l := range strings.SplitAfter(text, "\n") {
		body := strings.TrimSuffix(l, "\n")
		if strings.HasPrefix(body, "##") && (len(body) == 2 || body[2] == ' ' || body[2] == '\t') {
			want = append(want, cand{at: off, end: off + len(l)})
		}
		off += len(l)
	}
	s := sourceFromBytes([]byte(text))
	if !reflect.DeepEqual(s.cands, want) {
		t.Errorf("cands = %v, want %v", s.cands, want)
	}
	if s.size != len(text) || string(s.head) != "# T\nwudict: 1\n" || s.defs {
		t.Errorf("size %d head %q defs %v", s.size, s.head, s.defs)
	}
	if !sourceFromBytes([]byte("# T\nwudict: 1\n\n[x]: y\n")).defs {
		t.Error("a reference definition was not noticed")
	}
}

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, "wudict-md-*"))
	return m
}

// TestSourceFiles: a clean plain file is read where it is; a file that needs
// repair, or a compressed one, through a temporary file that Close removes.
func TestSourceFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	const doc = "# D\nwudict: 1\n\n## w\n\nbody\n"
	for _, tc := range []struct {
		name string
		data []byte
		temp bool
	}{
		{"clean" + Ext, []byte(doc), false},
		{"crlf" + Ext, []byte(strings.ReplaceAll(doc, "\n", "\r\n")), true},
		{"bom" + Ext, append([]byte("\xEF\xBB\xBF"), doc...), true},
		{"gz" + ExtGz, gz(t, doc), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), tc.name)
			if err := os.WriteFile(p, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			r, err := NewReader(p)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(tempFiles(t, tmp)) > 0; got != tc.temp {
				t.Errorf("temporary file: %v, want %v", got, tc.temp)
			}
			if e, err := r.Next(); err != nil || e.Headwords[0] != "w" || e.Body != "<p>body</p>\n" {
				t.Errorf("entry: %+v, %v", e, err)
			}
			if err := r.Close(); err != nil {
				t.Error(err)
			}
			if left := tempFiles(t, tmp); len(left) != 0 {
				t.Errorf("left behind: %v", left)
			}
		})
	}
}

// TestTruncatedGzip: a gzip stream cut short is an error, not a short text.
func TestTruncatedGzip(t *testing.T) {
	full := gz(t, "# D\nwudict: 1\n\n## w\n\n"+strings.Repeat("body ", 10000)+"\n")
	p := filepath.Join(t.TempDir(), "cut"+ExtGz)
	if err := os.WriteFile(p, full[:len(full)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	var fe *Error
	if _, err := NewReader(p); !errors.As(err, &fe) || fe.Code != "E-format" {
		t.Errorf("err = %v, want E-format", err)
	}
}
