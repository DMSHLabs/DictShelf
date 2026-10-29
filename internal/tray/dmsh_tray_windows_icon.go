// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tray

import _ "embed"

//go:embed icons/tray-windows.png
var windowsIconDictShelf2PNG []byte

//go:embed icons/tray-windows-dictshelf.png
var windowsIconDictShelfPNG []byte

//go:embed icons/tray-windows-dictshelf-transparent.png
var windowsIconDictShelfTransparentPNG []byte

//go:embed icons/tray-windows-dictshelf2-transparent.png
var windowsIconDictShelf2TransparentPNG []byte

func init() {
	// Windows uses the DictShelf book-on-shelf mark.
	iconPNG = windowsIconDictShelf2PNG
}

func WindowsIconIndex(name string) (int, bool) {
	switch name {
	case "dictshelf2":
		return 0, true
	case "dictshelf":
		return 1, true
	case "dictshelf-transparent":
		return 2, true
	case "dictshelf2-transparent":
		return 3, true
	default:
		return 0, false
	}
}

func SetWindowsIcon(name string) bool {
	var icon []byte
	switch name {
	case "dictshelf2":
		icon = windowsIconDictShelf2PNG
	case "dictshelf":
		icon = windowsIconDictShelfPNG
	case "dictshelf-transparent":
		icon = windowsIconDictShelfTransparentPNG
	case "dictshelf2-transparent":
		icon = windowsIconDictShelf2TransparentPNG
	default:
		return false
	}
	setActiveSystemTrayIcon(icon)
	return true
}
