// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	processLockKernel32 = syscall.NewLazyDLL("kernel32.dll")
	createMutexW        = processLockKernel32.NewProc("CreateMutexW")
	closeHandle         = processLockKernel32.NewProc("CloseHandle")
)

// acquireProcessLock allows only one wuDict2 Windows process per user session.
// The GUI and CLI builds share configuration and library files, so a different
// port or executable name must not let both processes work at the same time.
func acquireProcessLock() (func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot identify the Windows user: %w", err)
	}
	identity := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(home))))
	name, err := syscall.UTF16PtrFromString(fmt.Sprintf(`Local\wuDict2-%x`, identity[:12]))
	if err != nil {
		return nil, fmt.Errorf("cannot create the wuDict2 process lock: %w", err)
	}
	handle, _, callErr := createMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	if handle == 0 {
		if callErr == nil {
			callErr = syscall.EINVAL
		}
		return nil, fmt.Errorf("cannot create the wuDict2 process lock: %w", callErr)
	}
	const errorAlreadyExists syscall.Errno = 183
	if errno, ok := callErr.(syscall.Errno); ok && errno == errorAlreadyExists {
		closeHandle.Call(handle)
		return nil, fmt.Errorf("wuDict2 is already running. Close it before starting another copy")
	}
	return func() { closeHandle.Call(handle) }, nil
}
