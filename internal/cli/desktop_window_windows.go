// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/tray"
	"golang.org/x/sys/windows/registry"
)

var (
	desktopWindowMu         sync.Mutex
	desktopWindow           webview.WebView
	desktopWindowAddress    string
	desktopWindowStarting   bool
	desktopWindowHidden     atomic.Bool
	desktopWindowRequest    func(context.Context, string, string) (any, error)
	windowDwmapi            = syscall.NewLazyDLL("dwmapi.dll")
	windowUser32            = syscall.NewLazyDLL("user32.dll")
	procShowWindow          = windowUser32.NewProc("ShowWindow")
	procIsIconic            = windowUser32.NewProc("IsIconic")
	windowDwmSetAttribute   = windowDwmapi.NewProc("DwmSetWindowAttribute")
	windowShell32           = syscall.NewLazyDLL("shell32.dll")
	windowBrowseForFolder   = windowShell32.NewProc("SHBrowseForFolderW")
	windowGetPathFromIDList = windowShell32.NewProc("SHGetPathFromIDListW")
	windowComdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	windowGetOpenFileName   = windowComdlg32.NewProc("GetOpenFileNameW")
	windowGetSaveFileName   = windowComdlg32.NewProc("GetSaveFileNameW")
	windowOle32             = syscall.NewLazyDLL("ole32.dll")
	windowCoTaskMemFree     = windowOle32.NewProc("CoTaskMemFree")
	windowSetWindowsHookEx  = windowUser32.NewProc("SetWindowsHookExW")
	windowCallNextHookEx    = windowUser32.NewProc("CallNextHookEx")
	windowUnhookWindowsHook = windowUser32.NewProc("UnhookWindowsHookEx")
	windowOpenClipboard     = windowUser32.NewProc("OpenClipboard")
	windowGetClipboardData  = windowUser32.NewProc("GetClipboardData")
	windowClipboardSequence = windowUser32.NewProc("GetClipboardSequenceNumber")
	windowCloseClipboard    = windowUser32.NewProc("CloseClipboard")
	windowGlobalLock        = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalLock")
	windowGlobalUnlock      = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalUnlock")
	windowGlobalSize        = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalSize")
	windowGetWindowRect     = windowUser32.NewProc("GetWindowRect")
	windowSendInput         = windowUser32.NewProc("SendInput")
	windowGetAsyncKeyState  = windowUser32.NewProc("GetAsyncKeyState")
	windowGetModuleHandle   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

var desktopCloseToTray atomic.Bool
var desktopHotkeyMu sync.Mutex
var desktopHotkeySeq uint32 = 1
var desktopHotkeyActions = map[int]desktopHotkeyAction{}
var desktopHotkeyWindow uintptr
var desktopHotkeyCaptureActive bool
var desktopKeyboardHook uintptr
var desktopKeyboardHookReady bool
var desktopHookCallback = syscall.NewCallback(desktopKeyboardHookProc)
var desktopSwallowedKeysMu sync.Mutex
var desktopSwallowedKeyUps = map[uint32]bool{}
var desktopWindowSaveMu sync.Mutex
var desktopWindowSaveTimer *time.Timer

type desktopWindowRect struct{ Left, Top, Right, Bottom int32 }

type desktopHotkeyAction struct {
	sequence  []desktopHotkeyChord
	action    int
	id        int
	waiting   bool
	waitToken uint64
	copySeq   uint32
}

type desktopHotkeyChord struct {
	key  uint32
	mods uint32
}

type desktopKeyboardInput struct {
	Vk, Scan    uint16
	Flags, Time uint32
	ExtraInfo   uintptr
}

type desktopInput struct {
	Type uint32
	_    uint32 // alignment for the INPUT union on 64-bit Windows
	Ki   desktopKeyboardInput
	Pad  [8]byte // INPUT's union is the size of MOUSEINPUT on 64-bit Windows
}

// openDesktopWindow opens the local reader on a dedicated Windows UI thread.
// The tray keeps the main thread; WebView2 owns the locked thread below.
func openDesktopWindow(address, appAddress string, onClose func(), desktopRequest func(context.Context, string, string) (any, error), desktopStream func(context.Context, string, string, func([]byte)) error, desktopResource desktopResourceHandler, desktopOnly bool) bool {
	version, err := webviewloader.GetInstalledVersion()
	if err != nil || version == "" {
		if desktopOnly {
			logx.Warn("WebView2 unavailable in listenerless desktop mode: %v", err)
		} else {
			logx.Warn("WebView2 unavailable; opening the browser: %v", err)
		}
		return false
	}
	profile := os.Getenv("LOCALAPPDATA")
	if profile == "" {
		profile, err = os.UserCacheDir()
		if err != nil {
			return false
		}
	}
	profile = filepath.Join(profile, "DMSH Labs", "DictShelf", "WebView2")
	if root := os.Getenv(windowsPortableHomeEnv); root != "" {
		profile = filepath.Join(root, "WebView2")
	}
	if err := os.MkdirAll(profile, 0700); err != nil {
		logx.Warn("WebView2 profile unavailable: %v", err)
		return false
	}
	desktopWindowMu.Lock()
	if desktopWindow != nil {
		w := desktopWindow
		desktopWindowMu.Unlock()
		w.Dispatch(func() {
			restoreAndFocusWindow(uintptr(w.Window()))
			desktopWindowHidden.Store(false)
		})
		return true
	}
	if desktopWindowStarting {
		desktopWindowMu.Unlock()
		return true
	}
	desktopCloseToTray.Store(true)
	desktopWindowRequest = desktopRequest
	webview.SetCloseToTrayHook(func(hwnd uintptr) bool {
		if desktopWindowHidden.Load() || !desktopCloseToTray.Load() {
			return desktopWindowHidden.Load()
		}
		const swHide = 0
		procShowWindow.Call(hwnd, swHide)
		desktopWindowHidden.Store(true)
		return true
	})
	webview.SetWindowSizeHandler(func(hwnd uintptr, width, height int) {
		if width < 480 || height < 320 {
			return
		}
		desktopWindowSaveMu.Lock()
		if desktopWindowSaveTimer != nil {
			desktopWindowSaveTimer.Stop()
		}
		desktopWindowSaveTimer = time.AfterFunc(500*time.Millisecond, func() {
			var rect desktopWindowRect
			if ok, _, _ := windowGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
				return
			}
			payload, _ := json.Marshal(map[string]int{"width": int(rect.Right - rect.Left), "height": int(rect.Bottom - rect.Top)})
			desktopWindowMu.Lock()
			request := desktopWindowRequest
			desktopWindowMu.Unlock()
			if request != nil {
				if _, err := request(context.Background(), "setDesktopWindowSize", string(payload)); err != nil {
					logx.Warn("could not save desktop window size: %v", err)
				}
			}
		})
		desktopWindowSaveMu.Unlock()
	})
	desktopWindowStarting = true
	desktopWindowMu.Unlock()
	go runDesktopWindow(address, appAddress, profile, onClose, desktopRequest, desktopStream, desktopResource, desktopOnly)
	return true
}

