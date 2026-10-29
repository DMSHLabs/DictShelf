// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import "syscall"

var desktopDPIUser32 = syscall.NewLazyDLL("user32.dll")

func init() {
	// The tray backend creates its HICON from SM_CXSMICON. In a DPI-unaware
	// process Windows reports 16px and then scales that raster for the taskbar.
	// Per-monitor awareness makes both tray and window metrics match the
	// display's physical DPI before either icon is created.
	setDPIAwareness := desktopDPIUser32.NewProc("SetProcessDpiAwarenessContext")
	if setDPIAwareness.Find() == nil {
		if result, _, _ := setDPIAwareness.Call(^uintptr(3)); result != 0 { // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
			return
		}
	}
	setDPIAware := desktopDPIUser32.NewProc("SetProcessDPIAware")
	if setDPIAware.Find() == nil {
		setDPIAware.Call()
	}
}
