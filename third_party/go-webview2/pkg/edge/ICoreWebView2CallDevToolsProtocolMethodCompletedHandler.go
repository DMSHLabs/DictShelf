//go:build windows
// +build windows

package edge

import (
	"fmt"
	"unsafe"
)

type iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type iCoreWebView2CallDevToolsProtocolMethodCompletedHandler struct {
	vtbl *iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl
	impl iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl
}

type iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl interface {
	_IUnknownImpl
	CallDevToolsProtocolMethodCompleted(errorCode uintptr) uintptr
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerQueryInterface(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler, refiid, object uintptr) uintptr {
	return this.impl.QueryInterface(refiid, object)
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerAddRef(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.AddRef()
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerRelease(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.Release()
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler, errorCode uintptr, _ uintptr) uintptr {
	return this.impl.CallDevToolsProtocolMethodCompleted(errorCode)
}

var iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn = iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl{
	_IUnknownVtbl{
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerQueryInterface),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerAddRef),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerRelease),
	},
	NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke),
}

func newICoreWebView2CallDevToolsProtocolMethodCompletedHandler(done func(error)) *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler {
	h := &iCoreWebView2CallDevToolsProtocolMethodCompletedHandler{
		vtbl: &iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn,
	}
	h.impl = &callDevToolsCompleted{done: done, self: h}
	return h
}

type callDevToolsCompleted struct {
	done func(error)
	self *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler
}

func (*callDevToolsCompleted) QueryInterface(_, _ uintptr) uintptr { return 0 }
func (*callDevToolsCompleted) AddRef() uintptr                     { return 1 }
func (*callDevToolsCompleted) Release() uintptr                    { return 1 }
func (h *callDevToolsCompleted) CallDevToolsProtocolMethodCompleted(errorCode uintptr) uintptr {
	if h.done != nil {
		if int32(errorCode) < 0 {
			h.done(fmt.Errorf("WebView2 cache clear failed: HRESULT 0x%08X", uint32(errorCode)))
		} else {
			h.done(nil)
		}
		h.done = nil
	}
	if h.self != nil {
		devToolsHandlers.Delete(uintptr(unsafe.Pointer(h.self)))
		h.self = nil
	}
	return 0
}