func runDesktopWindow(address, appAddress, profile string, onClose func(), desktopRequest func(context.Context, string, string) (any, error), desktopStream func(context.Context, string, string, func([]byte)) error, desktopResource desktopResourceHandler, desktopOnly bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("WebView2 panic: %v\n%s", recovered, debug.Stack())
			panic(recovered)
		}
	}()
	log.Printf("WebView2 startup: creating window; profile=%q", profile)
	initialColor, customCaptionColor := desktopInitialWindowColors(desktopRequest)
	// Win32's CW_USEDEFAULT lets Windows choose the initial size and position.
	// The WebView wrapper substitutes 640×480 for zero, so pass the Win32
	// sentinel explicitly until a size has been saved.
	const windowUseDefault = uint(0x80000000)
	windowWidth, windowHeight := windowUseDefault, windowUseDefault
	windowCenter := false
	if desktopRequest != nil {
		if value, err := desktopRequest(context.Background(), "desktopWindowSize", ""); err == nil {
			if size, ok := value.(map[string]any); ok {
				width, widthOK := size["width"].(float64)
				height, heightOK := size["height"].(float64)
				if widthOK && heightOK && width >= 480 && width <= 10000 && height >= 320 && height <= 10000 {
					windowWidth, windowHeight = uint(width), uint(height)
					windowCenter = true
				}
			}
		}
	}
	webviewStarted := time.Now()
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath:  profile,
		AutoFocus: true,
		WindowOptions: webview.WindowOptions{Title: "DictShelf", Width: windowWidth, Height: windowHeight, IconId: 101, Center: windowCenter,
			BackgroundColor: initialColor, CaptionColor: customCaptionColor, StartupSplash: true},
	})
	if w == nil {
		log.Printf("WebView2 startup: window creation returned nil")
		desktopWindowMu.Lock()
		desktopWindowStarting = false
		desktopWindowMu.Unlock()
		if desktopOnly {
			logx.Warn("WebView2 window failed in listenerless desktop mode")
			tray.Alert("DictShelf", "WebView2 could not create the application window.")
			if onClose != nil {
				onClose()
			}
		} else if address != "" {
			logx.Warn("WebView2 window failed; opening the browser")
			browserCmd(address)
		}
		return
	}
	if !desktopInstallKeyboardHook() {
		logx.Warn("could not install the layout-independent global hot-key listener")
	}
	bindWindowsUpdater(w)
	log.Printf("WebView2 startup: window and runtime initialized in %s", time.Since(webviewStarted).Round(time.Millisecond))
	w.Init(`window.__wudictDocumentFirstScriptMs=performance.now()`)
	_ = w.Bind("wudictGetCloseToTray", func() bool {
		if desktopRequest == nil {
			return true
		}
		value, err := desktopRequest(context.Background(), "storageRead", "")
		if err != nil {
			return true
		}
		values, ok := value.(map[string]string)
		enabled := !ok || values["wudict_close_to_tray"] != "0"
		desktopCloseToTray.Store(enabled)
		return enabled
	})
	_ = w.Bind("wudictSetCloseToTray", func(enabled bool) error {
		desktopCloseToTray.Store(enabled)
		return nil
	})
	_ = w.Bind("wudictGetHotkeys", func() any {
		if desktopRequest == nil {
			return map[string]string{"show": "Ctrl+Shift+F11, Ctrl+Shift+F11", "translate": "Ctrl+Shift+B, Ctrl+Shift+B"}
		}
		value, err := desktopRequest(context.Background(), "desktopHotkeys", "")
		if err != nil {
			return map[string]string{"show": "Ctrl+Shift+F11, Ctrl+Shift+F11", "translate": "Ctrl+Shift+B, Ctrl+Shift+B"}
		}
		return value
	})
	_ = w.Bind("wudictSetHotkeys", func(kind, sequence string) bool {
		return desktopSetHotkeys(uintptr(w.Window()), kind, sequence)
	})
	_ = w.Bind("wudictSetHotkeyCapture", func(enabled bool) {
		desktopSetHotkeyCapture(uintptr(w.Window()), enabled)
	})
	desktopApplySavedHotkeys(uintptr(w.Window()), desktopRequest)
	_ = w.Bind("wudictHideWindow", func() error {
		w.Dispatch(func() {
			const swHide = 0
			procShowWindow.Call(uintptr(w.Window()), swHide)
			desktopWindowHidden.Store(true)
		})
		return nil
	})
	w.Init(`(function(){function setup(){if(window.chrome&&window.chrome.webview){window.chrome.webview.addEventListener('windowclose',function(event){if(window.wudictGetCloseToTray&&window.wudictGetCloseToTray()){event.preventDefault();window.wudictHideWindow()}})}}if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',setup,{once:true});else setup()})();`)
	if err := w.Bind("wudictStartupReady", func() error {
		log.Printf("WebView2 startup: application ready")
		w.Dispatch(func() { webview.HideStartupSplash(w) })
		return nil
	}); err != nil {
		log.Printf("WebView2 startup: could not bind splash transition: %v", err)
	}
	if err := w.Bind("wudictStartupStatus", func(message string) error {
		w.Dispatch(func() { webview.SetStartupSplashText(w, message) })
		return nil
	}); err != nil {
		log.Printf("WebView2 startup: could not bind splash status: %v", err)
	}
	time.AfterFunc(2*time.Minute, func() {
		w.Dispatch(func() { webview.HideStartupSplash(w) })
	})
	local, _ := url.Parse(address)
	app, _ := url.Parse(appAddress)
	log.Printf("WebView2 startup: parsed app and loopback origins")
	navigationAddress := address
	virtualOrigin := false
	log.Printf("WebView2 startup: preparing in-process origin")
	if desktopResource != nil && local != nil && app != nil {
		if resourceView, ok := w.(webview.WebResourceView); ok {
			filter := local.Scheme + "://" + local.Host + "/*"
			filter += "|" + app.Scheme + "://" + app.Host + "/*"
			filter += "|" + legacyDesktopWebViewOrigin + "/*"
			if err := resourceView.SetWebResourceRequestedHandler(filter, webview.WebResourceHandler(desktopResource)); err != nil {
				logx.Warn("WebView2 resource interception unavailable; using the loopback page: %v", err)
			} else {
				navigationAddress, virtualOrigin = appAddress, true
				log.Printf("WebView2 startup: in-process origin enabled; filter=%q", filter)
			}
		} else {
			logx.Warn("WebView2 resource interception unavailable; using the loopback page")
		}
	}
	if desktopOnly && !virtualOrigin {
		logx.Warn("WebView2 in-process resource handling is unavailable; cannot start without a listener")
		w.Destroy()
		desktopWindowMu.Lock()
		desktopWindowStarting = false
		desktopWindowMu.Unlock()
		tray.Alert("DictShelf", "The in-app WebView cannot serve application pages directly.")
		if onClose != nil {
			onClose()
		}
		return
	}
	log.Printf("WebView2 startup: in-process origin enabled=%t; initial target=%q", virtualOrigin, navigationAddress)
	if desktopRequest != nil || desktopStream != nil {
		baseURL := local.String()
		if virtualOrigin {
			baseURL = app.String()
		}
		bindNativeBridge(w, baseURL, desktopRequest, desktopStream)
	}
	var folderCallbackPending atomic.Bool
	_ = w.Bind("wudictChooseFolder", func() error {
		if !folderCallbackPending.CompareAndSwap(false, true) {
			return nil
		}
		w.Dispatch(func() {
			path, ok := chooseDesktopFolder(uintptr(w.Window()))
			if ok {
				w.Eval("window.postMessage({type:'wudict-folder-picked',path:" + jsString(path) + "},location.origin)")
			}
			folderCallbackPending.Store(false)
		})
		return nil
	})
	var archiveCallbackPending atomic.Bool
	_ = w.Bind("wudictChooseArchive", func() error {
		if !archiveCallbackPending.CompareAndSwap(false, true) {
			return nil
		}
		w.Dispatch(func() {
			path, ok := chooseDesktopArchive(uintptr(w.Window()))
			if ok {
				w.Eval("window.postMessage({type:'wudict-archive-picked',path:" + jsString(path) + "},location.origin)")
			}
			archiveCallbackPending.Store(false)
		})
		return nil
	})
	_ = w.Bind("wudictOpenExternal", func(raw string) error {
		link, err := url.Parse(raw)
		if err != nil || (link.Scheme != "http" && link.Scheme != "https") || link.Host == "" {
			return fmt.Errorf("invalid external link")
		}
		if local != nil && strings.EqualFold(link.Host, local.Host) || virtualOrigin && app != nil && strings.EqualFold(link.Host, app.Host) {
			return nil
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", link.String()).Start()
	})
	_ = w.Bind("wudictSetCaptionColor", func(color string) error {
		setDesktopCaptionColor(uintptr(w.Window()), color)
		return nil
	})
	w.Init(`window.__wudictDocumentScriptStartMs=performance.now();document.addEventListener('click',function(event){
  var a=event.target&&event.target.closest&&event.target.closest('a[href]');
  if(!a)return;
  try { var u=new URL(a.href); if((u.protocol==='http:'||u.protocol==='https:')&&u.origin!==location.origin){
    event.preventDefault();event.stopPropagation();window.wudictOpenExternal(u.href);
  }}catch(_){ }
},true);`)
	done := make(chan struct{})
	initialNavigationAddress := navigationAddress
	storageMigrationPending := false
	if virtualOrigin && local != nil && app != nil {
		storageMarker := filepath.Join(profile, "virtual-origin-storage-v1")
		settingsExist := false
		if desktopRequest != nil {
			if value, err := desktopRequest(context.Background(), "storageState", ""); err == nil {
				if state, ok := value.(map[string]bool); ok {
					settingsExist = state["exists"]
				}
			} else {
				logx.Warn("settings file status unavailable: %v", err)
			}
		}
		_ = w.Bind("wudictStorageSeeded", func() error {
			if err := os.WriteFile(storageMarker, []byte("ok\n"), 0600); err != nil {
				logx.Warn("WebView2 storage migration marker failed: %v", err)
				return err
			}
			return nil
		})
		if !settingsExist {
			storageMigrationPending = true
			var migrationFinished atomic.Bool
			var migrationImporting atomic.Bool
			finishMigration := func(imported bool, reason string) {
				if !migrationFinished.CompareAndSwap(false, true) {
					return
				}
				if reason != "" {
					logx.Warn("settings migration %s; continuing with the application", reason)
					tray.Alert("DictShelf", "Saved settings could not be transferred. The app will start with default settings; you can change them later.")
				}
				w.Dispatch(func() {
					if imported {
						log.Printf("WebView2 storage migration: opening application after import")
					} else {
						log.Printf("WebView2 storage migration: opening application without imported settings")
					}
					desktopWindowMu.Lock()
					desktopWindowAddress = appAddress
					desktopWindowMu.Unlock()
					w.Navigate(appAddress)
				})
			}
			startVirtualApp := func(entries string) {
				if migrationFinished.Load() || !migrationImporting.CompareAndSwap(false, true) {
					return
				}
				log.Printf("WebView2 storage migration: importing exported settings; bytes=%d", len(entries))
				if len(entries) > 8<<20 {
					finishMigration(false, "export is too large")
					return
				}
				if desktopRequest == nil {
					finishMigration(false, "direct storage bridge is unavailable")
					return
				}
				w.Dispatch(func() {
					w.Eval(`if(window.wudictMigrationStatus)window.wudictMigrationStatus("Saving your settings…")`)
				})
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					if _, err := desktopRequest(ctx, "storageImport", entries); err != nil {
						logx.Warn("legacy settings import failed: %v", err)
						finishMigration(false, "could not save the exported settings")
						return
					}
					finishMigration(true, "")
				}()
			}
			_ = w.Bind("wudictExportStorage", func(entries string) error {
				log.Printf("WebView2 storage migration: received profile storage; bytes=%d", len(entries))
				startVirtualApp(entries)
				return nil
			})
			oldOrigin := local.Scheme + "://" + local.Host
			markerExists := false
			if _, err := os.Stat(storageMarker); err == nil {
				markerExists = true
			}
			exportOrigin := oldOrigin
			if markerExists {
				exportOrigin = legacyDesktopWebViewOrigin
			}
			w.Init(desktopStorageExportScript(exportOrigin))
			if !markerExists {
				log.Printf("WebView2 storage migration: importing from old loopback origin %q", exportOrigin)
			} else {
				log.Printf("WebView2 storage migration: importing from previous virtual origin %q", exportOrigin)
			}
			time.AfterFunc(2*time.Minute, func() {
				finishMigration(false, "timed out after two minutes")
			})
			migrationURL, err := url.Parse(exportOrigin + "/__storage-migration")
			if err != nil {
				finishMigration(false, "could not prepare migration screen")
			} else {
				query := migrationURL.Query()
				query.Set("wudict_webview2", "1")
				migrationURL.RawQuery = query.Encode()
				initialNavigationAddress = migrationURL.String()
			}
		}
	}
	if storageMigrationPending {
		webview.HideStartupSplash(w)
	}
	log.Printf("WebView2 startup: navigating to %q; localStorage migration=%t", initialNavigationAddress, storageMigrationPending)
	desktopWindowMu.Lock()
	desktopWindow = w
	desktopWindowAddress = initialNavigationAddress
	desktopWindowStarting = false
	desktopWindowMu.Unlock()
	w.Navigate(initialNavigationAddress)
	log.Printf("WebView2 startup: navigation submitted")
	if !desktopOnly {
		go watchDesktopServer(w, local, done)
	}
	w.Run()
	log.Printf("WebView2 window loop returned")
	desktopUninstallKeyboardHook()
	close(done)
	w.Destroy()
	desktopWindowMu.Lock()
	desktopWindow = nil
	desktopWindowAddress = ""
	desktopWindowMu.Unlock()
	if onClose != nil {
		onClose()
	}
}

