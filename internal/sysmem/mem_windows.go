// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package sysmem

import (
	"syscall"
	"unsafe"
)

// stdlib syscall, not golang.org/x/sys, which does not wrap this call (see
// internal/fsx/free_windows.go).
var procGlobalMemoryStatusEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// memoryStatusEx is MEMORYSTATUSEX: dwLength must be its size, 64 bytes.
type memoryStatusEx struct {
	length, memoryLoad   uint32
	totalPhys, availPhys uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

func status() (memoryStatusEx, bool) {
	m := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	return m, r != 0
}

func total() int64 {
	m, ok := status()
	if !ok {
		return 0
	}
	return int64(m.totalPhys)
}

func available() (int64, bool) {
	m, ok := status()
	if !ok {
		return 0, false
	}
	return int64(m.availPhys), true
}
