package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type _ICoreWebView2HttpRequestHeadersVtbl struct {
	_IUnknownVtbl
	GetHeader ComProc
}

type ICoreWebView2HttpRequestHeaders struct {
	vtbl *_ICoreWebView2HttpRequestHeadersVtbl
}

func (h *ICoreWebView2HttpRequestHeaders) Release() uintptr {
	r, _, _ := h.vtbl.Release.Call(uintptr(unsafe.Pointer(h)))
	return r
}

func (h *ICoreWebView2HttpRequestHeaders) GetHeader(name string) (string, error) {
	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return "", err
	}
	var value *uint16
	_, _, err = h.vtbl.GetHeader.Call(
		uintptr(unsafe.Pointer(h)),
		uintptr(unsafe.Pointer(name16)),
		uintptr(unsafe.Pointer(&value)),
	)
	if err != windows.ERROR_SUCCESS {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(value))
	return windows.UTF16PtrToString(value), nil
}
