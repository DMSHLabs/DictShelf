// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tray

import _ "embed"

//go:embed icons/tray-windows-icon1.png
var trayIcon1 []byte

//go:embed icons/tray-windows-icon1_transparent.png
var trayIcon1Transparent []byte

//go:embed icons/tray-windows-icon1_transparent_light.png
var trayIcon1TransparentLight []byte

//go:embed icons/tray-windows-icon2.png
var trayIcon2 []byte

//go:embed icons/tray-windows-icon2_transparent.png
var trayIcon2Transparent []byte

//go:embed icons/tray-windows-icon2_transparent_light.png
var trayIcon2TransparentLight []byte

var windowsTrayIcons = map[string][]byte{
	"icon1": trayIcon1, "icon1_transparent": trayIcon1Transparent,
	"icon1_transparent_light": trayIcon1TransparentLight, "icon2": trayIcon2,
	"icon2_transparent": trayIcon2Transparent, "icon2_transparent_light": trayIcon2TransparentLight,
}

func init() { iconPNG = trayIcon2Transparent }

func WindowsIconIndex(name string) (int, bool) {
	switch name {
	case "icon2_transparent":
		return 0, true
	case "icon1":
		return 1, true
	case "icon1_transparent":
		return 2, true
	case "icon1_transparent_light":
		return 3, true
	case "icon2":
		return 4, true
	case "icon2_transparent_light":
		return 5, true
	default:
		return 0, false
	}
}

func SetWindowsIcon(name string) bool {
	icon, ok := windowsTrayIcons[name]
	if !ok {
		return false
	}
	setActiveSystemTrayIcon(icon)
	return true
}
