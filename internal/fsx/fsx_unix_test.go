// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package fsx

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

// A pipe reports size 0 and would never end: refused, not read.
func TestReadBoundedPipe(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Skip("mkfifo:", err)
	}
	if _, err := ReadBounded(p, 10); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("err = %v, want ErrNotRegular", err)
	}
}
