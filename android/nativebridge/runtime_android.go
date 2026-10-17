// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build android && cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/wuweidict/wudict/internal/config"
	_ "github.com/wuweidict/wudict/internal/format/bgl"
	_ "github.com/wuweidict/wudict/internal/format/dsl"
	_ "github.com/wuweidict/wudict/internal/format/mdx"
	_ "github.com/wuweidict/wudict/internal/format/slob"
	_ "github.com/wuweidict/wudict/internal/format/stardict"
	_ "github.com/wuweidict/wudict/internal/format/wmd"
	_ "github.com/wuweidict/wudict/internal/format/zim"
	"github.com/wuweidict/wudict/internal/intake"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/server"
)

//export WudictGoBridgeConfigSnapshot
func WudictGoBridgeConfigSnapshot() *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return nil
	}
	data, err := json.Marshal(app.Server.ConfigSnapshot(true, "127.0.0.1"))
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgePrefsSnapshot
func WudictGoBridgePrefsSnapshot() *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return nil
	}
	data, err := json.Marshal(app.Server.PrefsSnapshot())
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeBrowsePage
func WudictGoBridgeBrowsePage(query *C.char) *C.char {
	if query == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"body":{"error":"Go runtime is unavailable"}}`)
	}
	value, err := app.Server.DesktopJSON("", "browsePage", C.GoString(query))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 500, "body": map[string]string{"error": err.Error()}})
		return C.CString(string(data))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return C.CString(`{"status":500,"body":{"error":"could not encode browse page"}}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeBrowseFindPage
func WudictGoBridgeBrowseFindPage(query *C.char) *C.char {
	if query == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", "browseFindPage", C.GoString(query))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 500, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return C.CString(`{"status":500,"error":"could not encode Browse results"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeBrowseDiagnostic
func WudictGoBridgeBrowseDiagnostic(payload *C.char) *C.char {
	if payload == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return nil
	}
	if _, err := app.Server.DesktopJSON("", "browseDiagnostic", C.GoString(payload)); err != nil {
		return nil
	}
	return C.CString("ok")
}

//export WudictGoBridgeAbout
func WudictGoBridgeAbout(origin, dictionary *C.char) *C.char {
	if origin == nil || dictionary == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return nil
	}
	value, err := app.Server.DesktopJSON(C.GoString(origin), "about", C.GoString(dictionary))
	if err != nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeOrphans
func WudictGoBridgeOrphans(action, body *C.char) *C.char {
	if action == nil || body == nil {
		return nil
	}
	method := "orphans"
	if C.GoString(action) == "resolve" {
		method = "resolveOrphans"
	} else if C.GoString(action) != "get" {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"body":{"error":"Go runtime is unavailable"}}`)
	}
	value, err := app.Server.DesktopJSON("", method, C.GoString(body))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 400, "body": map[string]string{"error": err.Error()}})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"status": 200, "body": value})
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeRemoveLibrary
func WudictGoBridgeRemoveLibrary(query *C.char) *C.char {
	if query == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"body":{"error":"Go runtime is unavailable"}}`)
	}
	value, err := app.Server.DesktopJSON("", "removeLibrary", C.GoString(query))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 400, "body": map[string]string{"error": err.Error()}})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"status": 200, "body": value})
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeLooks
func WudictGoBridgeLooks(argument *C.char) *C.char {
	if argument == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"body":{"error":"Go runtime is unavailable"}}`)
	}
	value, err := app.Server.DesktopJSON("", "looks", C.GoString(argument))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 400, "body": map[string]string{"error": err.Error()}})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"status": 200, "body": value})
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeGroups
func WudictGoBridgeGroups(action, argument *C.char) *C.char {
	if action == nil || argument == nil {
		return nil
	}
	method := "groups"
	operation := C.GoString(action)
	switch operation {
	case "get":
	case "save":
		method = "saveGroups"
	case "reset":
		method = "resetGroups"
	default:
		// Dictionary Groups uses the same shared operations as the HTTP
		// handlers. Keep the JNI surface limited to this explicit allowlist.
		switch operation {
		case "userGroups", "createGroup", "renameGroup", "deleteGroup",
			"setGroupMember", "replaceGroupMembership", "addGroupMembers",
			"linkGroup", "saveGroupOrder":
			method = operation
		default:
			return nil
		}
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", method, C.GoString(argument))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "value": value})
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeLemmas
func WudictGoBridgeLemmas(action, code *C.char) *C.char {
	if action == nil || code == nil {
		return nil
	}
	method := "lemmas"
	switch C.GoString(action) {
	case "get":
	case "install":
		method = "installLemma"
	case "remove":
		method = "removeLemma"
	default:
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", method, C.GoString(code))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "value": value})
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeSetup
func WudictGoBridgeSetup(action, argument *C.char) *C.char {
	if action == nil || argument == nil {
		return nil
	}
	method := ""
	switch C.GoString(action) {
	case "setup", "folderSetup", "indexDefaults", "howto":
		method = C.GoString(action)
	default:
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", method, C.GoString(argument))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "value": value})
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeIntake
func WudictGoBridgeIntake(action, argument *C.char) *C.char {
	if action == nil || argument == nil {
		return nil
	}
	method := ""
	switch C.GoString(action) {
	case "status":
		method = "intakeStatus"
	case "url":
		method = "intakeURL"
	case "path":
		method = "intakePath"
	case "confirm":
		method = "intakeConfirm"
	case "cancel":
		method = "intakeCancel"
	case "fileStart":
		method = "intakeFileStart"
	case "fileChunk":
		method = "intakeFileChunk"
	case "fileFinish":
		method = "intakeFileFinish"
	case "fileCancel":
		method = "intakeFileCancel"
	default:
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", method, C.GoString(argument))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error(), "busy": errors.Is(err, intake.ErrBusy)})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "value": value})
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgePower
func WudictGoBridgePower(state *C.char) C.int {
	if state == nil {
		return 0
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return 0
	}
	if _, err := app.Server.DesktopJSON("", "power", C.GoString(state)); err != nil {
		return 0
	}
	return 1
}

//export WudictGoBridgeReindex
func WudictGoBridgeReindex(action *C.char) *C.char {
	if action == nil {
		return nil
	}
	method := ""
	switch C.GoString(action) {
	case "get":
		method = "reindex"
	case "start":
		method = "startReindex"
	case "stop":
		method = "stopReindex"
	default:
		return C.CString(`{"status":400,"body":{"error":"invalid reindex action"}}`)
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"body":{"error":"Go runtime is unavailable"}}`)
	}
	value, err := app.Server.DesktopJSON("", method, "")
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 409, "body": map[string]string{"error": err.Error()}})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"status": 200, "body": value})
	if err != nil {
		return C.CString(`{"status":500,"body":{"error":"could not encode reindex status"}}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeIndexWork
func WudictGoBridgeIndexWork(action, id, body *C.char) *C.char {
	if action == nil || id == nil || body == nil {
		return nil
	}
	method, argument := "", ""
	switch C.GoString(action) {
	case "get":
		method, argument = "indexWork", C.GoString(id)
	case "start":
		method, argument = "startIndexWork", C.GoString(body)
	case "startSingle":
		method, argument = "startSingleIndexWork", C.GoString(body)
	case "stop":
		method = "stopIndexWork"
	default:
		return C.CString(`{"status":400,"body":{"error":"invalid index action"}}`)
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"body":{"error":"Go runtime is unavailable"}}`)
	}
	value, err := app.Server.DesktopJSON("", method, argument)
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 409, "body": map[string]string{"error": err.Error()}})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"status": 200, "body": value})
	if err != nil {
		return C.CString(`{"status":500,"body":{"error":"could not encode index status"}}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgePresetsSnapshot
func WudictGoBridgePresetsSnapshot() *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return nil
	}
	value, err := app.Server.DesktopJSON("", "presets", "")
	if err != nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeSavePreset
func WudictGoBridgeSavePreset(body *C.char) *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"status":503,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", "savePreset", C.GoString(body))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "status": 400, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "status": 200, "value": value})
	if err != nil {
		return C.CString(`{"ok":false,"status":500,"error":"could not encode presets"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeStyleSnapshot
func WudictGoBridgeStyleSnapshot() *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return nil
	}
	value, err := app.Server.DesktopJSON("", "style", "")
	if err != nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeSaveStyle
func WudictGoBridgeSaveStyle(argument *C.char) *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"status":503,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", "saveStyle", C.GoString(argument))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"status": 500, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return C.CString(`{"status":500,"error":"could not encode style response"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeUserFilesSnapshot
func WudictGoBridgeUserFilesSnapshot() *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return nil
	}
	value, err := app.Server.DesktopJSON("", "userFiles", "")
	if err != nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export WudictGoBridgeUserFileOperation
func WudictGoBridgeUserFileOperation(method, argument *C.char) *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"status":503,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", C.GoString(method), C.GoString(argument))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "status": 400, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "status": 200, "value": value})
	if err != nil {
		return C.CString(`{"ok":false,"status":500,"error":"could not encode file response"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeUserStorage
func WudictGoBridgeUserStorage(method, argument *C.char) *C.char {
	if method == nil || argument == nil {
		return C.CString(`{"ok":false,"error":"invalid settings operation"}`)
	}
	operation := C.GoString(method)
	switch operation {
	case "storageState", "storageRead", "storageImport", "storageWrite":
	default:
		return C.CString(`{"ok":false,"error":"unsupported settings operation"}`)
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", operation, C.GoString(argument))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "value": value})
	if err != nil {
		return C.CString(`{"ok":false,"error":"could not encode settings response"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeSavePrefs
func WudictGoBridgeSavePrefs(body *C.char) *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"status":503,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.SavePrefs(strings.NewReader(C.GoString(body)))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "status": 400, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "status": 200, "value": value})
	if err != nil {
		return C.CString(`{"ok":false,"status":500,"error":"could not encode preferences"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeSaveLanguage
func WudictGoBridgeSaveLanguage(body *C.char) *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"status":503,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", "saveLanguage", C.GoString(body))
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "status": 400, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "status": 200, "value": value})
	if err != nil {
		return C.CString(`{"ok":false,"status":500,"error":"could not encode language response"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeSystemLogSnapshot
func WudictGoBridgeSystemLogSnapshot() *C.char {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"ok":false,"error":"Go runtime is unavailable"}`)
	}
	value, err := app.Server.DesktopJSON("", "systemLog", "")
	if err != nil {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return C.CString(string(data))
	}
	data, err := json.Marshal(map[string]any{"ok": true, "value": value})
	if err != nil {
		return C.CString(`{"ok":false,"error":"could not encode system log"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeSystemLogOptions
func WudictGoBridgeSystemLogOptions(enabled, baseEvents, webEvents C.int) C.int {
	logx.ConfigureSystemLog(enabled != 0, baseEvents != 0, webEvents != 0)
	return 1
}

//export WudictGoBridgeIndexWorkers
func WudictGoBridgeIndexWorkers(workers C.int) C.int {
	if workers < 1 || int(workers) > config.ParseWorkers("all") {
		return 0
	}
	server.SetIndexWorkers(int(workers))
	return 1
}

//export WudictGoBridgeWebResource
func WudictGoBridgeWebResource(rawURL, headersJSON *C.char) *C.char {
	if rawURL == nil || headersJSON == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"handled":false,"status":503}`)
	}
	requestHeaders := map[string]string{}
	if err := json.Unmarshal([]byte(C.GoString(headersJSON)), &requestHeaders); err != nil {
		return C.CString(`{"handled":false,"status":400}`)
	}
	body, status, headers, handled, spilled := app.Server.DesktopResource("GET", C.GoString(rawURL), requestHeaders, nil)
	if spilled {
		return C.CString(`{"handled":false,"status":500}`)
	}
	data, err := json.Marshal(map[string]any{"handled": handled, "status": status, "headers": headers, "body": body})
	if err != nil {
		return C.CString(`{"handled":false,"status":500}`)
	}
	return C.CString(string(data))
}

var mediaStreams struct {
	sync.Mutex
	next    atomic.Int64
	readers map[int64]io.ReadCloser
}

//export WudictGoBridgeMediaOpen
func WudictGoBridgeMediaOpen(rawURL, headersJSON *C.char) *C.char {
	if rawURL == nil || headersJSON == nil {
		return nil
	}
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return C.CString(`{"handled":false}`)
	}
	requestHeaders := map[string]string{}
	if json.Unmarshal([]byte(C.GoString(headersJSON)), &requestHeaders) != nil {
		return C.CString(`{"handled":false}`)
	}
	reader, status, headers, handled := app.Server.DesktopMediaStream(C.GoString(rawURL), requestHeaders)
	if !handled {
		return C.CString(`{"handled":false}`)
	}
	// Android WebResourceResponse cannot represent HTTP redirects or 304.
	// Let WebView use its ordinary network path for conditional cache hits.
	if status >= 300 && status < 400 {
		_ = reader.Close()
		return C.CString(`{"handled":false}`)
	}
	id := mediaStreams.next.Add(1)
	mediaStreams.Lock()
	if mediaStreams.readers == nil {
		mediaStreams.readers = make(map[int64]io.ReadCloser)
	}
	mediaStreams.readers[id] = reader
	mediaStreams.Unlock()
	data, err := json.Marshal(map[string]any{"handled": true, "stream": id, "status": status, "headers": headers})
	if err != nil {
		WudictGoBridgeMediaClose(C.longlong(id))
		return C.CString(`{"handled":false}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeMediaRead
func WudictGoBridgeMediaRead(handle C.longlong, dst *C.char, size C.int) C.int {
	if dst == nil || size <= 0 || size > 1<<20 {
		return -2
	}
	mediaStreams.Lock()
	reader := mediaStreams.readers[int64(handle)]
	mediaStreams.Unlock()
	if reader == nil {
		return -1
	}
	n, err := reader.Read(unsafe.Slice((*byte)(unsafe.Pointer(dst)), int(size)))
	if n > 0 {
		return C.int(n)
	}
	if err != nil && err != io.EOF {
		return -2
	}
	return -1
}

//export WudictGoBridgeMediaClose
func WudictGoBridgeMediaClose(handle C.longlong) {
	mediaStreams.Lock()
	reader := mediaStreams.readers[int64(handle)]
	delete(mediaStreams.readers, int64(handle))
	mediaStreams.Unlock()
	if reader != nil {
		_ = reader.Close()
	}
}

func closeMediaStreams() {
	mediaStreams.Lock()
	readers := mediaStreams.readers
	mediaStreams.readers = nil
	mediaStreams.Unlock()
	for _, reader := range readers {
		_ = reader.Close()
	}
}

var nativeRuntime struct {
	sync.Mutex
	app *server.Runtime
}

// A single process must never open the prepared library twice.
//
//export WudictGoBridgeRuntimeStart
func WudictGoBridgeRuntimeStart(path, flagsJSON, token, version *C.char) C.int {
	nativeRuntime.Lock()
	defer nativeRuntime.Unlock()
	if nativeRuntime.app != nil {
		return 0
	}
	flags := map[string]string{}
	if err := json.Unmarshal([]byte(C.GoString(flagsJSON)), &flags); err != nil {
		return -1
	}
	configPath := C.GoString(path)
	// The old child received this directory as HOME. Keep default lemma and
	// other user-data paths identical when Go runs inside the Android process.
	if err := os.Setenv("HOME", filepath.Dir(filepath.Dir(configPath))); err != nil {
		return -8
	}
	cfg, err := config.Load(configPath, flags)
	if err != nil {
		return -2
	}
	if _, err = server.ConfigureLibrary(cfg); err != nil {
		return -3
	}
	if _, err = server.RecoverLibrary(cfg); err != nil {
		logx.Warn("could not tidy the library: %v", err)
	}
	app, err := server.NewRuntime(cfg, configPath, C.GoString(token), C.GoString(version))
	if err != nil {
		return -5
	}
	nativeRuntime.app = app
	return 1
}

//export WudictGoBridgeRuntimeStop
func WudictGoBridgeRuntimeStop() C.int {
	cancelDictionaryStreams()
	closeMediaStreams()
	nativeRuntime.Lock()
	defer nativeRuntime.Unlock()
	if nativeRuntime.app == nil {
		return 0
	}
	// The registry remains process-owned. Android exits this process after
	// stopping work and starts a fresh process for changed settings.
	return 1
}
