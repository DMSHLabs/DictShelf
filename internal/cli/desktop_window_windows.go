// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
)

var (
	desktopWindowMu         sync.Mutex
	desktopWindow           webview.WebView
	desktopWindowStarting   bool
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
func openDesktopWindow(address string, onClose func(), desktopRequest func(string, string) (any, error), desktopStream func(context.Context, string, string, func([]byte)) error) bool {
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
	local, err := desktopWindowURL(address)
	if err != nil {
		return false
	}
	local = desktopWindowURLWithMarker(local)
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
		desktopWindowMu.Unlock()
		return true
	}
	desktopWindowStarting = true
	desktopWindowMu.Unlock()
	go runDesktopWindow(local, profile, onClose, desktopRequest, desktopStream)
	return true
}

func runDesktopWindow(address, profile string, onClose func(), desktopRequest func(string, string) (any, error), desktopStream func(context.Context, string, string, func([]byte)) error) {
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
		if address != "" {
			browserCmd(address)
		}
		return
	}
	local, _ := url.Parse(address)
	if desktopRequest != nil || desktopStream != nil {
		bindNativeBridge(w, local.String(), desktopRequest, desktopStream)
	}
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
	desktopWindowMu.Lock()
	desktopWindow = w
	desktopWindowStarting = false
	desktopWindowMu.Unlock()
	w.Navigate(address)
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

type nativeCall struct{ cancel context.CancelFunc }

var nativeCallsMu sync.Mutex
var nativeCalls = map[int]nativeCall{}

func bindNativeBridge(w webview.WebView, baseURL string, request func(string, string) (any, error), stream func(context.Context, string, string, func([]byte)) error) {
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
			if (method == "search" || method == "ingest" || method == "dicts" || method == "rescanIndexes") && stream != nil {
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
			value, err := request(method, argument)
			if ctx.Err() == nil {
				reply(value, err, true)
			}
		}()
		return nil
	}); err != nil {
		logx.Warn("WebView2 native request binding failed: %v", err)
		return
	}
	w.Init(`(function(){var next=1,pending=new Map();window.wudictNativeShell=true;window.wudictNativeCall=function(method,argument,onMessage,signal){return new Promise(function(resolve,reject){var id=next++,call={resolve:resolve,reject:reject,onMessage:onMessage};pending.set(id,call);if(signal){if(signal.aborted){pending.delete(id);reject(new DOMException('Aborted','AbortError'));return}signal.addEventListener('abort',function(){pending.delete(id);window.wudictNativeCancel(id);var e=new Error('search cancelled');e.name='AbortError';reject(e)},{once:true})}window.wudictNativeRequest(id,method,argument||'').catch(function(error){window.wudictNativeReply(id,null,String(error),true)})})};window.wudictNativeReply=function(id,value,error,done){var call=pending.get(id);if(!call)return;if(error){pending.delete(id);call.reject(new Error(error));return}if(value!==null&&call.onMessage)call.onMessage(value);if(done){pending.delete(id);call.resolve(value)}};window.addEventListener('pagehide',function(){for(var [id,call] of pending){pending.delete(id);window.wudictNativeCancel(id);call.reject(new Error('page closed'))}})})();`)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func desktopWindowURLWithMarker(address string) string {
	local, _ := url.Parse(address)
	query := local.Query()
	query.Set("wudict_webview2", "1")
	local.RawQuery = query.Encode()
	return local.String()
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
