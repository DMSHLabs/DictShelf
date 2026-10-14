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
func openDesktopWindow(address string, onClose func(), desktopSnapshot func(string) (any, error), desktopSearch func(context.Context, string, func([]byte)) error) bool {
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
	go runDesktopWindow(address, profile, onClose, desktopSnapshot, desktopSearch)
	return true
}

func runDesktopWindow(address, profile string, onClose func(), desktopSnapshot func(string) (any, error), desktopSearch func(context.Context, string, func([]byte)) error) {
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
	bridgeDone := make(chan struct{})
	var nativeCallsMu sync.Mutex
	nativeCalls := make(map[int]context.CancelFunc)
	if desktopSnapshot != nil || desktopSearch != nil {
		if err := w.Bind("wudictNativeCancel", func(id int) error {
			nativeCallsMu.Lock()
			cancel := nativeCalls[id]
			delete(nativeCalls, id)
			nativeCallsMu.Unlock()
			if cancel != nil {
				cancel()
			}
			return nil
		}); err != nil {
			logx.Warn("WebView2 native cancellation unavailable: %v", err)
		}
		if err := w.Bind("wudictNativeRequest", func(id int, method, argument string) error {
			if id <= 0 {
				return fmt.Errorf("invalid native request id")
			}
			ctx, cancel := context.WithCancel(context.Background())
			nativeCallsMu.Lock()
			if old := nativeCalls[id]; old != nil {
				old()
			}
			nativeCalls[id] = cancel
			nativeCallsMu.Unlock()
			go func() {
				defer func() {
					cancel()
					nativeCallsMu.Lock()
					delete(nativeCalls, id)
					nativeCallsMu.Unlock()
				}()
				reply := func(payload []byte, callErr error, done bool) {
					if len(payload) == 0 {
						payload = []byte("null")
					}
					errorJSON, _ := json.Marshal(errorText(callErr))
					select {
					case <-bridgeDone:
						return
					default:
					}
					w.Dispatch(func() {
						select {
						case <-bridgeDone:
							return
						default:
						}
						w.Eval("window.wudictNativeReply(" + strconv.Itoa(id) + "," + string(payload) + "," + string(errorJSON) + "," + strconv.FormatBool(done) + ")")
					})
				}
				switch method {
				case "config", "prefs":
					if desktopSnapshot == nil {
						reply(nil, fmt.Errorf("native snapshot API unavailable"), true)
						return
					}
					value, callErr := desktopSnapshot(method)
					payload, marshalErr := json.Marshal(value)
					if marshalErr != nil {
						callErr = marshalErr
					}
					reply(payload, callErr, true)
				case "search":
					if desktopSearch == nil {
						reply(nil, fmt.Errorf("native search API unavailable"), true)
						return
					}
					callErr := desktopSearch(ctx, argument, func(payload []byte) {
						reply(payload, nil, false)
					})
					reply(nil, callErr, true)
				default:
					reply(nil, fmt.Errorf("unknown native method %q", method), true)
				}
			}()
			return nil
		}); err != nil {
			logx.Warn("WebView2 native API unavailable: %v", err)
		} else {
			w.Init(`(function(){
  var pending=new Map(),next=1;
  function forget(id,call){pending.delete(id);if(call.signal)call.signal.removeEventListener('abort',call.abort)}
  window.wudictNativeCall=function(method,argument,onMessage,signal){return new Promise(function(resolve,reject){
    var id=next++,call={resolve:resolve,reject:reject,onMessage:onMessage,signal:signal};
    call.abort=function(){if(!pending.has(id))return;forget(id,call);window.wudictNativeCancel(id);var e=new Error('search cancelled');e.name='AbortError';reject(e)};
    pending.set(id,call);
    if(signal){if(signal.aborted){call.abort();return}signal.addEventListener('abort',call.abort,{once:true})}
    window.wudictNativeRequest(id,method,argument||'').catch(function(error){window.wudictNativeReply(id,null,String(error),true)});
  })};
  window.wudictNativeReply=function(id,value,error,done){
    var call=pending.get(id);if(!call)return;
    if(error){forget(id,call);call.reject(new Error(error));return}
    if(value!==null&&call.onMessage)call.onMessage(value);
    if(done){forget(id,call);call.resolve(value)}
  };
  window.addEventListener('pagehide',function(){for(var [id,call] of pending){forget(id,call);window.wudictNativeCancel(id);call.reject(new Error('page closed'))}});
})()`)
		}
	}
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
	close(bridgeDone)
	nativeCallsMu.Lock()
	for id, cancel := range nativeCalls {
		cancel()
		delete(nativeCalls, id)
	}
	nativeCallsMu.Unlock()
	w.Destroy()
	desktopWindowMu.Lock()
	desktopWindow = nil
	desktopWindowMu.Unlock()
	if onClose != nil {
		onClose()
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
