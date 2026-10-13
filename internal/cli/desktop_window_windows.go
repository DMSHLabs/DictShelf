// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"github.com/wuweidict/wudict/internal/logx"
)

var (
	desktopWindowMu         sync.Mutex
	desktopWindow           webview.WebView
	desktopWindowStarting   bool
	desktopWindowPendingURL string
	windowUser32            = syscall.NewLazyDLL("user32.dll")
	windowShowWindow        = windowUser32.NewProc("ShowWindow")
	windowForeground        = windowUser32.NewProc("SetForegroundWindow")
	windowShell32           = syscall.NewLazyDLL("shell32.dll")
	windowBrowseForFolder   = windowShell32.NewProc("SHBrowseForFolderW")
	windowGetPathFromIDList = windowShell32.NewProc("SHGetPathFromIDListW")
	windowOle32             = syscall.NewLazyDLL("ole32.dll")
	windowCoTaskMemFree     = windowOle32.NewProc("CoTaskMemFree")
)

// openDesktopWindow opens the local reader on a dedicated Windows UI thread.
// The tray keeps the main thread; WebView2 owns the locked thread below.
func openDesktopWindow(address string, onClose func()) bool {
	version, err := webviewloader.GetInstalledVersion()
	if err != nil || version == "" {
		logx.Warn("WebView2 unavailable; opening the browser: %v", err)
		return false
	}
	profile := os.Getenv("LOCALAPPDATA")
	if profile == "" {
		profile, err = os.UserCacheDir()
		if err != nil {
			return false
		}
	}
	profile = filepath.Join(profile, "wuDict", "WebView2")
	if err := os.MkdirAll(profile, 0700); err != nil {
		logx.Warn("WebView2 profile unavailable: %v", err)
		return false
	}
	local := ""
	if address != "" {
		parsed, err := desktopWindowURL(address)
		if err != nil {
			return false
		}
		local = desktopWindowURLWithMarker(parsed)
	}
	desktopWindowMu.Lock()
	if desktopWindow != nil {
		w := desktopWindow
		desktopWindowMu.Unlock()
		w.Dispatch(func() {
			if local != "" {
				w.Navigate(local)
			}
			hwnd := uintptr(w.Window())
			windowShowWindow.Call(hwnd, 9) // SW_RESTORE
			windowForeground.Call(hwnd)
		})
		return true
	}
	if desktopWindowStarting {
		if local != "" {
			desktopWindowPendingURL = local
		}
		desktopWindowMu.Unlock()
		return true
	}
	desktopWindowStarting = true
	desktopWindowPendingURL = local
	desktopWindowMu.Unlock()
	go runDesktopWindow(profile, onClose)
	return true
}

func showStartupWindow() { openDesktopWindow("", nil) }

func closeStartupWindow() {
	desktopWindowMu.Lock()
	w := desktopWindow
	desktopWindowMu.Unlock()
	if w != nil {
		w.Dispatch(func() { w.Terminate() })
	}
}

func runDesktopWindow(profile string, onClose func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath:      profile,
		AutoFocus:     true,
		WindowOptions: webview.WindowOptions{Title: "wuDict2", Width: 1120, Height: 820, IconId: 101, Center: true},
	})
	if w == nil {
		desktopWindowMu.Lock()
		address := desktopWindowPendingURL
		desktopWindowPendingURL = ""
		desktopWindowStarting = false
		desktopWindowMu.Unlock()
		logx.Warn("WebView2 window failed; opening the browser")
		if address != "" {
			browserCmd(address)
		}
		return
	}
	var local *url.URL
	var folderCallbackPending atomic.Bool
	_ = w.Bind("wudictChooseFolder", func() error {
		if !folderCallbackPending.CompareAndSwap(false, true) {
			return nil
		}
		w.Dispatch(func() {
			path, ok := chooseDesktopFolder(uintptr(w.Window()))
			if ok {
				w.Eval("window.__wdFolderPicked(" + jsString(path) + ")")
			}
			folderCallbackPending.Store(false)
		})
		return nil
	})
	_ = w.Bind("wudictOpenExternal", func(raw string) error {
		link, err := url.Parse(raw)
		if err != nil || (link.Scheme != "http" && link.Scheme != "https") || link.Host == "" {
			return fmt.Errorf("invalid external link")
		}
		if local != nil && strings.EqualFold(link.Host, local.Host) {
			return nil
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", link.String()).Start()
	})
	w.Init(`document.addEventListener('click',function(event){
  var a=event.target&&event.target.closest&&event.target.closest('a[href]');
  if(!a)return;
  try { var u=new URL(a.href); if((u.protocol==='http:'||u.protocol==='https:')&&u.origin!==location.origin){
    event.preventDefault();event.stopPropagation();window.wudictOpenExternal(u.href);
  }}catch(_){ }
},true);`)
	done := make(chan struct{})
	w.Navigate(startupSplashURL())
	desktopWindowMu.Lock()
	desktopWindow = w
	address := desktopWindowPendingURL
	desktopWindowPendingURL = ""
	desktopWindowStarting = false
	desktopWindowMu.Unlock()
	if address != "" {
		w.Navigate(address)
		if parsed, err := url.Parse(address); err == nil {
			local = parsed
			go watchDesktopServer(w, local, done)
		}
	}
	w.Run()
	close(done)
	w.Destroy()
	desktopWindowMu.Lock()
	desktopWindow = nil
	desktopWindowMu.Unlock()
	if onClose != nil {
		onClose()
	}
}

func desktopWindowURLWithMarker(address string) string {
	local, _ := url.Parse(address)
	query := local.Query()
	query.Set("wudict_webview2", "1")
	local.RawQuery = query.Encode()
	return local.String()
}

func startupSplashURL() string {
	language, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	message := "Starting wuDict2…"
	if language&0x3ff == 0x19 {
		message = "Запуск wuDict2…"
	}
	html := `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>
*{box-sizing:border-box}html,body{height:100%;margin:0}body{display:grid;place-items:center;background:#f5f3ef;color:#302d29;font:16px "Segoe UI",sans-serif}.card{display:flex;align-items:center;gap:16px}.spin{width:24px;height:24px;border:3px solid #ded9d0;border-top-color:#8b6545;border-radius:50%;animation:turn .8s linear infinite}@keyframes turn{to{transform:rotate(360deg)}}
</style><main class="card"><span class="spin"></span><span>` + message + `</span></main>`
	return "data:text/html," + url.PathEscape(html)
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
		} else if seen {
			missed++
			if missed >= 3 {
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
