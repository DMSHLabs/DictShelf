// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DMSHLabs/DictShelf/internal/config"
	"github.com/DMSHLabs/DictShelf/internal/tray"
)

var (
	iconUser32              = syscall.NewLazyDLL("user32.dll")
	iconLoadImage           = iconUser32.NewProc("LoadImageW")
	iconSendMessage         = iconUser32.NewProc("SendMessageW")
	iconRedrawWindow        = iconUser32.NewProc("RedrawWindow")
	iconGetSystemMetrics    = iconUser32.NewProc("GetSystemMetrics")
	iconGetSystemMetricsDPI = iconUser32.NewProc("GetSystemMetricsForDpi")
	iconGetDPIForWindow     = iconUser32.NewProc("GetDpiForWindow")
	iconShell32             = syscall.NewLazyDLL("shell32.dll")
	iconGetKnownFolderPath  = iconShell32.NewProc("SHGetKnownFolderPath")
	iconShellChangeNotify   = iconShell32.NewProc("SHChangeNotify")
	iconSetProcessAppID     = iconShell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	iconGetWindowStore      = iconShell32.NewProc("SHGetPropertyStoreForWindow")
	iconOle32               = syscall.NewLazyDLL("ole32.dll")
	iconCoInitializeEx      = iconOle32.NewProc("CoInitializeEx")
	iconCoUninitialize      = iconOle32.NewProc("CoUninitialize")
	iconCoCreateInstance    = iconOle32.NewProc("CoCreateInstance")
	iconCoTaskMemFree       = iconOle32.NewProc("CoTaskMemFree")
)

type desktopKnownFolderID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type desktopPropertyKey struct {
	FormatID   desktopKnownFolderID
	PropertyID uint32
}

type desktopPropVariant struct {
	Type     uint16
	Reserved [3]uint16
	Value    uintptr
}

var (
	desktopTaskbarIconChange  atomic.Uint64
	desktopCurrentTaskbarIcon atomic.Int32
)

const windowsTaskbarRefreshHelperArg = "--dictshelf-taskbar-refresh-helper"

func runWindowsTaskbarRefreshHelper(args []string) bool {
	if len(args) == 0 || args[0] != windowsTaskbarRefreshHelperArg {
		return false
	}
	if len(args) != 2 {
		return true
	}
	iconIndex, err := strconv.Atoi(args[1])
	if err != nil || iconIndex < 0 || iconIndex > 5 {
		return true
	}
	// Explorer can restore the pinned icon from its cached app identity after
	// the window disappears. Refresh after that transition has settled.
	time.Sleep(900 * time.Millisecond)
	desktopUpdateTaskbarShortcutIcon(iconIndex)
	return true
}

func desktopScheduleTaskbarShortcutRefresh(iconIndex int) {
	if iconIndex < 0 || iconIndex > 5 {
		return
	}
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exePath, windowsTaskbarRefreshHelperArg, strconv.Itoa(iconIndex))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
	}
}

func desktopSetWindowIcon(hwnd uintptr, iconIndex int) {
	if hwnd == 0 || iconIndex < 0 || iconIndex > 5 {
		return
	}
	hinstance, _, _ := windowGetModuleHandle.Call(0)
	width, height := desktopWindowIconSize(hwnd, 49, 50, 16) // SM_CXSMICON/CYSMICON
	icon, _, _ := iconLoadImage.Call(hinstance, uintptr(101+iconIndex), 1, width, height, 0x8000)
	if icon != 0 {
		iconSendMessage.Call(hwnd, 0x80, 0, icon) // WM_SETICON, ICON_SMALL
	}
}

func desktopSetTaskbarIcon(hwnd uintptr, iconIndex int, dispatch func(func())) {
	if hwnd == 0 || iconIndex < 0 || iconIndex > 5 {
		return
	}
	hinstance, _, _ := windowGetModuleHandle.Call(0)
	width, height := desktopWindowIconSize(hwnd, 11, 12, 32) // SM_CXICON/CYICON
	icon, _, _ := iconLoadImage.Call(hinstance, uintptr(101+iconIndex), 1, width, height, 0x8000)
	if icon != 0 {
		desktopCurrentTaskbarIcon.Store(int32(iconIndex))
		desktopSetTaskbarRelaunchIcon(hwnd, iconIndex)
		// Clear first so Explorer does not keep displaying the previous cached
		// taskbar image when the window remains visible during a theme change.
		iconSendMessage.Call(hwnd, 0x80, 1, 0) // WM_SETICON, ICON_BIG
		iconSendMessage.Call(hwnd, 0x80, 1, icon)
		iconRedrawWindow.Call(hwnd, 0, 0, 0x0400|0x0100|0x0080) // RDW_FRAME | RDW_UPDATENOW | RDW_INVALIDATE
		change := desktopTaskbarIconChange.Add(1)
		desktopRefreshTaskbarButton(hwnd, false)
		// Explorer processes taskbar changes asynchronously. Add the button on a
		// later UI turn so it cannot reuse the entry that was just removed.
		time.AfterFunc(150*time.Millisecond, func() {
			dispatch(func() {
				if desktopTaskbarIconChange.Load() == change {
					desktopRefreshTaskbarButton(hwnd, true)
				}
			})
		})
	}
}

