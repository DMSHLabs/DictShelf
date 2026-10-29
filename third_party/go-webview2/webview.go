//go:build windows
// +build windows

package webview2

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"github.com/jchv/go-webview2/pkg/edge"

	"golang.org/x/sys/windows"
)

var (
	windowContext        = map[uintptr]interface{}{}
	windowContextSync    sync.RWMutex
	closeToTrayHook      func(uintptr) bool
	minimizeToTrayHook   func(uintptr) bool
	hotkeyHandler        func(uintptr, uintptr, uintptr) bool
	windowSizeHandler    func(uintptr, int, int)
	windowMessageHandler func(uintptr, uintptr, uintptr, uintptr) bool
)

// SetCloseToTrayHook lets a Windows host consume WM_CLOSE without destroying
// the WebView window. Returning true means the hook handled the close.
func SetCloseToTrayHook(hook func(uintptr) bool) { closeToTrayHook = hook }

// SetMinimizeToTrayHook lets a Windows host consume the minimize command and
// hide the WebView window instead. Returning true means the hook handled it.
func SetMinimizeToTrayHook(hook func(uintptr) bool) { minimizeToTrayHook = hook }

// SetHotkeyHandler handles WM_HOTKEY messages for the native host.
func SetHotkeyHandler(handler func(uintptr, uintptr, uintptr) bool) { hotkeyHandler = handler }

// SetWindowSizeHandler observes the native client size as it changes.
func SetWindowSizeHandler(handler func(uintptr, int, int)) { windowSizeHandler = handler }

// SetWindowMessageHandler observes native window messages before the default
// handler processes them. Return true to consume a message.
func SetWindowMessageHandler(handler func(uintptr, uintptr, uintptr, uintptr) bool) {
	windowMessageHandler = handler
}

func getWindowContext(wnd uintptr) interface{} {
	windowContextSync.RLock()
	defer windowContextSync.RUnlock()
	return windowContext[wnd]
}

func setWindowContext(wnd uintptr, data interface{}) {
	windowContextSync.Lock()
	defer windowContextSync.Unlock()
	windowContext[wnd] = data
}

type browser interface {
	Embed(hwnd uintptr) bool
	Resize()
	Navigate(url string)
	NavigateToString(htmlContent string)
	Init(script string)
	Eval(script string)
	NotifyParentWindowPositionChanged() error
	Focus()
}

type webview struct {
	hwnd          uintptr
	mainthread    uintptr
	browser       browser
	autofocus     bool
	maxsz         w32.Point
	minsz         w32.Point
	m             sync.Mutex
	bindings      map[string]interface{}
	dispatchq     []func()
	startupSplash uintptr
}

type WindowOptions struct {
	Title           string
	Width           uint
	Height          uint
	IconId          uint
	Center          bool
	BackgroundColor string // CSS #RRGGBB for the initial client surface
	CaptionColor    string // CSS #RRGGBB; empty uses the Windows default
	StartupSplash   bool   // paints a native loading screen while WebView2 initializes
}

var (
	webviewGDI32           = windows.NewLazySystemDLL("gdi32.dll")
	createBrush            = webviewGDI32.NewProc("CreateSolidBrush")
	deleteObject           = webviewGDI32.NewProc("DeleteObject")
	webviewUser32          = windows.NewLazySystemDLL("user32.dll")
	webviewDWM             = windows.NewLazySystemDLL("dwmapi.dll")
	dwmSetAttr             = webviewDWM.NewProc("DwmSetWindowAttribute")
	webviewGDI             = windows.NewLazySystemDLL("gdi32.dll")
	createPen              = webviewGDI.NewProc("CreatePen")
	selectObject           = webviewGDI.NewProc("SelectObject")
	moveToEx               = webviewGDI.NewProc("MoveToEx")
	lineTo                 = webviewGDI.NewProc("LineTo")
	createCompatibleDC     = webviewGDI.NewProc("CreateCompatibleDC")
	createCompatibleBitmap = webviewGDI.NewProc("CreateCompatibleBitmap")
	bitBlt                 = webviewGDI.NewProc("BitBlt")
	deleteDC               = webviewGDI.NewProc("DeleteDC")
	beginPaint             = webviewUser32.NewProc("BeginPaint")
	endPaint               = webviewUser32.NewProc("EndPaint")
	getClientRect          = webviewUser32.NewProc("GetClientRect")
	invalidateRect         = webviewUser32.NewProc("InvalidateRect")
	setTimer               = webviewUser32.NewProc("SetTimer")
	killTimer              = webviewUser32.NewProc("KillTimer")
	setWindowPos           = webviewUser32.NewProc("SetWindowPos")
	createFont             = webviewGDI.NewProc("CreateFontW")
	setTextColor           = webviewGDI.NewProc("SetTextColor")
	setBkMode              = webviewGDI.NewProc("SetBkMode")
	fillRect               = webviewUser32.NewProc("FillRect")
	drawText               = webviewUser32.NewProc("DrawTextW")
	startupSplashClassOnce sync.Once
	startupSplashClassName = "wudict2-startup-splash"
	startupSplashes        = map[uintptr]*startupSplashState{}
	windowBackgroundBrush  uintptr
)

