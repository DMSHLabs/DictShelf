// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tray

import _ "embed"

//go:embed icons/tray-windows.png
var windowsIconPNG []byte

func init() {
	// Windows uses the DictShelf book-on-shelf mark.
	iconPNG = windowsIconPNG
}
