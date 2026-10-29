// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iconUser32             = syscall.NewLazyDLL("user32.dll")
	iconLoadImage          = iconUser32.NewProc("LoadImageW")
	iconSendMessage        = iconUser32.NewProc("SendMessageW")
	iconShell32            = syscall.NewLazyDLL("shell32.dll")
	iconGetKnownFolderPath = iconShell32.NewProc("SHGetKnownFolderPath")
	iconShellChangeNotify  = iconShell32.NewProc("SHChangeNotify")
	iconOle32              = syscall.NewLazyDLL("ole32.dll")
	iconCoInitializeEx     = iconOle32.NewProc("CoInitializeEx")
	iconCoUninitialize     = iconOle32.NewProc("CoUninitialize")
	iconCoCreateInstance   = iconOle32.NewProc("CoCreateInstance")
	iconCoTaskMemFree      = iconOle32.NewProc("CoTaskMemFree")
)

type desktopKnownFolderID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

func desktopSetWindowIcon(hwnd uintptr, iconIndex int) {
	if hwnd == 0 || iconIndex < 0 || iconIndex > 3 {
		return
	}
	hinstance, _, _ := windowGetModuleHandle.Call(0)
	for _, size := range []struct {
		width int
		kind  uintptr
	}{{16, 0}, {32, 1}} {
		icon, _, _ := iconLoadImage.Call(hinstance, uintptr(101+iconIndex), 1, uintptr(size.width), uintptr(size.width), 0x8000)
		if icon != 0 {
			iconSendMessage.Call(hwnd, 0x80, size.kind, icon) // WM_SETICON
		}
	}
}

// desktopUpdateShortcutIcon updates the DictShelf shortcut when Setup created
// one. The icon index is the resource order in DictShelf.exe (101 through 104).
func desktopUpdateShortcutIcon(iconIndex int) {
	if iconIndex < 0 || iconIndex > 3 {
		return
	}
	var candidates []string
	for _, folder := range []desktopKnownFolderID{
		{Data1: 0xB4BFCC3A, Data2: 0xDB2C, Data3: 0x424C, Data4: [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41}}, // Desktop
		{Data1: 0xC4AA340D, Data2: 0xF20F, Data3: 0x4863, Data4: [8]byte{0xAF, 0xEF, 0xF8, 0x7E, 0xF2, 0xE6, 0xBA, 0x25}}, // Public Desktop
	} {
		var path *uint16
		hr, _, _ := iconGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&folder)), 0, 0, uintptr(unsafe.Pointer(&path)))
		if int32(hr) < 0 || path == nil {
			continue
		}
		candidates = append(candidates, filepath.Join(windows.UTF16PtrToString(path), "DictShelf (DMSH Labs).lnk"))
		iconCoTaskMemFree.Call(uintptr(unsafe.Pointer(path)))
	}
	if len(candidates) == 0 {
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := iconCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
	if int32(hr) < 0 {
		return
	}
	defer iconCoUninitialize.Call()

	clsid := desktopKnownFolderID{Data1: 0x00021401, Data2: 0, Data3: 0, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLink := desktopKnownFolderID{Data1: 0x000214F9, Data2: 0, Data3: 0, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile := desktopKnownFolderID{Data1: 0x0000010B, Data2: 0, Data3: 0, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

	exePath, err := os.Executable()
	if err != nil {
		return
	}
	exe, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return
	}
	for _, linkPath := range candidates {
		if _, err := os.Stat(linkPath); err != nil {
			continue
		}
		wideLink, err := syscall.UTF16PtrFromString(linkPath)
		if err != nil {
			continue
		}
		var shellLink uintptr
		hr, _, _ := iconCoCreateInstance.Call(
			uintptr(unsafe.Pointer(&clsid)), 0, 1,
			uintptr(unsafe.Pointer(&iidShellLink)), uintptr(unsafe.Pointer(&shellLink)),
		)
		if int32(hr) < 0 || shellLink == 0 {
			continue
		}
		var persist uintptr
		hr = desktopCOMCall(shellLink, 0, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&persist))) // QueryInterface
		if int32(hr) >= 0 && persist != 0 {
			loadHR := desktopCOMCall(persist, 5, uintptr(unsafe.Pointer(wideLink)), 2) // IPersistFile.Load, STGM_READWRITE
			if int32(loadHR) >= 0 {
				var shortcutTarget [32768]uint16
				pathHR := desktopCOMCall(shellLink, 3, uintptr(unsafe.Pointer(&shortcutTarget[0])), uintptr(len(shortcutTarget)), 0, 4) // GetPath, SLGP_RAWPATH
				if int32(pathHR) >= 0 && strings.EqualFold(filepath.Clean(windows.UTF16ToString(shortcutTarget[:])), filepath.Clean(exePath)) {
					setHR := desktopCOMCall(shellLink, 17, uintptr(unsafe.Pointer(exe)), uintptr(-101-iconIndex)) // explicit resource IDs 101–104
					if int32(setHR) >= 0 {
						saveHR := desktopCOMCall(persist, 6, uintptr(unsafe.Pointer(wideLink)), 1) // IPersistFile.Save
						if int32(saveHR) >= 0 {
							iconShellChangeNotify.Call(0x00002000, 0x1005, uintptr(unsafe.Pointer(wideLink)), 0) // SHCNE_UPDATEITEM, SHCNF_PATHW|SHCNF_FLUSH
						}
					}
				}
			}
			desktopCOMCall(persist, 2) // Release
		}
		desktopCOMCall(shellLink, 2) // Release
		runtime.KeepAlive(&clsid)
		runtime.KeepAlive(&iidShellLink)
		runtime.KeepAlive(&iidPersistFile)
		runtime.KeepAlive(wideLink)
		runtime.KeepAlive(exe)
	}
}

func desktopCOMCall(object uintptr, method int, args ...uintptr) uintptr {
	vtable := *(*uintptr)(unsafe.Pointer(object))
	entry := *(*uintptr)(unsafe.Pointer(vtable + uintptr(method)*unsafe.Sizeof(uintptr(0))))
	callArgs := make([]uintptr, 1, len(args)+1)
	callArgs[0] = object
	callArgs = append(callArgs, args...)
	hr, _, _ := syscall.SyscallN(entry, callArgs...)
	return hr
}