type paintStruct struct {
	HDC       windows.Handle
	Erase     int32
	Paint     w32.Rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type startupSplashState struct {
	background uint32
	foreground uint32
	phase      float64
	text       string
}

func parseWindowColor(value string) (r, g, b uint8, ok bool) {
	if len(value) != 7 || value[0] != '#' {
		return 0, 0, 0, false
	}
	raw, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(raw >> 16), uint8(raw >> 8), uint8(raw), true
}

func windowColorRef(r, g, b uint8) uintptr {
	return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16)
}

func setCaptionColor(hwnd uintptr, value string) {
	if hwnd == 0 {
		return
	}
	caption, text := uint32(0xFFFFFFFF), uint32(0xFFFFFFFF)
	if r, g, b, ok := parseWindowColor(value); ok {
		caption = uint32(windowColorRef(r, g, b))
		if 299*uint32(r)+587*uint32(g)+114*uint32(b) >= 128000 {
			text = 0
		} else {
			text = 0x00FFFFFF
		}
	}
	_, _, _ = dwmSetAttr.Call(hwnd, 35, uintptr(unsafe.Pointer(&caption)), unsafe.Sizeof(caption))
	_, _, _ = dwmSetAttr.Call(hwnd, 36, uintptr(unsafe.Pointer(&text)), unsafe.Sizeof(text))
}

type WebViewOptions struct {
	Window unsafe.Pointer
	Debug  bool

	// DataPath specifies the datapath for the WebView2 runtime to use for the
	// browser instance.
	DataPath string

	// AutoFocus will try to keep the WebView2 widget focused when the window
	// is focused.
	AutoFocus bool

	// WindowOptions customizes the window that is created to embed the
	// WebView2 widget.
	WindowOptions WindowOptions
}

// New creates a new webview in a new window.
func New(debug bool) WebView { return NewWithOptions(WebViewOptions{Debug: debug}) }

// NewWindow creates a new webview using an existing window.
//
// Deprecated: Use NewWithOptions.
func NewWindow(debug bool, window unsafe.Pointer) WebView {
	return NewWithOptions(WebViewOptions{Debug: debug, Window: window})
}

// NewWithOptions creates a new webview using the provided options.
func NewWithOptions(options WebViewOptions) WebView {
	w := &webview{}
	w.bindings = map[string]interface{}{}
	w.autofocus = options.AutoFocus

	chromium := edge.NewChromium()
	chromium.MessageCallback = w.msgcb
	chromium.DataPath = options.DataPath
	if r, g, b, ok := parseWindowColor(options.WindowOptions.BackgroundColor); ok {
		chromium.InitialBackgroundColor = &edge.COREWEBVIEW2_COLOR{A: 255, R: r, G: g, B: b}
	}
	chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)

	w.browser = chromium
	w.mainthread, _, _ = w32.Kernel32GetCurrentThreadID.Call()
	if !w.CreateWithOptions(options.WindowOptions) {
		return nil
	}

	settings, err := chromium.GetSettings()
	if err != nil {
		log.Fatal(err)
	}
	// Hide the native hover-status overlay so it does not cover application controls.
	err = settings.PutIsStatusBarEnabled(false)
	if err != nil {
		log.Fatal(err)
	}
	// disable context menu
	err = settings.PutAreDefaultContextMenusEnabled(options.Debug)
	if err != nil {
		log.Fatal(err)
	}
	// disable developer tools
	err = settings.PutAreDevToolsEnabled(options.Debug)
	if err != nil {
		log.Fatal(err)
	}

	return w
}

