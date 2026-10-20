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
	"time"
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
		if guiLaunch {
			activateRunningDesktopWindow()
			return nil, errExistingDesktopInstance
		}
		return nil, fmt.Errorf("wuDict2 is already running. Close it before starting another copy")
	}
	return func() { closeHandle.Call(handle) }, nil
}

var processLockUser32 = syscall.NewLazyDLL("user32.dll")

// activateRunningDesktopWindow waits briefly because the first process can
// own the mutex while WebView2 is still creating its top-level window.
func activateRunningDesktopWindow() {
	title, err := syscall.UTF16PtrFromString("wuDict2")
	if err != nil {
		return
	}
	findWindowW := processLockUser32.NewProc("FindWindowW")
	for deadline := time.Now().Add(4 * time.Second); ; {
		hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(title)))
		if hwnd != 0 {
			restoreAndFocusWindow(hwnd)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type windowRect struct {
	left, top, right, bottom int32
}

type monitorInfo struct {
	cbSize  uint32
	monitor windowRect
	work    windowRect
	dwFlags uint32
}

func restoreAndFocusWindow(hwnd uintptr) {
	showWindow := processLockUser32.NewProc("ShowWindow")
	setForegroundWindow := processLockUser32.NewProc("SetForegroundWindow")
	getWindowRect := processLockUser32.NewProc("GetWindowRect")
	monitorFromWindow := processLockUser32.NewProc("MonitorFromWindow")
	getMonitorInfoW := processLockUser32.NewProc("GetMonitorInfoW")
	setWindowPos := processLockUser32.NewProc("SetWindowPos")

	const (
		swRestore             = 9
		monitorDefaultNearest = 2
		swpNoSize             = 0x0001
		swpNoZOrder           = 0x0004
		swpNoActivate         = 0x0010
	)
	showWindow.Call(hwnd, swRestore)

	var bounds windowRect
	if ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds))); ok != 0 {
		monitor, _, _ := monitorFromWindow.Call(hwnd, monitorDefaultNearest)
		info := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		if monitor != 0 {
			if ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
				width := bounds.right - bounds.left
				height := bounds.bottom - bounds.top
				workWidth := info.work.right - info.work.left
				workHeight := info.work.bottom - info.work.top
				x, y := bounds.left, bounds.top
				if width > workWidth || x < info.work.left {
					x = info.work.left
				} else if bounds.right > info.work.right {
					x = info.work.right - width
				}
				if height > workHeight || y < info.work.top {
					y = info.work.top
				} else if bounds.bottom > info.work.bottom {
					y = info.work.bottom - height
				}
				if x != bounds.left || y != bounds.top {
					setWindowPos.Call(hwnd, 0, uintptr(int32AsUint(x)), uintptr(int32AsUint(y)), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
				}
			}
		}
	}
	setForegroundWindow.Call(hwnd)
}

func int32AsUint(value int32) uint32 { return uint32(value) }
