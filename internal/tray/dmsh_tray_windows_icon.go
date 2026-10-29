// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tray

import _ "embed"
import (
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

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

var (
	windowsTrayChoiceMu sync.RWMutex
	windowsTrayChoice   string
	windowsTaskbarIcon  func(string)
	windowsThemeWatch   sync.Once
)

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
	if _, ok := windowsTrayIcons[name]; !ok {
		return false
	}
	windowsTrayChoiceMu.Lock()
	windowsTrayChoice = name
	windowsTrayChoiceMu.Unlock()
	applyWindowsTrayChoice(name)
	windowsThemeWatch.Do(func() { go watchWindowsTrayTheme() })
	return true
}

func SetWindowsTaskbarIconCallback(callback func(string)) {
	windowsTrayChoiceMu.Lock()
	windowsTaskbarIcon = callback
	name := windowsTrayChoice
	windowsTrayChoiceMu.Unlock()
	if name != "" {
		applyWindowsTrayChoice(name)
	}
}

func applyWindowsTrayChoice(name string) {
	icon := windowsTrayIcons[name]
	dark := icon
	taskbarName := name
	switch name {
	case "icon1_transparent", "icon1_transparent_light":
		icon = trayIcon1Transparent
		dark = trayIcon1TransparentLight
		if windowsTrayUsesDarkTheme() {
			taskbarName = "icon1_transparent_light"
		} else {
			taskbarName = "icon1_transparent"
		}
	case "icon2_transparent", "icon2_transparent_light":
		icon = trayIcon2Transparent
		dark = trayIcon2TransparentLight
		if windowsTrayUsesDarkTheme() {
			taskbarName = "icon2_transparent_light"
		} else {
			taskbarName = "icon2_transparent"
		}
	}
	setActiveSystemTrayIcons(icon, dark)
	windowsTrayChoiceMu.RLock()
	callback := windowsTaskbarIcon
	windowsTrayChoiceMu.RUnlock()
	if callback != nil {
		callback(taskbarName)
	}
}

func windowsTrayUsesDarkTheme() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && value == 0
}

func watchWindowsTrayTheme() {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE|registry.NOTIFY)
	if err != nil {
		return
	}
	defer key.Close()
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return
	}
	defer windows.CloseHandle(event)
	for {
		if err := windows.RegNotifyChangeKeyValue(windows.Handle(key), false, windows.REG_NOTIFY_CHANGE_LAST_SET, event, true); err != nil {
			return
		}
		if _, err := windows.WaitForSingleObject(event, windows.INFINITE); err != nil {
			return
		}
		_ = windows.ResetEvent(event)
		windowsTrayChoiceMu.RLock()
		name := windowsTrayChoice
		windowsTrayChoiceMu.RUnlock()
		if name != "" {
			applyWindowsTrayChoice(name)
		}
	}
}
