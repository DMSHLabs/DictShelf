// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package fsx

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestFreeSpace(t *testing.T) {
	dir := t.TempDir()
	supported := runtime.GOOS == "darwin" || runtime.GOOS == "linux" ||
		runtime.GOOS == "android" || runtime.GOOS == "windows"
	tests := []struct {
		name   string
		dir    string
		wantOK bool
	}{
		{"existing folder", dir, supported},
		{"missing folder", filepath.Join(dir, "no", "such"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			free, ok := FreeSpace(tt.dir)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && free <= 0 {
				t.Errorf("free = %d on a writable folder", free)
			}
		})
	}
}

func TestClampFree(t *testing.T) {
	tests := []struct {
		in   uint64
		want int64
	}{
		{0, 0},
		{12345, 12345},
		{1 << 62, 1 << 62},
		{1<<64 - 1, 1 << 62},
	}
	for _, tt := range tests {
		if got := clampFree(tt.in); got != tt.want {
			t.Errorf("clampFree(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