type desktopKeyboardEvent struct {
	VirtualKey uint32
	ScanCode   uint32
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

func desktopInstallKeyboardHook() bool {
	if desktopKeyboardHook != 0 {
		return true
	}
	module, _, _ := windowGetModuleHandle.Call(0)
	hook, _, _ := windowSetWindowsHookEx.Call(13, desktopHookCallback, module, 0)
	if hook == 0 {
		return false
	}
	desktopKeyboardHook = hook
	desktopKeyboardHookReady = true
	return true
}

func desktopUninstallKeyboardHook() {
	if desktopKeyboardHook != 0 {
		windowUnhookWindowsHook.Call(desktopKeyboardHook)
		desktopKeyboardHook = 0
	}
	desktopKeyboardHookReady = false
}

func desktopKeyboardHookProc(code int, message, data uintptr) uintptr {
	if code < 0 {
		result, _, _ := windowCallNextHookEx.Call(0, uintptr(code), message, data)
		return result
	}
	const (
		wmKeyDown     = 0x0100
		wmKeyUp       = 0x0101
		wmSysKeyDown  = 0x0104
		wmSysKeyUp    = 0x0105
		llkhfExtended = 0x01
		llkhfInjected = 0x10
		llkhfUp       = 0x80
	)
	if data == 0 {
		return 0
	}
	event := *(*desktopKeyboardEvent)(unsafe.Pointer(data))
	if event.Flags&llkhfInjected != 0 {
		result, _, _ := windowCallNextHookEx.Call(0, uintptr(code), message, data)
		return result
	}
	scanCode := event.ScanCode
	if event.Flags&llkhfExtended != 0 {
		scanCode |= 0x100
	}
	if message == wmKeyUp || message == wmSysKeyUp || event.Flags&llkhfUp != 0 {
		desktopSwallowedKeysMu.Lock()
		swallow := desktopSwallowedKeyUps[scanCode]
		delete(desktopSwallowedKeyUps, scanCode)
		desktopSwallowedKeysMu.Unlock()
		if swallow {
			return 1
		}
		result, _, _ := windowCallNextHookEx.Call(0, uintptr(code), message, data)
		return result
	}
	if message != wmKeyDown && message != wmSysKeyDown {
		result, _, _ := windowCallNextHookEx.Call(0, uintptr(code), message, data)
		return result
	}
	desktopSwallowedKeysMu.Lock()
	if desktopSwallowedKeyUps[scanCode] {
		desktopSwallowedKeysMu.Unlock()
		return 1
	}
	desktopSwallowedKeysMu.Unlock()
	desktopHotkeyMu.Lock()
	if desktopHotkeyCaptureActive || !desktopKeyboardHookReady {
		desktopHotkeyMu.Unlock()
		result, _, _ := windowCallNextHookEx.Call(0, uintptr(code), message, data)
		return result
	}
	mods := desktopCurrentModifierMask()
	var hotkeyID int
	for _, action := range desktopHotkeyActions {
		if len(action.sequence) == 0 {
			continue
		}
		first := action.sequence[0]
		if first.key == scanCode && first.mods == mods {
			hotkeyID = action.id
			break
		}
		if len(action.sequence) == 2 && action.waiting {
			second := action.sequence[1]
			if second.key == scanCode && second.mods == mods {
				hotkeyID = action.id + 1
				break
			}
		}
	}
	hwnd := desktopHotkeyWindow
	desktopHotkeyMu.Unlock()
	if hotkeyID == 0 {
		result, _, _ := windowCallNextHookEx.Call(0, uintptr(code), message, data)
		return result
	}
	desktopSwallowedKeysMu.Lock()
	desktopSwallowedKeyUps[scanCode] = true
	desktopSwallowedKeysMu.Unlock()
	desktopHandleHotkey(hwnd, uintptr(hotkeyID), 0)
	return 1
}

func desktopCurrentModifierMask() uint32 {
	var mods uint32
	for _, key := range []struct{ flag, vk uint32 }{{0x0002, 0x11}, {0x0004, 0x10}, {0x0001, 0x12}} {
		state, _, _ := windowGetAsyncKeyState.Call(uintptr(key.vk))
		if int16(state) < 0 {
			mods |= key.flag
		}
	}
	for _, vk := range []uint32{0x5B, 0x5C} {
		state, _, _ := windowGetAsyncKeyState.Call(uintptr(vk))
		if int16(state) < 0 {
			mods |= 0x0008
			break
		}
	}
	return mods
}

// desktopInitialWindowColors reads the durable appearance before WebView2
// creates a visible window. This lets the native window and WebView blank
// surface use the same paper while the runtime and first page initialize.
func desktopInitialWindowColors(request func(context.Context, string, string) (any, error)) (background, caption string) {
	dark := false
	if request != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if value, err := request(ctx, "storageRead", ""); err == nil {
			if values, ok := value.(map[string]string); ok {
				switch values["wudict_theme"] {
				case "dark":
					dark = true
				case "light":
					dark = false
				case "auto":
					dark = windowsPrefersDark()
				}
				key := "wudict_desktop_appearance_light"
				if dark {
					key = "wudict_desktop_appearance_dark"
				}
				var appearance struct {
					ColorEnabled bool   `json:"colorEnabled"`
					Color        string `json:"color"`
					Image        string `json:"image"`
				}
				if json.Unmarshal([]byte(values[key]), &appearance) == nil &&
					(appearance.Image != "" || appearance.ColorEnabled) && validWindowHexColor(appearance.Color) {
					return strings.ToUpper(appearance.Color), strings.ToUpper(appearance.Color)
				}
			}
		}
	}
	if dark {
		return "#1C1A17", "#1C1A17"
	}
	return "#F4ECD8", "#F4ECD8"
}

func validWindowHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 24)
	return err == nil
}

func windowsPrefersDark() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	return err == nil && value == 0
}

// setDesktopCaptionColor applies the app's chosen window paper to the native
// Windows caption. DWM ignores these attributes on Windows versions that do
// not support caption colors, so failure is intentionally non-fatal.
func setDesktopCaptionColor(hwnd uintptr, color string) {
	if hwnd == 0 {
		return
	}
	caption, text := uint32(0xFFFFFFFF), uint32(0xFFFFFFFF) // DWMWA_COLOR_DEFAULT
	if len(color) == 7 && color[0] == '#' {
		var r, g, b uint32
		if _, err := fmt.Sscanf(color[1:], "%02x%02x%02x", &r, &g, &b); err == nil {
			// COLORREF is 0x00BBGGRR, unlike CSS's #RRGGBB.
			caption = r | g<<8 | b<<16
			if 299*r+587*g+114*b >= 128000 {
				text = 0
			} else {
				text = 0x00FFFFFF
			}
		}
	}
	_, _, _ = windowDwmSetAttribute.Call(hwnd, 35, uintptr(unsafe.Pointer(&caption)), unsafe.Sizeof(caption)) // DWMWA_CAPTION_COLOR
	_, _, _ = windowDwmSetAttribute.Call(hwnd, 36, uintptr(unsafe.Pointer(&text)), unsafe.Sizeof(text))       // DWMWA_TEXT_COLOR
}

func desktopStorageExportScript(origin string) string {
	return `(function(){if(location.origin!==` + jsString(origin) + `||location.pathname!=="/__storage-migration"||window!==window.top)return;window.__wudictMigrationExporting=true;function exportStorage(){var attempts=0;function send(){if(typeof window.wudictExportStorage==="function"){if(window.wudictMigrationStatus)window.wudictMigrationStatus("Transferring settings and search history…");try{var rows=[];for(var i=0;i<localStorage.length;i++){var key=localStorage.key(i);rows.push([key,localStorage.getItem(key)])}window.wudictExportStorage(JSON.stringify(rows))}catch(_){window.wudictExportStorage("[]")}return}if(++attempts<1200){if(window.wudictMigrationStatus)window.wudictMigrationStatus("Connecting to the settings store…");setTimeout(send,100)}}send()}if(document.readyState==="loading")document.addEventListener("DOMContentLoaded",exportStorage,{once:true});else exportStorage()})();`
}

type nativeCall struct{ cancel context.CancelFunc }

