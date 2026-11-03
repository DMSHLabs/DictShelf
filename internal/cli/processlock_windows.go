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

	"golang.org/x/sys/windows"
)

var (
	processLockKernel32 = syscall.NewLazyDLL("kernel32.dll")
	createMutexW        = processLockKernel32.NewProc("CreateMutexW")
	closeHandle         = processLockKernel32.NewProc("CloseHandle")
)

// acquireProcessLock allows only one DictShelf Windows process per user session.
// The GUI and CLI builds share configuration and library files, so a different
// port or executable name must not let both processes work at the same time.
func acquireProcessLock() (func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot identify the Windows user: %w", err)
	}
	identity := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(home))))
	name, err := syscall.UTF16PtrFromString(fmt.Sprintf(`Local\DictShelf-%x`, identity[:12]))
	if err != nil {
		return nil, fmt.Errorf("cannot create the DictShelf process lock: %w", err)
	}
	handle, _, callErr := createMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	if handle == 0 {
		if callErr == nil {
			callErr = syscall.EINVAL
		}
		return nil, fmt.Errorf("cannot create the DictShelf process lock: %w", callErr)
	}
	const errorAlreadyExists syscall.Errno = 183
	if errno, ok := callErr.(syscall.Errno); ok && errno == errorAlreadyExists {
		closeHandle.Call(handle)
		if guiLaunch {
			activateRunningDesktopWindow()
			return nil, errExistingDesktopInstance
		}
		return nil, fmt.Errorf("DictShelf is already running. Close it before starting another copy")
	}
	return func() { closeHandle.Call(handle) }, nil
}

var processLockUser32 = syscall.NewLazyDLL("user32.dll")

const desktopReaderWindowClass = "webview"

var (
	desktopEnumWindows   = processLockUser32.NewProc("EnumWindows")
	desktopClassName     = processLockUser32.NewProc("GetClassNameW")
	desktopWindowProcess = processLockUser32.NewProc("GetWindowThreadProcessId")
	desktopEnumCallback  = syscall.NewCallback(desktopWindowSearchProc)
	desktopSearchImage   string
	desktopSearchResult  uintptr
)

// desktopWindowSearchProc keeps the first top-level reader window that belongs
// to another process running this same program.
func desktopWindowSearchProc(hwnd, _ uintptr) uintptr {
	name := make([]uint16, 64)
	count, _, _ := desktopClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	runtime.KeepAlive(name)
	if count == 0 || syscall.UTF16ToString(name[:count]) != desktopReaderWindowClass {
		return 1
	}
	var pid uint32
	desktopWindowProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == uint32(os.Getpid()) {
		return 1
	}
	// The executable's name, not its full path: one user runs one DictShelf (the
	// lock guarantees it), and a second copy in another folder is the same program
	// and must still be able to bring the window back.
	if !strings.EqualFold(filepath.Base(desktopProcessImage(pid)), desktopSearchImage) {
		return 1
	}
	desktopSearchResult = hwnd
	return 0
}

// desktopOwnReaderWindow returns the reader window owned by the running copy of
// this program, or 0.
//
// It must not be found by TITLE: the reader is titled "DictShelf", and Windows 11
// mirrors a window's title onto a shell-owned proxy of its own
// (Windows.Internal.Shell.TabProxyWindow) that FindWindow returns first and that
// restoring does nothing for. Finding it by title therefore left a second launch
// looking like it had done nothing at all.
func desktopOwnReaderWindow() uintptr {
	exe, err := os.Executable()
	if err != nil {
		return 0
	}
	desktopSearchImage = filepath.Base(exe)
	desktopSearchResult = 0
	desktopEnumWindows.Call(desktopEnumCallback, 0)
	return desktopSearchResult
}

func desktopProcessImage(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 1024)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return syscall.UTF16ToString(buffer[:size])
}

// activateRunningDesktopWindow waits briefly because the first process can
// own the mutex while WebView2 is still creating its top-level window.
func activateRunningDesktopWindow() {
	for deadline := time.Now().Add(4 * time.Second); ; {
		if hwnd := desktopOwnReaderWindow(); hwnd != 0 {
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
		swpNoMove             = 0x0002
		swpNoZOrder           = 0x0004
		swpNoActivate         = 0x0010
		swpShowWindow         = 0x0040
	)
	showWindow.Call(hwnd, swRestore)
	desktopReapplyTaskbarIcon(hwnd)

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
	// Put the restored window at the top of the normal Z order and activate it.
	// This complements SetForegroundWindow for restores triggered by the global
	// keyboard hook, which runs outside WebView2's UI thread.
	setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	setForegroundWindow.Call(hwnd)
}

func int32AsUint(value int32) uint32 { return uint32(value) }
