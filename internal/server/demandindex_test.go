// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuweidict/wudict/internal/fsx"
)

// unpreparedEntry is one dictionary with no library index in an isolated
// registry.
func unpreparedEntry(t *testing.T) (*Server, *entry) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "d.fake"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedDBDir(t)
	reg := unpreparedRegistry(t, []string{dir})
	return New(reg), reg.all()[0]
}

// waitUntil polls a condition that background work will make true.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func prepared(e *entry) bool {
	p, ok := validPrepared(e.Path)
	return ok && fsx.FileExists(p)
}