var nativeCallsMu sync.Mutex
var nativeCalls = map[int]nativeCall{}

func bindNativeBridge(w webview.WebView, baseURL string, request func(context.Context, string, string) (any, error), stream func(context.Context, string, string, func([]byte)) error) {
	if err := w.Bind("wudictNativeCancel", func(id int) error {
		nativeCallsMu.Lock()
		call, ok := nativeCalls[id]
		delete(nativeCalls, id)
		nativeCallsMu.Unlock()
		if ok {
			call.cancel()
		}
		return nil
	}); err != nil {
		logx.Warn("WebView2 native cancel binding failed: %v", err)
		return
	}
	if err := w.Bind("wudictNativeRequest", func(id int, method, argument string) error {
		if id <= 0 {
			return fmt.Errorf("invalid native request id")
		}
		if method == "clearBrowserCache" {
			cache, ok := w.(webview.BrowserCacheView)
			if !ok {
				return fmt.Errorf("WebView2 cache clearing is unavailable")
			}
			reply := func(err error) {
				valueJSON := "null"
				if err == nil {
					valueJSON = `{"cleared":true}`
				}
				errorJSON, _ := json.Marshal(errorText(err))
				w.Dispatch(func() {
					w.Eval("window.wudictNativeReply(" + strconv.Itoa(id) + "," + valueJSON + "," + string(errorJSON) + ",true)")
				})
			}
			w.Dispatch(func() {
				if err := cache.ClearBrowserCache(reply); err != nil {
					reply(err)
				}
			})
			return nil
		}
		if method == "saveArticle" {
			var request struct {
				Filename  string            `json:"filename"`
				Content   string            `json:"content"`
				Mode      string            `json:"mode"`
				Resources map[string]string `json:"resources"`
			}
			if err := json.Unmarshal([]byte(argument), &request); err != nil || len(argument) > 128*1024*1024 ||
				len(request.Content) == 0 || len(request.Content) > 64*1024*1024 ||
				(request.Mode != "html" && request.Mode != "complete" && request.Mode != "single") || filepath.Base(request.Filename) != request.Filename || !strings.HasSuffix(strings.ToLower(request.Filename), ".html") {
				return fmt.Errorf("invalid article export")
			}
			go func() {
				path, ok := chooseArticleSavePath(uintptr(w.Window()), request.Filename)
				var value any
				var err error
				if ok {
					err = os.WriteFile(path, []byte(request.Content), 0600)
					if err == nil && request.Mode == "complete" && len(request.Resources) > 0 {
						folder := strings.TrimSuffix(path, filepath.Ext(path)) + "_files"
						err = os.MkdirAll(folder, 0700)
						for name, encoded := range request.Resources {
							if err != nil {
								break
							}
							comma := strings.IndexByte(encoded, ',')
							if comma < 0 || filepath.Base(name) != name {
								err = fmt.Errorf("invalid article resource")
								break
							}
							var data []byte
							data, err = base64.StdEncoding.DecodeString(encoded[comma+1:])
							if err == nil {
								err = os.WriteFile(filepath.Join(folder, name), data, 0600)
							}
						}
					}
					if err == nil {
						value = map[string]string{"path": path, "filename": filepath.Base(path)}
					}
				} else {
					err = fmt.Errorf("article save canceled")
				}
				payload, _ := json.Marshal(value)
				errorJSON, _ := json.Marshal(errorText(err))
				w.Dispatch(func() {
					w.Eval("window.wudictNativeReply(" + strconv.Itoa(id) + "," + string(payload) + "," + string(errorJSON) + ",true)")
				})
			}()
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		nativeCallsMu.Lock()
		nativeCalls[id] = nativeCall{cancel: cancel}
		nativeCallsMu.Unlock()
		go func() {
			defer func() { cancel(); nativeCallsMu.Lock(); delete(nativeCalls, id); nativeCallsMu.Unlock() }()
			reply := func(value any, err error, done bool) {
				payload, _ := json.Marshal(value)
				errorJSON, _ := json.Marshal(errorText(err))
				w.Dispatch(func() {
					w.Eval("window.wudictNativeReply(" + strconv.Itoa(id) + "," + string(payload) + "," + string(errorJSON) + "," + strconv.FormatBool(done) + ")")
				})
			}
			if (method == "search" || method == "ingest" || method == "dicts" || method == "rescan" || method == "rescanIndexes" || method == "browse" || method == "browseFind") && stream != nil {
				err := stream(ctx, method, argument, func(message []byte) {
					var value any
					if json.Unmarshal(message, &value) == nil {
						reply(value, nil, false)
					}
				})
				if ctx.Err() != nil {
					return
				}
				reply(nil, err, true)
				return
			}
			if request == nil {
				reply(nil, fmt.Errorf("native request API unavailable"), true)
				return
			}
			value, err := request(ctx, method, argument)
			if ctx.Err() == nil {
				reply(value, err, true)
			}
		}()
		return nil
	}); err != nil {
		logx.Warn("WebView2 native request binding failed: %v", err)
		return
	}
	w.Init(`(function(){function markWebView(){if(document.documentElement)document.documentElement.setAttribute('data-webview2','')}if(document.documentElement)markWebView();else document.addEventListener('DOMContentLoaded',markWebView,{once:true});var next=1,pending=new Map();window.wudictNativeShell=true;window.wudictWindowsShell=true;window.wudictDesktopAppearance=true;window.wudictNativeCall=function(method,argument,onMessage,signal){return new Promise(function(resolve,reject){var id=next++,call={resolve:resolve,reject:reject,onMessage:onMessage};pending.set(id,call);if(signal){if(signal.aborted){pending.delete(id);reject(new DOMException('Aborted','AbortError'));return}signal.addEventListener('abort',function(){pending.delete(id);window.wudictNativeCancel(id);var e=new Error('search cancelled');e.name='AbortError';reject(e)},{once:true})}window.wudictNativeRequest(id,method,argument||'').catch(function(error){window.wudictNativeReply(id,null,String(error),true)})})};window.wudictNativeReply=function(id,value,error,done){var call=pending.get(id);if(!call)return;if(error){pending.delete(id);call.reject(new Error(error));return}if(value!==null&&call.onMessage)call.onMessage(value);if(done){pending.delete(id);call.resolve(value)}};window.addEventListener('pagehide',function(){for(var [id,call] of pending){pending.delete(id);window.wudictNativeCancel(id);call.reject(new Error('page closed'))}})})();`)
	// A restored page can retain an older appearanceRequest implementation.
	// Handle its Android prompt protocol here before WebView2 shows a JS dialog.
	w.Init(`(function(){var prompt=window.prompt;window.prompt=function(message,value){if(message==='wudict:appearance'&&window.wudictWindowsShell===true){try{window.wudictDesktopAppearance=true;var request=JSON.parse(value||'{}');return JSON.stringify(typeof desktopAppearanceRequest==='function'?desktopAppearanceRequest(request):null)}catch(_){return 'null'}}return prompt.apply(window,arguments)}})();`)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func desktopSetHotkeys(hwnd uintptr, kind, value string) bool {
	return desktopSetHotkeysValue(hwnd, kind, value, true)
}

func desktopSetHotkeysValue(hwnd uintptr, kind, value string, persist bool) bool {
	sequence, ok := parseDesktopHotkey(value)
	if !ok || hwnd == 0 || !desktopKeyboardHookReady || kind != "show" && kind != "translate" {
		return false
	}
	desktopHotkeyMu.Lock()
	defer desktopHotkeyMu.Unlock()
	desktopHotkeyWindow = hwnd
	action := 0
	if kind == "translate" {
		action = 1
	}
	old, hadOld := desktopHotkeyActions[action]
	if hadOld && len(old.sequence) == len(sequence) {
		same := true
		for i := range sequence {
			if old.sequence[i] != sequence[i] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	if hadOld {
		delete(desktopHotkeyActions, action)
	}
	id := int(desktopHotkeySeq)
	registered := append([]desktopHotkeyChord(nil), sequence...)
	if current, exists := desktopHotkeyActions[1-action]; exists {
		for _, a := range current.sequence {
			for _, b := range sequence {
				if a == b {
					if hadOld {
						desktopSetHotkeysRestore(hwnd, old)
					}
					return false
				}
			}
		}
	}
	desktopHotkeySeq += uint32(len(sequence))
	desktopHotkeyActions[action] = desktopHotkeyAction{sequence: registered, action: action, id: id}
	if persist && desktopWindowRequest != nil {
		stored := map[string]string{"show": "Ctrl+Shift+F11, Ctrl+Shift+F11", "translate": "Ctrl+Shift+B, Ctrl+Shift+B"}
		if raw, err := desktopWindowRequest(context.Background(), "desktopHotkeys", ""); err == nil {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &stored)
		}
		if kind == "show" {
			stored["show"] = value
		} else {
			stored["translate"] = value
		}
		b, _ := json.Marshal(stored)
		if _, err := desktopWindowRequest(context.Background(), "setDesktopHotkeys", string(b)); err != nil {
			logx.Warn("could not save global hot keys: %v", err)
		}
	}
	return true
}

func desktopApplySavedHotkeys(hwnd uintptr, request func(context.Context, string, string) (any, error)) {
	values := map[string]string{"show": "Ctrl+Shift+F11, Ctrl+Shift+F11", "translate": "Ctrl+Shift+B, Ctrl+Shift+B"}
	if request != nil {
		if raw, err := request(context.Background(), "desktopHotkeys", ""); err == nil {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &values)
		}
	}
	for _, item := range []struct{ kind, key string }{{"show", values["show"]}, {"translate", values["translate"]}} {
		if item.key == "" {
			continue
		}
		sequence, ok := parseDesktopHotkey(item.key)
		if !ok {
			logx.Warn("ignoring invalid saved %s hot key", item.kind)
			continue
		}
		// Defaults are loaded without rewriting state.json; registration is still
		// done through the same conflict checks as a user edit.
		if !desktopRegisterHotkey(hwnd, item.kind, sequence, item.key) {
			logx.Warn("could not register saved %s hot key", item.kind)
		}
	}
}

func desktopRegisterHotkey(hwnd uintptr, kind string, sequence []desktopHotkeyChord, value string) bool {
	return desktopSetHotkeysValue(hwnd, kind, value, false)
}

func desktopSetHotkeysRestore(hwnd uintptr, old desktopHotkeyAction) {
	desktopHotkeyActions[old.action] = old
}

func desktopSetHotkeyCapture(hwnd uintptr, enabled bool) {
	desktopHotkeyMu.Lock()
	defer desktopHotkeyMu.Unlock()
	if desktopHotkeyCaptureActive == enabled {
		return
	}
	desktopHotkeyCaptureActive = enabled
	for _, action := range desktopHotkeyActions {
		if len(action.sequence) == 0 {
			continue
		}
		if enabled {
			action.waiting = false
			action.waitToken++
			desktopHotkeyActions[action.action] = action
		}
	}
}

func parseDesktopHotkey(value string) ([]desktopHotkeyChord, bool) {
	parts := strings.Split(value, ",")
	if len(parts) < 1 || len(parts) > 2 {
		return nil, false
	}
	result := make([]desktopHotkeyChord, 0, len(parts))
	for _, part := range parts {
		var chord desktopHotkeyChord
		for _, token := range strings.Split(strings.TrimSpace(part), "+") {
			switch strings.ToLower(strings.TrimSpace(token)) {
			case "ctrl", "control":
				chord.mods |= 0x0002
			case "alt":
				chord.mods |= 0x0001
			case "shift":
				chord.mods |= 0x0004
			case "win", "meta":
				chord.mods |= 0x0008
			default:
				if chord.key != 0 {
					return nil, false
				}
				chord.key = desktopPhysicalScanCode(strings.TrimSpace(token))
			}
		}
		if chord.key == 0 || chord.mods == 0 {
			return nil, false
		}
		result = append(result, chord)
	}
	return result, true
}

func desktopPhysicalScanCode(key string) uint32 {
	letters := map[byte]uint32{
		'A': 0x1E, 'B': 0x30, 'C': 0x2E, 'D': 0x20, 'E': 0x12, 'F': 0x21,
		'G': 0x22, 'H': 0x23, 'I': 0x17, 'J': 0x24, 'K': 0x25, 'L': 0x26,
		'M': 0x32, 'N': 0x31, 'O': 0x18, 'P': 0x19, 'Q': 0x10, 'R': 0x13,
		'S': 0x1F, 'T': 0x14, 'U': 0x16, 'V': 0x2F, 'W': 0x11, 'X': 0x2D,
		'Y': 0x15, 'Z': 0x2C,
	}
	if len(key) == 1 {
		b := key[0]
		if b >= 'a' && b <= 'z' {
			b -= 'a' - 'A'
		}
		if scan, ok := letters[b]; ok {
			return scan
		}
		digits := map[byte]uint32{'1': 0x02, '2': 0x03, '3': 0x04, '4': 0x05, '5': 0x06, '6': 0x07, '7': 0x08, '8': 0x09, '9': 0x0A, '0': 0x0B}
		if scan, ok := digits[b]; ok {
			return scan
		}
	}
	if strings.HasPrefix(strings.ToLower(key), "f") {
		if n, err := strconv.Atoi(key[1:]); err == nil && n >= 1 && n <= 24 {
			if n <= 10 {
				return uint32(0x3B + n - 1)
			}
			if n == 11 {
				return 0x57
			}
			if n == 12 {
				return 0x58
			}
			return uint32(0x64 + n - 13)
		}
	}
	keys := map[string]uint32{
		"space": 0x39, "tab": 0x0F, "escape": 0x01, "enter": 0x1C, "backspace": 0x0E,
		"insert": 0x152, "delete": 0x153, "home": 0x147, "end": 0x14F,
		"up": 0x148, "down": 0x150, "left": 0x14B, "right": 0x14D,
		"pageup": 0x149, "pagedown": 0x151,
	}
	return keys[strings.ToLower(key)]
}

func desktopHandleHotkey(hwnd uintptr, id, lparam uintptr) bool {
	desktopHotkeyMu.Lock()
	var action desktopHotkeyAction
	ok := false
	part := -1
	for _, candidate := range desktopHotkeyActions {
		if candidate.id == int(id) {
			action, part, ok = candidate, 0, true
		} else if len(candidate.sequence) == 2 && candidate.waiting && candidate.id+1 == int(id) {
			action, part, ok = candidate, 1, true
		}
		if ok {
			break
		}
	}
	if !ok {
		desktopHotkeyMu.Unlock()
		return false
	}
	if len(action.sequence) == 2 {
		current := desktopHotkeyActions[action.action]
		if current.waiting && part == 0 && action.sequence[1] != action.sequence[0] {
			current.waiting = false
			current.waitToken++
			desktopHotkeyActions[action.action] = current
			desktopHotkeyMu.Unlock()
			if action.action == 1 {
				desktopSendHotkeyChord(desktopHotkeyChord{key: 0x2E, mods: 0x0002}, true)
			} else {
				desktopSendHotkeyChord(action.sequence[0], false)
			}
			return true
		}
		if !current.waiting {
			current.waiting = true
			current.waitToken++
			waitToken := current.waitToken
			if action.action == 1 {
				sequence, _, _ := windowClipboardSequence.Call()
				current.copySeq = uint32(sequence)
			}
			desktopHotkeyActions[action.action] = current
			desktopHotkeyMu.Unlock()
			if action.action == 1 {
				desktopSendHotkeyChord(desktopHotkeyChord{key: 0x2E, mods: 0x0002}, true)
			} else {
				desktopSendHotkeyChord(action.sequence[0], false)
			}
			go func(actionKey, actionID int, waitToken uint64) {
				time.Sleep(900 * time.Millisecond)
				desktopHotkeyMu.Lock()
				defer desktopHotkeyMu.Unlock()
				current, exists := desktopHotkeyActions[actionKey]
				if exists && current.id == actionID && current.waiting && current.waitToken == waitToken {
					current.waiting = false
					desktopHotkeyActions[actionKey] = current
				}
			}(action.action, action.id, waitToken)
			return true
		}
		current.waiting = false
		current.waitToken++
		desktopHotkeyActions[action.action] = current
	}
	desktopHotkeyMu.Unlock()
	switch action.action {
	case 0:
		if desktopWindowHidden.Swap(false) || func() bool { iconic, _, _ := procIsIconic.Call(hwnd); return iconic != 0 }() {
			restoreAndFocusWindow(hwnd)
		} else {
			procShowWindow.Call(hwnd, 0)
			desktopWindowHidden.Store(true)
		}
	case 1:
		copySeq := action.copySeq
		if len(action.sequence) == 1 {
			sequence, _, _ := windowClipboardSequence.Call()
			copySeq = uint32(sequence)
			desktopSendHotkeyChord(desktopHotkeyChord{key: 0x2E, mods: 0x0002}, true)
		} else {
			desktopWindowHidden.Store(false)
			restoreAndFocusWindow(hwnd)
		}
		go func() {
			time.Sleep(60 * time.Millisecond)
			if copySeq != 0 {
				deadline := time.Now().Add(500 * time.Millisecond)
				for time.Now().Before(deadline) {
					sequence, _, _ := windowClipboardSequence.Call()
					if uint32(sequence) != copySeq {
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
				sequence, _, _ := windowClipboardSequence.Call()
				if uint32(sequence) == copySeq {
					if len(action.sequence) == 1 && hwnd != 0 {
						desktopWindowHidden.Store(false)
						restoreAndFocusWindow(hwnd)
					}
					return
				}
			}
			clipboard := desktopReadClipboardText()
			desktopHotkeyMu.Lock()
			hwnd := desktopHotkeyWindow
			desktopHotkeyMu.Unlock()
			desktopWindowMu.Lock()
			w := desktopWindow
			desktopWindowMu.Unlock()
			if w == nil {
				return
			}
			if len(action.sequence) == 1 && hwnd != 0 {
				desktopWindowHidden.Store(false)
				restoreAndFocusWindow(hwnd)
			}
			if clipboard == "" {
				return
			}
			b, _ := json.Marshal(clipboard)
			w.DispatchJavaScript("window.wudictHotkeyLookup&&window.wudictHotkeyLookup(" + string(b) + ")")
		}()
	}
	return true
}

func desktopSendHotkeyChord(chord desktopHotkeyChord, suppressExtraModifiers bool) {
	const (
		inputKeyboard = 1
		keyEventUp    = 0x0002
	)
	modifiers := []uint16{}
	emulatedModifiers := []uint16{}
	extraModifiers := []uint16{}
	if suppressExtraModifiers {
		for _, mod := range []struct {
			flag uint32
			vk   uint16
		}{{0x0001, 0x12}, {0x0004, 0x10}, {0x0008, 0x5B}, {0x0008, 0x5C}} {
			if chord.mods&mod.flag == 0 {
				state, _, _ := windowGetAsyncKeyState.Call(uintptr(mod.vk))
				if int16(state) < 0 {
					extraModifiers = append(extraModifiers, mod.vk)
				}
			}
		}
	}
	for _, mod := range []struct {
		flag uint32
		vk   uint16
	}{{0x0001, 0x12}, {0x0002, 0x11}, {0x0004, 0x10}, {0x0008, 0x5B}} {
		if chord.mods&mod.flag != 0 {
			modifiers = append(modifiers, mod.vk)
			state, _, _ := windowGetAsyncKeyState.Call(uintptr(mod.vk))
			if int16(state) >= 0 {
				emulatedModifiers = append(emulatedModifiers, mod.vk)
			}
		}
	}
	// Reassert modifiers in the injected sequence because the hook consumes the
	// non-modifier key. Release only modifiers we synthesized.
	inputs := make([]desktopInput, 0, len(extraModifiers)*2+len(modifiers)*2+2)
	for _, vk := range extraModifiers {
		inputs = append(inputs, desktopInput{Type: inputKeyboard, Ki: desktopKeyboardInput{Vk: vk, Flags: keyEventUp}})
	}
	for _, vk := range modifiers {
		inputs = append(inputs, desktopInput{Type: inputKeyboard, Ki: desktopKeyboardInput{Vk: vk}})
	}
	inputs = append(inputs,
		desktopPhysicalKeyInput(chord.key, false),
		desktopPhysicalKeyInput(chord.key, true),
	)
	for i := len(emulatedModifiers) - 1; i >= 0; i-- {
		inputs = append(inputs, desktopInput{Type: inputKeyboard, Ki: desktopKeyboardInput{Vk: emulatedModifiers[i], Flags: keyEventUp}})
	}
	for i := len(extraModifiers) - 1; i >= 0; i-- {
		inputs = append(inputs, desktopInput{Type: inputKeyboard, Ki: desktopKeyboardInput{Vk: extraModifiers[i]}})
	}
	sent, _, _ := windowSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	if sent != uintptr(len(inputs)) {
		logx.Warn("could not replay the first hot key chord to the active application")
	}
}

func desktopPhysicalKeyInput(scanCode uint32, keyUp bool) desktopInput {
	const (
		keyEventUp        = 0x0002
		keyEventScanCode  = 0x0008
		keyEventExtended  = 0x0001
		extendedScanMask  = 0x0100
		scanCodeValueMask = 0x00FF
	)
	flags := uint32(keyEventScanCode)
	if scanCode&extendedScanMask != 0 {
		flags |= keyEventExtended
	}
	if keyUp {
		flags |= keyEventUp
	}
	return desktopInput{Type: 1, Ki: desktopKeyboardInput{Scan: uint16(scanCode & scanCodeValueMask), Flags: flags}}
}

func desktopReadClipboardText() string {
	const cfUnicodeText = 13
	if ok, _, _ := windowOpenClipboard.Call(0); ok == 0 {
		return ""
	}
	defer windowCloseClipboard.Call()
	h, _, _ := windowGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	p, _, _ := windowGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer windowGlobalUnlock.Call(h)
	size, _, _ := windowGlobalSize.Call(h)
	if size < 2 || size > 1<<20 {
		return ""
	}
	buf := unsafe.Slice((*uint16)(unsafe.Pointer(p)), int(size/2))
	i := 0
	for i < len(buf) && buf[i] != 0 {
		i++
	}
	return syscall.UTF16ToString(buf[:i])
}

// chooseDesktopFolder shows Windows' native folder browser and returns a
// filesystem path only when the user selects a folder and confirms it.
func chooseDesktopFolder(owner uintptr) (string, bool) {
	const (
		bifReturnOnlyFSDirs = 0x0001
		bifNewDialogStyle   = 0x0040
	)
	type browseInfo struct {
		owner, root uintptr
		displayName *uint16
		title       *uint16
		flags       uint32
		callback    uintptr
		param       uintptr
		image       int32
	}
	title, _ := syscall.UTF16PtrFromString("Select a dictionary folder")
	info := browseInfo{owner: owner, title: title, flags: bifReturnOnlyFSDirs | bifNewDialogStyle}
	item, _, _ := windowBrowseForFolder.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(title)
	if item == 0 {
		return "", false
	}
	defer windowCoTaskMemFree.Call(item)
	path := make([]uint16, 32768)
	ok, _, _ := windowGetPathFromIDList.Call(item, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return "", false
	}
	for i, ch := range path {
		if ch == 0 {
			return syscall.UTF16ToString(path[:i]), true
		}
	}
	return "", false
}

// chooseDesktopArchive uses the Windows file dialog so a selected archive can
// be read by Go in place rather than streamed through the local HTTP server.
func chooseDesktopArchive(owner uintptr) (string, bool) {
	const (
		ofExplorer      = 0x00080000
		ofFileMustExist = 0x00001000
		ofPathMustExist = 0x00000800
		ofNoChangeDir   = 0x00000008
	)
	type openFileName struct {
		structSize       uint32
		owner            uintptr
		instance         uintptr
		filter           *uint16
		customFilter     *uint16
		maxCustFilter    uint32
		filterIndex      uint32
		file             *uint16
		maxFile          uint32
		fileTitle        *uint16
		maxFileTitle     uint32
		initialDir       *uint16
		title            *uint16
		flags            uint32
		fileOffset       uint16
		fileExtension    uint16
		defaultExtension *uint16
		customData       uintptr
		hook             uintptr
		templateName     *uint16
		reservedPtr      uintptr
		reserved         uint32
		flagsEx          uint32
	}
	filter := utf16MultiString("Dictionary archives and files\x00*.zip;*.7z;*.mdx;*.mdd;*.ifo;*.idx;*.dict;*.dz;*.syn;*.dsl;*.slob;*.bgl;*.zim;*.md;*.gz\x00All files\x00*.*")
	title, _ := syscall.UTF16PtrFromString("Choose a dictionary file")
	buffer := make([]uint16, 32768)
	info := openFileName{
		structSize: uint32(unsafe.Sizeof(openFileName{})), owner: owner,
		filter: &filter[0], filterIndex: 1, file: &buffer[0], maxFile: uint32(len(buffer)),
		title: title, flags: ofExplorer | ofFileMustExist | ofPathMustExist | ofNoChangeDir,
	}
	ok, _, _ := windowGetOpenFileName.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(title)
	runtime.KeepAlive(buffer)
	if ok == 0 {
		return "", false
	}
	for i, ch := range buffer {
		if ch == 0 {
			return syscall.UTF16ToString(buffer[:i]), true
		}
	}
	return "", false
}

func chooseArticleSavePath(owner uintptr, filename string) (string, bool) {
	const (
		ofExplorer         = 0x00080000
		ofOverwritePrompt  = 0x00000002
		ofPathMustExist    = 0x00000800
		ofNoChangeDir      = 0x00000008
		ofNoTestFileCreate = 0x00010000
	)
	type openFileName struct {
		structSize                 uint32
		owner, instance            uintptr
		filter, customFilter       *uint16
		maxCustFilter, filterIndex uint32
		file                       *uint16
		maxFile                    uint32
		fileTitle                  *uint16
		maxFileTitle               uint32
		initialDir, title          *uint16
		flags                      uint32
		fileOffset, fileExtension  uint16
		defaultExtension           *uint16
		customData, hook           uintptr
		templateName               *uint16
		reservedPtr                uintptr
		reserved, flagsEx          uint32
	}
	buffer := make([]uint16, 32768)
	defaultName, _ := syscall.UTF16FromString(filepath.Base(filename))
	copy(buffer, defaultName)
	filter := utf16MultiString("HTML article\x00*.html\x00All files\x00*.*")
	title, _ := syscall.UTF16PtrFromString("Save article")
	extension, _ := syscall.UTF16PtrFromString("html")
	info := openFileName{
		structSize: uint32(unsafe.Sizeof(openFileName{})), owner: owner,
		filter: &filter[0], filterIndex: 1, file: &buffer[0], maxFile: uint32(len(buffer)),
		title: title, defaultExtension: extension,
		flags: ofExplorer | ofOverwritePrompt | ofPathMustExist | ofNoChangeDir | ofNoTestFileCreate,
	}
	ok, _, _ := windowGetSaveFileName.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(title)
	runtime.KeepAlive(extension)
	runtime.KeepAlive(buffer)
	if ok == 0 {
		return "", false
	}
	for i, ch := range buffer {
		if ch == 0 {
			return syscall.UTF16ToString(buffer[:i]), true
		}
	}
	return "", false
}

func utf16MultiString(value string) []uint16 {
	units := make([]uint16, 0, len(value)+1)
	for _, part := range strings.Split(value, "\x00") {
		encoded, _ := syscall.UTF16FromString(part)
		units = append(units, encoded[:len(encoded)-1]...)
		units = append(units, 0)
	}
	return append(units, 0)
}

func jsString(s string) string {
	quoted, _ := json.Marshal(s)
	return string(quoted)
}

func watchDesktopServer(w webview.WebView, address *url.URL, done <-chan struct{}) {
	client := &http.Client{Timeout: 1500 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	probe := *address
	probe.Path, probe.RawQuery, probe.Fragment = "/", "", ""
	seen, missed := false, 0
	for {
		response, err := client.Get(probe.String())
		alive := err == nil && strings.HasPrefix(response.Header.Get("Server"), "wudict")
		if response != nil {
			response.Body.Close()
		}
		if alive {
			seen, missed = true, 0
			log.Printf("WebView2 server watch: server alive; status=%s server=%q", response.Status, response.Header.Get("Server"))
		} else if seen {
			missed++
			if err != nil {
				log.Printf("WebView2 server watch: probe failed; missed=%d error=%v", missed, err)
			} else {
				log.Printf("WebView2 server watch: unexpected response; missed=%d status=%s server=%q", missed, response.Status, response.Header.Get("Server"))
			}
			if missed >= 3 {
				log.Printf("WebView2 server watch: terminating window after three failed probes")
				w.Dispatch(func() { w.Terminate() })
				return
			}
		}
		select {
		case <-done:
			return
		case <-time.After(2 * time.Second):
		}
	}
}
