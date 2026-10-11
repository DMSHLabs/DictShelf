// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin || linux

package fsx

import "syscall"

func freeSpace(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	// Bavail, not Bfree: the blocks reserved for root are not ours to fill.
	// The field types differ per platform; both are non-negative counts.
	bsize := uint64(st.Bsize)
	bavail := uint64(st.Bavail)
	if bsize != 0 && bavail > (1<<62)/bsize {
		return 1 << 62, true
	}
	return clampFree(bavail * bsize), true
}
