// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package fsx

// FreeSpace is the space an unprivileged writer may still use on the file
// system holding dir, in bytes; ok is false where it cannot be read (an
// unsupported platform, a missing folder), and a caller then proceeds as if
// space were not its concern - a write that runs out reports that itself.
func FreeSpace(dir string) (free int64, ok bool) { return freeSpace(dir) }

// clampFree is a uint64 byte count as an int64, saturating: no file system
// holds 8 EB, but a bogus statfs answer must not turn negative.
func clampFree(n uint64) int64 {
	if n > 1<<62 {
		return 1 << 62
	}
	return int64(n)
}
