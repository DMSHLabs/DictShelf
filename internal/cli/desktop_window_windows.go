// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"context"
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
	windowDwmapi            = syscall.NewLazyDLL("dwmapi.dll")
	windowUser32            = syscall.NewLazyDLL("user32.dll")
	procShowWindow          = windowUser32.NewProc("ShowWindow")
	windowDwmSetAttribute   = windowDwmapi.NewProc("DwmSetWindowAttribute")
	windowShell32           = syscall.NewLazyDLL("shell32.dll")
	windowBrowseForFolder   = windowShell32.NewProc("SHBrowseForFolderW")
	windowGetPathFromIDList = windowShell32.NewProc("SHGetPathFromIDListW")
	windowComdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	windowGetOpenFileName   = windowComdlg32.NewProc("GetOpenFileNameW")
	windowOle32             = syscall.NewLazyDLL("ole32.dll")
	windowCoTaskMemFree     = windowOle32.NewProc("CoTaskMemFree")
)

var desktopCloseToTray atomic.Bool

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
		navigationAddress := desktopWindowAddress
		desktopWindowMu.Unlock()
		w.Dispatch(func() {
			if navigationAddress != "" {
				w.Navigate(navigationAddress)
			}
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
	webview.SetCloseToTrayHook(func(hwnd uintptr) bool {
		if desktopWindowHidden.Load() || !desktopCloseToTray.Load() {
			return desktopWindowHidden.Load()
		}
		const swHide = 0
		procShowWindow.Call(hwnd, swHide)
		desktopWindowHidden.Store(true)
		return true
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
	webviewStarted := time.Now()
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath:  profile,
		AutoFocus: true,
		WindowOptions: webview.WindowOptions{Title: "DictShelf", Width: 1120, Height: 820, IconId: 101, Center: true,
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
	time.AfterFunc(30*time.Second, func() {
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

// desktopInitialWindowColors reads the durable appearance before WebView2
// creates a visible window. This lets the native window and WebView blank
// surface use the same paper while the runtime and first page initialize.
func desktopInitialWindowColors(request func(context.Context, string, string) (any, error)) (background, caption string) {
	dark := windowsPrefersDark()
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
		return "#191A1C", ""
	}
	return "#FBFAF8", ""
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