// SetWebResourceRequestedHandler adds one or more | separated request filters
// and handles matching requests. The callback runs on the WebView2 UI thread.
func (w *webview) SetWebResourceRequestedHandler(filter string, handler WebResourceHandler) error {
	chromium, ok := w.browser.(*edge.Chromium)
	if !ok {
		return errors.New("WebView2 resource interception is unavailable")
	}
	if handler == nil {
		return errors.New("WebView2 resource handler is nil")
	}
	chromium.WebResourceRequestedCallback = func(request *edge.ICoreWebView2WebResourceRequest, args *edge.ICoreWebView2WebResourceRequestedEventArgs) {
		uri, err := request.GetUri()
		if err != nil {
			log.Printf("WebView2 resource URI failed: %v", err)
			return
		}
		method, err := request.GetMethod()
		if err != nil {
			log.Printf("WebView2 resource method failed: %v", err)
			return
		}
		requestHeaders := make(map[string]string, 7)
		if requestHeadersCOM, err := request.GetHeaders(); err == nil {
			defer requestHeadersCOM.Release()
			for _, name := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match", "If-Unmodified-Since", "If-Match", "Accept-Language"} {
				if value, err := requestHeadersCOM.GetHeader(name); err == nil && value != "" {
					requestHeaders[name] = value
				}
			}
		}
		// Intercepted WebView2 requests can omit Accept-Language. Resolve the
		// desktop app's System language from the Windows user UI language instead
		// of letting the interface silently fall back to English.
		if proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage"); proc != nil {
			if lang, _, _ := proc.Call(); lang != 0 {
				switch uint16(lang) & 0x03ff {
				case 0x0019: // Russian
					requestHeaders["Accept-Language"] = "ru"
				default:
					requestHeaders["Accept-Language"] = "en"
				}
			}
		}
		var spool *edge.FileStream
		spill := func() (io.WriteSeeker, error) {
			var err error
			spool, err = edge.NewFileStream()
			if err != nil {
				return nil, err
			}
			return spool, nil
		}
		body, status, headers, handled, spilled := handler(method, uri, requestHeaders, spill)
		log.Printf("WebView2 resource request: method=%s uri=%s handled=%t status=%d", method, uri, handled, status)
		if !handled {
			if spool != nil {
				_ = spool.Close()
			}
			return
		}
		if status == 0 {
			status = 200
		}
		var response *edge.ICoreWebView2WebResourceResponse
		if spilled {
			if spool == nil {
				log.Printf("WebView2 resource stream is unavailable")
				return
			}
			if _, err = spool.Seek(0, io.SeekStart); err != nil {
				_ = spool.Close()
				log.Printf("WebView2 resource rewind failed: %v", err)
				return
			}
			response, err = chromium.Environment().CreateWebResourceResponseFromStream(spool.Pointer(), status, http.StatusText(status), headers)
		} else {
			response, err = chromium.Environment().CreateWebResourceResponse(body, status, http.StatusText(status), headers)
		}
		if err != nil {
			if spool != nil {
				_ = spool.Close()
			}
			log.Printf("WebView2 resource response failed: %v", err)
			return
		}
		if err := args.PutResponse(response); err != nil {
			log.Printf("WebView2 resource response could not be installed: %v", err)
		}
		response.Release()
		if spool != nil {
			_ = spool.Close()
		}
	}
	for _, pattern := range strings.Split(filter, "|") {
		if pattern = strings.TrimSpace(pattern); pattern != "" {
			chromium.AddWebResourceRequestedFilter(pattern, edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL)
		}
	}
	return nil
}

