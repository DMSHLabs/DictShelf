// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/format/wmd"
)

func TestFailuresKeep(t *testing.T) {
	saved := diskFree
	defer func() { diskFree = saved }()
	room := &dict.NoRoomError{What: "to unpack it", Dir: "/tmp", Need: 5000, Free: 100}
	unreadable := &wmd.Error{Code: "E-format", Msg: "not a gzip stream"}
	tests := []struct {
		name     string
		err      error
		sig      string // at lookup; the failure is recorded at "1:1"
		free     int64
		freeOK   bool
		want     bool // lookup answers with a failure
		tooLarge bool
	}{
		{"unreadable, same file", unreadable, "1:1", 0, false, true, false},
		{"unreadable, file changed", unreadable, "2:1", 0, false, false, false},
		{"not a kept kind", errors.New("permission denied"), "1:1", 0, false, false, false},
		{"no room in memory", &dict.NoRoomError{What: "to unpack it", Need: 9, Free: 1}, "1:1", 0, false, false, false},
		{"no room, still none", room, "1:1", 4000, true, true, true},
		{"no room, now room", room, "1:1", 6000, true, false, false},
		{"no room, free space unknown", room, "1:1", 0, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diskFree = func(string) (int64, bool) { return tt.free, tt.freeOK }
			f := loadFailures("")
			f.record("/d/x.wudict.md.gz", "1:1", tt.err)
			err := f.lookup("/d/x.wudict.md.gz", tt.sig)
			if (err != nil) != tt.want {
				t.Fatalf("lookup = %v, want a failure %v", err, tt.want)
			}
			if err == nil {
				if f.lookup("/d/x.wudict.md.gz", "1:1") != nil {
					t.Error("a record that no longer holds was kept")
				}
				return
			}
			if errors.Is(err, dict.ErrTooLarge) != tt.tooLarge {
				t.Errorf("ErrTooLarge = %v, want %v", !tt.tooLarge, tt.tooLarge)
			}
			if !strings.Contains(err.Error(), strings.SplitN(tt.err.Error(), ":", 2)[0]) {
				t.Errorf("recalled %q for %q", err, tt.err)
			}
		})
	}
}

func TestFailuresFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.wudict.md.gz")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "open-failures.json")
	f := loadFailures(path)
	f.record(src, "1:1", &wmd.Error{Code: "E-format", Msg: "not a gzip stream"})
	f.record(filepath.Join(dir, "gone.wudict.md.gz"), "1:1", &wmd.Error{Code: "E-format", Msg: "x"})

	g := loadFailures(path)
	if err := g.lookup(src, "1:1"); !errors.Is(err, dict.ErrUnreadable) {
		t.Errorf("after a restart: %v", err)
	}
	if len(g.recs) != 1 {
		t.Errorf("a record of a file that is gone was kept: %v", g.recs)
	}
	g.forget()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("forget left the file: %v", err)
	}

	for _, bad := range []string{"", "{", `{"version":99,"files":{}}`, `[]`} {
		if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if h := loadFailures(path); len(h.recs) != 0 {
			t.Errorf("%q: read %v", bad, h.recs)
		}
	}
}

// storedGz is text as a gzip file whose size is known in advance: stored, not
// compressed, so it is header + one stored block + trailer.
func storedGz(t *testing.T, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw, err := gzip.NewWriterLevel(&b, gzip.NoCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestOpenFailureKept: a failure kept from an earlier start answers the open
// without opening - proven with a file that would now open, made the same
// size and mtime as the one that failed - until the user's Rescan forgets it.
func TestOpenFailureKept(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "d.wudict.md.gz")
	good := storedGz(t, "# D\nwudict: 1\n\n## a\n\nb\n")
	if err := os.WriteFile(src, bytes.Repeat([]byte("x"), len(good)), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	sig := sourceSig(src)
	path := filepath.Join(t.TempDir(), "open-failures.json")

	first := &Registry{fails: loadFailures(path)}
	_, failed := (&entry{ID: "d", Path: src, reg: first}).open()
	if !errors.Is(failed, dict.ErrUnreadable) {
		t.Fatalf("garbage opened: %v", failed)
	}

	if err := os.WriteFile(src, good, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(src, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if sourceSig(src) != sig {
		t.Skip("this file system does not keep the mtime set")
	}

	second := &Registry{fails: loadFailures(path)}
	if _, err := (&entry{ID: "d", Path: src, reg: second}).open(); err == nil || err.Error() != failed.Error() {
		t.Fatalf("the kept failure did not answer: %v", err)
	}

	second.ForgetFailures()
	e := &entry{ID: "d", Path: src, reg: second}
	d, err := e.open()
	if err != nil {
		t.Fatalf("after Rescan: %v", err)
	}
	t.Cleanup(func() { e.ingestMu.Lock(); e.closeNow(); e.ingestMu.Unlock() })
	if d.Meta().Name != "D" {
		t.Errorf("opened %q", d.Meta().Name)
	}
}