func desktopReapplyTaskbarIcon(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	iconIndex := int(desktopCurrentTaskbarIcon.Load())
	if iconIndex < 0 || iconIndex > 5 {
		return
	}
	hinstance, _, _ := windowGetModuleHandle.Call(0)
	width, height := desktopWindowIconSize(hwnd, 11, 12, 32)
	icon, _, _ := iconLoadImage.Call(hinstance, uintptr(101+iconIndex), 1, width, height, 0x8000)
	if icon != 0 {
		iconSendMessage.Call(hwnd, 0x80, 1, icon) // WM_SETICON, ICON_BIG
		iconRedrawWindow.Call(hwnd, 0, 0, 0x0400|0x0100|0x0080)
	}
}

func desktopSetTaskbarAppID() {
	appID, _ := syscall.UTF16PtrFromString("DMSHLabs.DictShelf")
	if appID != nil {
		iconSetProcessAppID.Call(uintptr(unsafe.Pointer(appID)))
		runtime.KeepAlive(appID)
	}
}

func desktopSetTaskbarRelaunchIcon(hwnd uintptr, iconIndex int) {
	if hwnd == 0 || iconIndex < 0 || iconIndex > 5 {
		return
	}
	iconPath, err := desktopTaskbarICOPath(iconIndex)
	if err != nil {
		return
	}
	resource, err := syscall.UTF16PtrFromString(iconPath + ",0")
	if err != nil {
		return
	}
	iid := desktopKnownFolderID{Data1: 0x886D8EEB, Data2: 0x8CF2, Data3: 0x4446, Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	var store uintptr
	hr, _, _ := iconGetWindowStore.Call(hwnd, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&store)))
	if int32(hr) < 0 || store == 0 {
		return
	}
	defer desktopCOMCall(store, 2) // IUnknown.Release
	key := desktopPropertyKey{
		FormatID:   desktopKnownFolderID{Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39, Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}},
		PropertyID: 3, // PKEY_AppUserModel_RelaunchIconResource
	}
	value := desktopPropVariant{Type: 31, Value: uintptr(unsafe.Pointer(resource))}               // VT_LPWSTR
	hr = desktopCOMCall(store, 6, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&value))) // IPropertyStore.SetValue
	if int32(hr) < 0 {
		return
	}
	runtime.KeepAlive(resource)
	runtime.KeepAlive(&value)
	runtime.KeepAlive(&key)
	// Setting the window's AppUserModelID after its relaunch icon tells the
	// taskbar to refresh the identity and relaunch metadata for this button.
	appID, err := syscall.UTF16PtrFromString("DMSHLabs.DictShelf")
	if err != nil {
		return
	}
	appIDKey := desktopPropertyKey{FormatID: key.FormatID, PropertyID: 5} // PKEY_AppUserModel_ID
	appIDValue := desktopPropVariant{Type: 31, Value: uintptr(unsafe.Pointer(appID))}
	hr = desktopCOMCall(store, 6, uintptr(unsafe.Pointer(&appIDKey)), uintptr(unsafe.Pointer(&appIDValue)))
	if int32(hr) >= 0 {
		desktopCOMCall(store, 7) // IPropertyStore.Commit
	}
	runtime.KeepAlive(appID)
	runtime.KeepAlive(&appIDValue)
	runtime.KeepAlive(&appIDKey)
}