func (w *webview) SetVirtualHostNameToFolderMapping(hostName, folderPath string) error {
	chromium, ok := w.browser.(*edge.Chromium)
	if !ok {
		return errors.New("WebView2 virtual host mapping is unavailable")
	}
	webview3 := chromium.GetICoreWebView2_3()
	if webview3 == nil {
		log.Printf("WebView2 virtual-host mapping: ICoreWebView2_3 is unavailable")
		return errors.New("WebView2 virtual host mapping is unsupported")
	}
	log.Printf("WebView2 virtual-host mapping: ICoreWebView2_3 queried; setting %q -> %q", hostName, folderPath)
	defer webview3.Release()
	err := webview3.SetVirtualHostNameToFolderMapping(hostName, folderPath, edge.COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_DENY_CORS)
	log.Printf("WebView2 virtual-host mapping: call returned; error=%v", err)
	return err
}

type rpcMessage struct {
	ID     int               `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func jsString(v interface{}) string { b, _ := json.Marshal(v); return string(b) }

func (w *webview) msgcb(msg string) {
	d := rpcMessage{}
	if err := json.Unmarshal([]byte(msg), &d); err != nil {
		log.Printf("invalid RPC message: %v", err)
		return
	}
	log.Printf("WebView2 RPC received: method=%s id=%d", d.Method, d.ID)

	id := strconv.Itoa(d.ID)
	if res, err := w.callbinding(d); err != nil {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else if b, err := json.Marshal(res); err != nil {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].resolve(" + string(b) + "); window._rpc[" + id + "] = undefined")
		})
	}
}

func (w *webview) callbinding(d rpcMessage) (interface{}, error) {
	w.m.Lock()
	f, ok := w.bindings[d.Method]
	w.m.Unlock()
	if !ok {
		return nil, nil
	}

	v := reflect.ValueOf(f)
	isVariadic := v.Type().IsVariadic()
	numIn := v.Type().NumIn()
	if (isVariadic && len(d.Params) < numIn-1) || (!isVariadic && len(d.Params) != numIn) {
		return nil, errors.New("function arguments mismatch")
	}
	args := []reflect.Value{}
	for i := range d.Params {
		var arg reflect.Value
		if isVariadic && i >= numIn-1 {
			arg = reflect.New(v.Type().In(numIn - 1).Elem())
		} else {
			arg = reflect.New(v.Type().In(i))
		}
		if err := json.Unmarshal(d.Params[i], arg.Interface()); err != nil {
			return nil, err
		}
		args = append(args, arg.Elem())
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	res := v.Call(args)
	switch len(res) {
	case 0:
		// No results from the function, just return nil
		return nil, nil

	case 1:
		// One result may be a value, or an error
		if res[0].Type().Implements(errorType) {
			if res[0].Interface() != nil {
				return nil, res[0].Interface().(error)
			}
			return nil, nil
		}
		return res[0].Interface(), nil

	case 2:
		// Two results: first one is value, second is error
		if !res[1].Type().Implements(errorType) {
			return nil, errors.New("second return value must be an error")
		}
		if res[1].Interface() == nil {
			return res[0].Interface(), nil
		}
		return res[0].Interface(), res[1].Interface().(error)

	default:
		return nil, errors.New("unexpected number of return values")
	}
}

func wndproc(hwnd, msg, wp, lp uintptr) uintptr {
	if w, ok := getWindowContext(hwnd).(*webview); ok {
		if windowMessageHandler != nil && windowMessageHandler(hwnd, msg, wp, lp) {
			return 0
		}
		switch msg {
		case w32.WMMove, w32.WMMoving:
			_ = w.browser.NotifyParentWindowPositionChanged()
		case w32.WMNCLButtonDown:
			_, _, _ = w32.User32SetFocus.Call(w.hwnd)
			r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		case w32.WMSize:
			if windowSizeHandler != nil {
				windowSizeHandler(hwnd, int(uint16(lp)), int(uint16(lp>>16)))
			}
			if w.browser != nil {
				w.browser.Resize()
			}
			if w.startupSplash != 0 {
				var rect w32.Rect
				_, _, _ = getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
				_, _, _ = setWindowPos.Call(w.startupSplash, 0, 0, 0,
					uintptr(rect.Right-rect.Left), uintptr(rect.Bottom-rect.Top), 0x0010)
			}
		case w32.WMActivate:
			if wp == w32.WAInactive {
				break
			}
			if w.autofocus {
				w.browser.Focus()
			}
		case w32.WMHotkey:
			if hotkeyHandler != nil && hotkeyHandler(hwnd, wp, lp) {
				return 0
			}
		case w32.WMSysCommand:
			if wp&0xFFF0 == w32.SCMinimize && minimizeToTrayHook != nil && minimizeToTrayHook(hwnd) {
				return 0
			}
			r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		case w32.WMClose:
			if closeToTrayHook != nil && closeToTrayHook(hwnd) {
				return 0
			}
			_, _, _ = w32.User32DestroyWindow.Call(hwnd)
		case w32.WMDestroy:
			w.Terminate()
		case w32.WMGetMinMaxInfo:
			lpmmi := (*w32.MinMaxInfo)(unsafe.Pointer(lp))
			if w.maxsz.X > 0 && w.maxsz.Y > 0 {
				lpmmi.PtMaxSize = w.maxsz
				lpmmi.PtMaxTrackSize = w.maxsz
			}
			if w.minsz.X > 0 && w.minsz.Y > 0 {
				lpmmi.PtMinTrackSize = w.minsz
			}
		default:
			r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		}
		return 0
	}
	r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func registerStartupSplashClass() {
	startupSplashClassOnce.Do(func() {
		name, _ := windows.UTF16PtrFromString(startupSplashClassName)
		var instance windows.Handle
		_ = windows.GetModuleHandleEx(0, nil, &instance)
		wc := w32.WndClassExW{CbSize: uint32(unsafe.Sizeof(w32.WndClassExW{})),
			HInstance: instance, LpfnWndProc: windows.NewCallback(startupSplashWndProc), LpszClassName: name}
		_, _, _ = w32.User32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
}

func createStartupSplash(parent uintptr, background string) uintptr {
	registerStartupSplashClass()
	r, g, b, ok := parseWindowColor(background)
	if !ok {
		return 0
	}
	color := uint32(windowColorRef(r, g, b))
	foreground := uint32(0x00201A12)
	if 299*uint32(r)+587*uint32(g)+114*uint32(b) < 128000 {
		foreground = 0x00F4EEE4
	}
	var rect w32.Rect
	_, _, _ = getClientRect.Call(parent, uintptr(unsafe.Pointer(&rect)))
	className, _ := windows.UTF16PtrFromString(startupSplashClassName)
	var instance windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &instance)
	hwnd, _, _ := w32.User32CreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), 0,
		0x40000000, 0, 0, uintptr(rect.Right-rect.Left), uintptr(rect.Bottom-rect.Top),
		parent, 0, uintptr(instance), 0) // WS_CHILD; shown after its paint state is registered.
	if hwnd == 0 {
		return 0
	}
	startupSplashes[hwnd] = &startupSplashState{background: color, foreground: foreground, text: "DictShelf"}
	setTimer.Call(hwnd, 1, 70, 0)
	_, _, _ = w32.User32ShowWindow.Call(hwnd, w32.SWShow)
	return hwnd
}

func startupSplashWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	state := startupSplashes[hwnd]
	switch msg {
	case 0x000F: // WM_PAINT
		var ps paintStruct
		hdc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var rect w32.Rect
		_, _, _ = getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		if state != nil {
			width, height := rect.Right-rect.Left, rect.Bottom-rect.Top
			buffer, _, _ := createCompatibleDC.Call(hdc)
			bitmap, _, _ := createCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
			oldBitmap, _, _ := selectObject.Call(buffer, bitmap)
			brush, _, _ := createBrush.Call(uintptr(state.background))
			fillRect.Call(buffer, uintptr(unsafe.Pointer(&rect)), brush)
			_, _, _ = deleteObject.Call(brush)
			paintStartupSpinner(buffer, rect, state)
			face, _ := windows.UTF16PtrFromString("Segoe UI")
			font, _, _ := createFont.Call(^uintptr(35), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
			oldFont, _, _ := selectObject.Call(buffer, font)
			setTextColor.Call(buffer, uintptr(state.foreground))
			setBkMode.Call(buffer, 1) // TRANSPARENT
			label := state.text
			if label == "" {
				label = "DictShelf"
			}
			text, _ := windows.UTF16PtrFromString(label)
			textRect := w32.Rect{Left: 16, Top: rect.Bottom/2 + 68, Right: rect.Right - 16, Bottom: rect.Bottom/2 + 124}
			drawText.Call(buffer, uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&textRect)), 0x00000001|0x00000004)
			bitBlt.Call(hdc, 0, 0, uintptr(width), uintptr(height), buffer, 0, 0, 0x00CC0020)
			selectObject.Call(buffer, oldFont)
			selectObject.Call(buffer, oldBitmap)
			deleteObject.Call(font)
			deleteObject.Call(bitmap)
			deleteDC.Call(buffer)
		}
		endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0014: // WM_ERASEBKGND: WM_PAINT supplies a complete buffered frame.
		return 1
	case 0x0113: // WM_TIMER
		if state != nil {
			state.phase += 0.12
			_, _, _ = invalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case 0x0002: // WM_DESTROY
		killTimer.Call(hwnd, 1)
		delete(startupSplashes, hwnd)
		return 0
	}
	r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func paintStartupSpinner(hdc uintptr, rect w32.Rect, state *startupSplashState) {
	cx, cy := int(rect.Right-rect.Left)/2, int(rect.Bottom-rect.Top)/2-48
	const radius, count = 42.0, 44
	for i := 0; i < count; i++ {
		a := state.phase + 2*math.Pi*float64(i)/count
		b := a + 2*math.Pi*0.72/count
		x1, y1 := cx+int(math.Cos(a)*radius), cy+int(math.Sin(a)*radius)
		x2, y2 := cx+int(math.Cos(b)*radius), cy+int(math.Sin(b)*radius)
		pen, _, _ := createPen.Call(0, 7, uintptr(startupGradientColor(float64(i)/count)))
		oldPen, _, _ := selectObject.Call(hdc, pen)
		moveToEx.Call(hdc, uintptr(x1), uintptr(y1), 0)
		lineTo.Call(hdc, uintptr(x2), uintptr(y2))
		selectObject.Call(hdc, oldPen)
		deleteObject.Call(pen)
	}
}

func startupGradientColor(t float64) uint32 {
	palette := [4][3]uint8{{255, 177, 40}, {255, 220, 120}, {255, 239, 190}, {226, 126, 0}}
	x := t * float64(len(palette))
	i := int(x) % len(palette)
	f := x - math.Floor(x)
	a, b := palette[i], palette[(i+1)%len(palette)]
	r := uint8(float64(a[0]) + (float64(b[0])-float64(a[0]))*f)
	g := uint8(float64(a[1]) + (float64(b[1])-float64(a[1]))*f)
	bl := uint8(float64(a[2]) + (float64(b[2])-float64(a[2]))*f)
	return uint32(windowColorRef(r, g, bl))
}

func (w *webview) Create(debug bool, window unsafe.Pointer) bool {
	// This function signature stopped making sense a long time ago.
	// It is but legacy cruft at this point.
	return w.CreateWithOptions(WindowOptions{})
}

func (w *webview) CreateWithOptions(opts WindowOptions) bool {
	var hinstance windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinstance)

	var icon uintptr
	if opts.IconId == 0 {
		// load default icon
		icow, _, _ := w32.User32GetSystemMetrics.Call(w32.SystemMetricsCxIcon)
		icoh, _, _ := w32.User32GetSystemMetrics.Call(w32.SystemMetricsCyIcon)
		icon, _, _ = w32.User32LoadImageW.Call(uintptr(hinstance), 32512, icow, icoh, 0)
	} else {
		// load icon from resource
		icon, _, _ = w32.User32LoadImageW.Call(uintptr(hinstance), uintptr(opts.IconId), 1, 0, 0, w32.LR_DEFAULTSIZE|w32.LR_SHARED)
	}

	className, _ := windows.UTF16PtrFromString("webview")
	wc := w32.WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(w32.WndClassExW{})),
		HInstance:     hinstance,
		LpszClassName: className,
		HIcon:         windows.Handle(icon),
		HIconSm:       windows.Handle(icon),
		LpfnWndProc:   windows.NewCallback(wndproc),
	}
	_, _, _ = w32.User32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	windowName, _ := windows.UTF16PtrFromString(opts.Title)

	windowWidth := opts.Width
	if windowWidth == 0 {
		windowWidth = 640
	}
	windowHeight := opts.Height
	if windowHeight == 0 {
		windowHeight = 480
	}

	var posX, posY uint
	if opts.Center {
		// get screen size
		screenWidth, _, _ := w32.User32GetSystemMetrics.Call(w32.SM_CXSCREEN)
		screenHeight, _, _ := w32.User32GetSystemMetrics.Call(w32.SM_CYSCREEN)
		// calculate window position
		posX = (uint(screenWidth) - windowWidth) / 2
		posY = (uint(screenHeight) - windowHeight) / 2
	} else {
		// use default position
		posX = w32.CW_USEDEFAULT
		posY = w32.CW_USEDEFAULT
	}

	w.hwnd, _, _ = w32.User32CreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0xCF0000, // WS_OVERLAPPEDWINDOW
		uintptr(posX),
		uintptr(posY),
		uintptr(windowWidth),
		uintptr(windowHeight),
		0,
		0,
		uintptr(hinstance),
		0,
	)
	setWindowContext(w.hwnd, w)
	if r, g, b, ok := parseWindowColor(opts.BackgroundColor); ok {
		brush, _, _ := createBrush.Call(windowColorRef(r, g, b))
		// GCLP_HBRBACKGROUND (-10). The window is still hidden here, so changing
		// the shared class brush before ShowWindow cannot flash the old color.
		w32.SetClassLong(w.hwnd, -10, brush)
		if windowBackgroundBrush != 0 {
			_, _, _ = deleteObject.Call(windowBackgroundBrush)
		}
		windowBackgroundBrush = brush
	}
	setCaptionColor(w.hwnd, opts.CaptionColor)
	if opts.StartupSplash {
		w.startupSplash = createStartupSplash(w.hwnd, opts.BackgroundColor)
	}

	_, _, _ = w32.User32ShowWindow.Call(w.hwnd, w32.SWShow)
	_, _, _ = w32.User32UpdateWindow.Call(w.hwnd)
	_, _, _ = w32.User32SetFocus.Call(w.hwnd)

	if !w.browser.Embed(w.hwnd) {
		return false
	}
	if w.startupSplash != 0 {
		_, _, _ = setWindowPos.Call(w.startupSplash, 0, 0, 0, 0, 0, 0x0013)
	}
	w.browser.Resize()
	return true
}

func (w *webview) Destroy() {
	_, _, _ = w32.User32PostMessageW.Call(w.hwnd, w32.WMClose, 0, 0)
}

func (w *webview) hideStartupSplash() {
	if w.startupSplash == 0 {
		return
	}
	_, _, _ = w32.User32DestroyWindow.Call(w.startupSplash)
	w.startupSplash = 0
}

// HideStartupSplash removes the native loading screen from a WebView created
// with WindowOptions.StartupSplash.
func HideStartupSplash(view WebView) {
	if w, ok := view.(*webview); ok {
		w.hideStartupSplash()
	}
}

// SetStartupSplashText updates the message on a WebView's native startup splash.
// Call it on the UI thread, normally through WebView.Dispatch.
func SetStartupSplashText(view WebView, text string) {
	if w, ok := view.(*webview); ok && w.startupSplash != 0 {
		if state := startupSplashes[w.startupSplash]; state != nil {
			state.text = text
			_, _, _ = invalidateRect.Call(w.startupSplash, 0, 0)
		}
	}
}

func (w *webview) Run() {
	var msg w32.Msg
	for {
		_, _, _ = w32.User32GetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if msg.Message == w32.WMApp {
			w.m.Lock()
			q := append([]func(){}, w.dispatchq...)
			w.dispatchq = []func(){}
			w.m.Unlock()
			for _, v := range q {
				v()
			}
		} else if msg.Message == w32.WMQuit {
			return
		}
		r, _, _ := w32.User32GetAncestor.Call(uintptr(msg.Hwnd), w32.GARoot)
		r, _, _ = w32.User32IsDialogMessage.Call(r, uintptr(unsafe.Pointer(&msg)))
		if r != 0 {
			continue
		}
		_, _, _ = w32.User32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = w32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (w *webview) Terminate() {
	_, _, _ = w32.User32PostQuitMessage.Call(0)
}

func (w *webview) Window() unsafe.Pointer {
	return unsafe.Pointer(w.hwnd)
}

func (w *webview) Navigate(url string) {
	w.browser.Navigate(url)
}

func (w *webview) SetHtml(html string) {
	w.browser.NavigateToString(html)
}

func (w *webview) SetTitle(title string) {
	_title, err := windows.UTF16FromString(title)
	if err != nil {
		_title, _ = windows.UTF16FromString("")
	}
	_, _, _ = w32.User32SetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(&_title[0])))
}

func (w *webview) SetSize(width int, height int, hints Hint) {
	index := w32.GWLStyle
	style := w32.GetWindowLong(w.hwnd, index)
	if hints == HintFixed {
		style &^= (w32.WSThickFrame | w32.WSMaximizeBox)
	} else {
		style |= (w32.WSThickFrame | w32.WSMaximizeBox)
	}
	w32.SetWindowLong(w.hwnd, index, style)

	if hints == HintMax {
		w.maxsz.X = int32(width)
		w.maxsz.Y = int32(height)
	} else if hints == HintMin {
		w.minsz.X = int32(width)
		w.minsz.Y = int32(height)
	} else {
		r := w32.Rect{}
		r.Left = 0
		r.Top = 0
		r.Right = int32(width)
		r.Bottom = int32(height)
		_, _, _ = w32.User32AdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), w32.WSOverlappedWindow, 0)
		_, _, _ = w32.User32SetWindowPos.Call(
			w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			w32.SWPNoZOrder|w32.SWPNoActivate|w32.SWPNoMove|w32.SWPFrameChanged)
		w.browser.Resize()
	}
}

func (w *webview) Init(js string) {
	w.browser.Init(js)
}

func (w *webview) Eval(js string) {
	w.browser.Eval(js)
}

func (w *webview) DispatchJavaScript(js string) { w.Dispatch(func() { w.Eval(js) }) }

func (w *webview) ClearBrowserCache(done func(error)) error {
	cache, ok := w.browser.(interface{ ClearBrowserCache(func(error)) error })
	if !ok {
		return errors.New("browser cache clearing is unavailable")
	}
	return cache.ClearBrowserCache(done)
}

func (w *webview) Dispatch(f func()) {
	w.m.Lock()
	w.dispatchq = append(w.dispatchq, f)
	w.m.Unlock()
	_, _, _ = w32.User32PostThreadMessageW.Call(w.mainthread, w32.WMApp, 0, 0)
}

func (w *webview) Bind(name string, f interface{}) error {
	v := reflect.ValueOf(f)
	if v.Kind() != reflect.Func {
		return errors.New("only functions can be bound")
	}
	if n := v.Type().NumOut(); n > 2 {
		return errors.New("function may only return a value or a value+error")
	}
	w.m.Lock()
	w.bindings[name] = f
	w.m.Unlock()

	w.Init("(function() { var name = " + jsString(name) + ";" + `
		var RPC = window._rpc = (window._rpc || {nextSeq: 1});
		window[name] = function() {
		  var seq = RPC.nextSeq++;
		  var promise = new Promise(function(resolve, reject) {
			RPC[seq] = {
			  resolve: resolve,
			  reject: reject,
			};
		  });
			window.external.invoke(JSON.stringify({
			id: seq,
			method: name,
			params: Array.prototype.slice.call(arguments),
		  }));
		  return promise;
		}
	})()`)

	return nil
}
