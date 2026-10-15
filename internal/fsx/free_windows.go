// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package fsx

import (
	"syscall"
	"unsafe"
)

// stdlib syscall, not golang.org/x/sys: kernel32.dll is a KnownDLL, so the
// System32-only search x/sys adds buys nothing (see
// internal/server/foreground_windows.go).
var procGetDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func freeSpace(dir string) (int64, bool) {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, false
	}
	// the first out-parameter is the space available to this caller, which
	// honours per-user quotas; the totals are not needed
	var avail uint64
	if r, _, _ := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), 0, 0); r == 0 {
		return 0, false
	}
	return clampFree(avail), true
}
