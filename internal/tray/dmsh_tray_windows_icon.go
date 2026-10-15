// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tray

import _ "embed"

//go:embed icons/tray-windows.png
var windowsIconPNG []byte

func init() {
	// The shared tray image remains the upstream mark for Linux. Windows is the
	// wuDict2 desktop product and uses the fork's digit-bearing mark.
	iconPNG = windowsIconPNG
}
