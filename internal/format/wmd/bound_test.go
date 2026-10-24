// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

func TestUnpackBound(t *testing.T) {
	tests := []struct {
		name   string
		packed int64
		want   int64
	}{
		{"unknown size: the floor", 0, 1 << 30},
		{"small file: the floor", 1_000_000, 1 << 30},
		{"at the crossover", (1 << 30) / 32, 1 << 30},
		{"166 MB: 32 times", 166_000_000, min(166_000_000*32, textMax)},
		{"just past what 32 times can hold", textMax/32 + 1, textMax},
		{"huge", 1 << 61, textMax},
	}
	for _, tt := range tests {
		if got := unpackBound(tt.packed); got != tt.want {
			t.Errorf("%s: unpackBound(%d) = %d, want %d", tt.name, tt.packed, got, tt.want)
		}
	}
}

// bigGz is a .wudict.md.gz whose text is far larger than the file.
func bigGz(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "big"+ExtGz)
	if err := os.WriteFile(p, gz(t, "# Big\nwudict: 1\n\n## a\n\n"+strings.Repeat("x", 8192)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestUnpackRatio: past the floor, the bound grows with the file (R2.4).
func TestUnpackRatio(t *testing.T) {
	savedMax, savedRatio := maxMarkdown, unpackRatio
	defer func() { maxMarkdown, unpackRatio = savedMax, savedRatio }()
	p := bigGz(t)
	tests := []struct {
		name    string
		ratio   int64
		wantErr bool
	}{
		{"ratio 1: over", 1, true},
		{"ratio 10000: within", 10000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxMarkdown, unpackRatio = 1024, tt.ratio
			r, err := NewReader(p)
			if r != nil {
				r.Close()
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "unpacks to more than") {
				t.Errorf("message: %v", err)
			}
		})
	}
}

// TestNoRoom: unpacking stops above the disk's reserve, and that is not a
// verdict on the file.
func TestNoRoom(t *testing.T) {
	saved, savedRoom := freeSpace, textRoom
	defer func() { freeSpace, textRoom = saved, savedRoom }()
	textRoom = func() int64 { return 0 } // the temporary file is what is tested
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // where os.CreateTemp("") writes, on unix
	p := bigGz(t)
	tests := []struct {
		name   string
		free   int64
		ok     bool
		noRoom bool
	}{
		{"plenty", 1 << 40, true, false},
		{"unknown", 0, false, false},
		{"under the reserve", unpackReserve + roomStep - 1, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			freeSpace = func(string) (int64, bool) { return tt.free, tt.ok }
			r, err := NewReader(p)
			if r != nil {
				r.Close()
			}
			var re *dict.NoRoomError
			if got := errors.As(err, &re); got != tt.noRoom {
				t.Fatalf("err = %v, want NoRoomError %v", err, tt.noRoom)
			}
			if !tt.noRoom {
				return
			}
			if !errors.Is(err, dict.ErrTooLarge) || errors.Is(err, dict.ErrUnreadable) {
				t.Errorf("NoRoomError must be ErrTooLarge and not ErrUnreadable: %v", err)
			}
			if re.Need <= re.Free {
				t.Errorf("Need %d <= Free %d", re.Need, re.Free)
			}
			if runtime.GOOS != "windows" {
				if left, _ := filepath.Glob(filepath.Join(tmp, "wudict-md-*")); len(left) > 0 {
					t.Errorf("temporary file left behind: %v", left)
				}
			}
		})
	}
}

// trailer is a stand-in for a gzip file of size bytes whose last 4 bytes say
// isize; only the trailer is read.
type trailer uint32

func (t trailer) ReadAt(p []byte, _ int64) (int, error) {
	binary.LittleEndian.PutUint32(p, uint32(t))
	return 4, nil
}

func TestUnpackEstimate(t *testing.T) {
	const gib4 = int64(1) << 32
	tests := []struct {
		name  string
		isize uint32
		size  int64
		limit int64
		want  int64
	}{
		{"the trailer", 12345, 64, 1 << 40, 12345},
		{"text smaller than its gzip file", 23, 50, 1 << 40, 23},
		{"5 GB from 1 GB: wrapped once", uint32(5_000_000_000 - gib4), 1_000_000_000, 1 << 40, 5_000_000_000},
		{"wrapped, landing above the file: short by 4 GiB", uint32(5_000_000_000 - gib4), 700_000_000, 1 << 40, 5_000_000_000 - gib4},
		{"9 GB from 4.5 GB: the least the trailer allows", uint32(9_000_000_000 - 2*gib4), 4_500_000_000, 1 << 40, 9_000_000_000 - gib4},
		{"past the limit: nothing", 12345, 64, 1000, 0},
		{"too short to be gzip", 12345, 17, 1 << 40, 0},
		{"zero", 0, 64, 1 << 40, 0},
	}
	for _, tt := range tests {
		if got := unpackEstimate(trailer(tt.isize), tt.size, tt.limit); got != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, got, tt.want)
		}
	}
}

// TestUnpackReport: unpacking says how far it has come, against the
// trailer's size.
func TestUnpackReport(t *testing.T) {
	text := "# R\nwudict: 1\n\n## a\n\nb\n"
	p := filepath.Join(t.TempDir(), "r"+ExtGz)
	if err := os.WriteFile(p, gz(t, text), 0o644); err != nil {
		t.Fatal(err)
	}
	defer dict.ClearActivity(p)
	r, err := NewReader(p)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	want := dict.Activity{Phase: dict.PhaseUnpacking, Done: int64(len(text)), Total: int64(len(text))}
	if a, ok := dict.ActivityOf(p); !ok || a != want {
		t.Errorf("activity %+v, %v; want %+v", a, ok, want)
	}
}
