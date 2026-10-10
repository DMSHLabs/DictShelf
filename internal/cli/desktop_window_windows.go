// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"github.com/wuweidict/wudict/internal/logx"
)

var (
	desktopWindowMu       sync.Mutex
	desktopWindow         webview.WebView
	desktopWindowStarting bool
	windowUser32          = syscall.NewLazyDLL("user32.dll")
	windowShowWindow      = windowUser32.NewProc("ShowWindow")
	windowForeground      = windowUser32.NewProc("SetForegroundWindow")
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
	desktopWindowMu.Lock()
	if desktopWindow != nil {
		w := desktopWindow
		desktopWindowMu.Unlock()
		w.Dispatch(func() {
			hwnd := uintptr(w.Window())
			windowShowWindow.Call(hwnd, 9) // SW_RESTORE
			windowForeground.Call(hwnd)
		})
		return true
	}
	if desktopWindowStarting {
		desktopWindowMu.Unlock()
		return true
	}
	desktopWindowStarting = true
	desktopWindowMu.Unlock()
	go runDesktopWindow(address, profile, onClose)
	return true
}

func runDesktopWindow(address, profile string, onClose func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath:      profile,
		AutoFocus:     true,
		WindowOptions: webview.WindowOptions{Title: "wuDict2", Width: 1120, Height: 820, IconId: 101, Center: true},
	})
	if w == nil {
		desktopWindowMu.Lock()
		desktopWindowStarting = false
		desktopWindowMu.Unlock()
		logx.Warn("WebView2 window failed; opening the browser")
		browserCmd(address)
		return
	}
	desktopWindowMu.Lock()
	desktopWindow, desktopWindowStarting = w, false
	desktopWindowMu.Unlock()
	local, _ := url.Parse(address)
	_ = w.Bind("wudictOpenExternal", func(raw string) error {
		link, err := url.Parse(raw)
		if err != nil || (link.Scheme != "http" && link.Scheme != "https") || link.Host == "" {
			return fmt.Errorf("invalid external link")
		}
		if strings.EqualFold(link.Host, local.Host) {
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
	w.Navigate(address)
	done := make(chan struct{})
	go watchDesktopServer(w, local, done)
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