func desktopRefreshTaskbarButton(hwnd uintptr, add bool) {
	// Re-register the visible window's taskbar button so Explorer reads its
	// new ICON_BIG without hiding or minimizing the application window.
	initResult, _, _ := iconCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
	if int32(initResult) >= 0 {
		defer iconCoUninitialize.Call()
	} else if uint32(initResult) != 0x80010106 { // RPC_E_CHANGED_MODE
		return
	}
	clsid := desktopKnownFolderID{Data1: 0x56FDF344, Data2: 0xFD6D, Data3: 0x11D0, Data4: [8]byte{0x95, 0x8A, 0, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iid := desktopKnownFolderID{Data1: 0x56FDF342, Data2: 0xFD6D, Data3: 0x11D0, Data4: [8]byte{0x95, 0x8A, 0, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	var taskbar uintptr
	hr, _, _ := iconCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, 1,
		uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&taskbar)))
	if int32(hr) < 0 || taskbar == 0 {
		return
	}
	defer desktopCOMCall(taskbar, 2)           // Release
	if int32(desktopCOMCall(taskbar, 3)) < 0 { // ITaskbarList.HrInit
		return
	}
	if add {
		desktopCOMCall(taskbar, 4, hwnd) // AddTab
	} else {
		desktopCOMCall(taskbar, 5, hwnd) // DeleteTab
	}
}

func desktopWindowIconSize(hwnd uintptr, metricW, metricH, fallback int) (uintptr, uintptr) {
	width, height := uintptr(0), uintptr(0)
	var dpi uintptr
	if iconGetDPIForWindow.Find() == nil {
		dpi, _, _ = iconGetDPIForWindow.Call(hwnd)
	}
	if dpi != 0 && iconGetSystemMetricsDPI.Find() == nil {
		width, _, _ = iconGetSystemMetricsDPI.Call(uintptr(metricW), dpi)
		height, _, _ = iconGetSystemMetricsDPI.Call(uintptr(metricH), dpi)
	}
	if width == 0 {
		width, _, _ = iconGetSystemMetrics.Call(uintptr(metricW))
	}
	if height == 0 {
		height, _, _ = iconGetSystemMetrics.Call(uintptr(metricH))
	}
	if width == 0 {
		width = uintptr(fallback)
	}
	if height == 0 {
		height = uintptr(fallback)
	}
	return width, height
}

// desktopUpdateShortcutIcon updates the DictShelf shortcut when Setup created
// one. The icon index is the resource order in DictShelf.exe (101 through 106).
func desktopUpdateShortcutIcon(iconIndex int) {
	desktopUpdateShortcuts(iconIndex, -1)
}

func desktopUpdateTaskbarShortcutIcon(iconIndex int) {
	desktopUpdateShortcuts(-1, iconIndex)
}

func desktopRefreshTaskbarShortcutIcon(hwnd uintptr) {
	iconIndex := int(desktopCurrentTaskbarIcon.Load())
	if name, ok := tray.WindowsTaskbarIconName(); ok {
		if current, found := tray.WindowsIconIndex(name); found {
			iconIndex = current
		}
	}
	if iconIndex >= 0 && iconIndex <= 5 {
		desktopSetTaskbarRelaunchIcon(hwnd, iconIndex)
		desktopUpdateTaskbarShortcutIcon(iconIndex)
		desktopScheduleTaskbarShortcutRefresh(iconIndex)
	}
}

func desktopTaskbarICOPath(iconIndex int) (string, error) {
	name, ok := tray.WindowsIconName(iconIndex)
	if !ok {
		return "", fmt.Errorf("unknown taskbar icon index %d", iconIndex)
	}
	darkData, lightData, _, ok := tray.WindowsTaskbarICOPair(name)
	if !ok {
		return "", fmt.Errorf("taskbar icon %q is unavailable", name)
	}
	configPath := config.HomeConfig()
	if configPath == "" {
		return "", fmt.Errorf("cannot locate DictShelf data folder")
	}
	iconsDir := filepath.Join(filepath.Dir(configPath), "icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		return "", err
	}
	darkPath := filepath.Join(iconsDir, "taskbar-dark.ico")
	lightPath := filepath.Join(iconsDir, "taskbar-light.ico")
	if err := desktopWriteTaskbarICO(darkPath, darkData); err != nil {
		return "", err
	}
	if err := desktopWriteTaskbarICO(lightPath, lightData); err != nil {
		return "", err
	}
	if strings.HasSuffix(name, "_transparent_light") {
		return lightPath, nil
	}
	return darkPath, nil
}

func desktopWriteTaskbarICO(path string, data []byte) error {
	temp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

func desktopPruneTaskbarICOFiles(iconsDir string) {
	entries, err := os.ReadDir(iconsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(strings.ToLower(entry.Name()), "taskbar-") || !strings.EqualFold(filepath.Ext(entry.Name()), ".ico") {
			continue
		}
		if !strings.EqualFold(entry.Name(), "taskbar-dark.ico") && !strings.EqualFold(entry.Name(), "taskbar-light.ico") {
			_ = os.Remove(filepath.Join(iconsDir, entry.Name()))
		}
	}
}

func desktopUpdateShortcuts(appIconIndex, taskbarIconIndex int) {
	if appIconIndex < -1 || appIconIndex > 5 || taskbarIconIndex < -1 || taskbarIconIndex > 5 {
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
	if appData := os.Getenv("APPDATA"); appData != "" {
		pinned := filepath.Join(appData, "Microsoft", "Internet Explorer", "Quick Launch", "User Pinned", "TaskBar")
		if entries, err := os.ReadDir(pinned); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".lnk") {
					candidates = append(candidates, filepath.Join(pinned, entry.Name()))
				}
			}
		}
	}
	if len(candidates) == 0 {
		return
	}
	taskbarIconPath := ""
	if taskbarIconIndex >= 0 {
		var err error
		taskbarIconPath, err = desktopTaskbarICOPath(taskbarIconIndex)
		if err != nil {
			return
		}
	}
	updatedAny := false

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
					isPinned := strings.Contains(strings.ToLower(linkPath), `\user pinned\taskbar\`)
					if isPinned {
						if taskbarIconIndex >= 0 {
							desktopSetShellLinkRelaunchIcon(shellLink, taskbarIconPath)
						}
						desktopSetShellLinkAppID(shellLink)
						if taskbarIconIndex >= 0 {
							// Point Explorer at the standalone ICO so it does not fall back
							// to the first icon resource in DictShelf.exe after exit.
							wideIcon, iconErr := syscall.UTF16PtrFromString(taskbarIconPath)
							if iconErr == nil {
								desktopCOMCall(shellLink, 17, uintptr(unsafe.Pointer(wideIcon)), 0)
								runtime.KeepAlive(wideIcon)
							}
						}
					} else if appIconIndex >= 0 {
						setHR := desktopCOMCall(shellLink, 17, uintptr(unsafe.Pointer(exe)), uintptr(-101-appIconIndex)) // desktop shortcut keeps app icon
						if int32(setHR) >= 0 {
							desktopSetShellLinkAppID(shellLink)
						}
					}
					saveHR := desktopCOMCall(persist, 6, uintptr(unsafe.Pointer(wideLink)), 1) // IPersistFile.Save
					if int32(saveHR) >= 0 {
						iconShellChangeNotify.Call(0x00002000, 0x1005, uintptr(unsafe.Pointer(wideLink)), 0) // SHCNE_UPDATEITEM, SHCNF_PATHW|SHCNF_FLUSH
						updatedAny = true
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
	if updatedAny && taskbarIconIndex >= 0 {
		// The pinned taskbar can retain the previous relaunch image after an
		// app window disappears. Match the full shell refresh used by the
		// standalone taskbar-icon repair script.
		iconShellChangeNotify.Call(0x08000000, 0, 0, 0) // SHCNE_ASSOCCHANGED
		desktopPruneTaskbarICOFiles(filepath.Dir(taskbarIconPath))
	}
}

func desktopSetShellLinkAppID(shellLink uintptr) {
	key := desktopPropertyKey{FormatID: desktopKnownFolderID{Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39, Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}}, PropertyID: 5}
	setShellLinkStringProperty(shellLink, key, "DMSHLabs.DictShelf")
}

func desktopSetShellLinkRelaunchIcon(shellLink uintptr, iconPath string) {
	key := desktopPropertyKey{FormatID: desktopKnownFolderID{Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39, Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}}, PropertyID: 3}
	setShellLinkStringProperty(shellLink, key, iconPath+",0")
}

func setShellLinkStringProperty(shellLink uintptr, key desktopPropertyKey, text string) {
	iid := desktopKnownFolderID{Data1: 0x886D8EEB, Data2: 0x8CF2, Data3: 0x4446, Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	var store uintptr
	hr := desktopCOMCall(shellLink, 0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&store))) // QueryInterface
	if int32(hr) < 0 || store == 0 {
		return
	}
	defer desktopCOMCall(store, 2)
	valuePtr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	value := desktopPropVariant{Type: 31, Value: uintptr(unsafe.Pointer(valuePtr))}               // VT_LPWSTR
	hr = desktopCOMCall(store, 6, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&value))) // SetValue
	if int32(hr) >= 0 {
		desktopCOMCall(store, 7) // Commit
	}
	runtime.KeepAlive(valuePtr)
	runtime.KeepAlive(&value)
	runtime.KeepAlive(&key)
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
