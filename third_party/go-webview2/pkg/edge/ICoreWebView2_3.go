package edge

import (
	"fmt"
	"log"
	"unsafe"

	"golang.org/x/sys/windows"
)

type iCoreWebView2_3Vtbl struct {
	iCoreWebView2_2Vtbl
	TrySuspend                          ComProc
	Resume                              ComProc
	GetIsSuspended                      ComProc
	SetVirtualHostNameToFolderMapping   ComProc
	ClearVirtualHostNameToFolderMapping ComProc
}

type ICoreWebView2_3 struct {
	vtbl *iCoreWebView2_3Vtbl
}

func (i *ICoreWebView2_3) Release() uintptr {
	log.Printf("WebView2 virtual-host mapping: releasing ICoreWebView2_3")
	r, _, _ := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
	log.Printf("WebView2 virtual-host mapping: ICoreWebView2_3 released")
	return r
}

func (i *ICoreWebView2_3) SetVirtualHostNameToFolderMapping(hostName, folderPath string, accessKind COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND) error {
	_hostName, err := windows.UTF16PtrFromString(hostName)
	if err != nil {
		return err
	}

	_folderPath, err := windows.UTF16PtrFromString(folderPath)
	if err != nil {
		return err
	}

	hr, _, _ := i.vtbl.SetVirtualHostNameToFolderMapping.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(_hostName)),
		uintptr(unsafe.Pointer(_folderPath)),
		uintptr(accessKind),
	)
	if int32(hr) < 0 {
		return fmt.Errorf("SetVirtualHostNameToFolderMapping failed with HRESULT 0x%08X", uint32(hr))
	}
	return nil
}

func (i *ICoreWebView2) GetICoreWebView2_3() *ICoreWebView2_3 {
	var result *ICoreWebView2_3

	iidICoreWebView2_3 := NewGUID("{A0D6DF20-3B92-416D-AA0C-437A9C727857}")
	log.Printf("WebView2 virtual-host mapping: querying ICoreWebView2_3")
	hr, _, callErr := i.vtbl.QueryInterface.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(iidICoreWebView2_3)),
		uintptr(unsafe.Pointer(&result)))
	log.Printf("WebView2 virtual-host mapping: QueryInterface HRESULT=0x%08X error=%v result=%p", uint32(hr), callErr, result)
	if int32(hr) < 0 || result == nil {
		return nil
	}

	return result
}

func (e *Chromium) GetICoreWebView2_3() *ICoreWebView2_3 {
	return e.webview.GetICoreWebView2_3()
}
