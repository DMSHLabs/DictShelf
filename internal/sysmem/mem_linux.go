// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package sysmem

// Linux, and Android, which builds with the linux files.

func total() int64 {
	t, _, _ := meminfo("/")
	return t
}

// available is MemAvailable, bounded by what the process's memory cgroups
// leave free.
func available() (int64, bool) {
	t, a, ok := meminfo("/")
	if c, cok := cgroupAvailable("/", t); cok && (!ok || c < a) {
		return c, true
	}
	return a, ok
}
