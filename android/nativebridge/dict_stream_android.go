// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build android && cgo

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/DMSHLabs/DictShelf/internal/server"
)

type dictionaryStream struct {
	cancel   context.CancelFunc
	messages chan json.RawMessage
	done     chan struct{}
	err      string // written before done closes
}

var dictionaryStreams = struct {
	sync.Mutex
	next   int
	active map[int]*dictionaryStream
}{active: make(map[int]*dictionaryStream)}

//export WudictGoBridgeDictStreamStart
func WudictGoBridgeDictStreamStart(progress C.int) C.int {
	query := ""
	if progress != 0 {
		query = "progress=1"
	}
	return startAndroidStream(func(ctx context.Context, app *server.Runtime, emit func([]byte)) error {
		return app.Server.DesktopStream(ctx, "", "dicts", query, emit)
	})
}

//export WudictGoBridgeRescanStreamStart
func WudictGoBridgeRescanStreamStart() C.int {
	return startAndroidStream(func(ctx context.Context, app *server.Runtime, emit func([]byte)) error {
		return app.Server.DesktopStream(ctx, "", "rescan", "", emit)
	})
}

//export WudictGoBridgeRescanIndexesStart
func WudictGoBridgeRescanIndexesStart(origin, request *C.char) C.int {
	if origin == nil || request == nil {
		return -1
	}
	baseURL, body := C.GoString(origin), C.GoString(request)
	return startAndroidStream(func(ctx context.Context, app *server.Runtime, emit func([]byte)) error {
		return app.Server.DesktopStream(ctx, baseURL, "rescanIndexes", body, emit)
	})
}

//export WudictGoBridgeIngestStart
func WudictGoBridgeIngestStart(origin, query *C.char) C.int {
	if origin == nil || query == nil {
		return -1
	}
	baseURL, values := C.GoString(origin), C.GoString(query)
	return startAndroidStream(func(ctx context.Context, app *server.Runtime, emit func([]byte)) error {
		return app.Server.DesktopStream(ctx, baseURL, "ingest", values, emit)
	})
}

//export WudictGoBridgeSearchStreamStart
func WudictGoBridgeSearchStreamStart(origin, request *C.char) C.int {
	if origin == nil || request == nil {
		return -1
	}
	baseURL, query := C.GoString(origin), C.GoString(request)
	return startAndroidStream(func(ctx context.Context, app *server.Runtime, emit func([]byte)) error {
		return app.Server.DesktopStream(ctx, baseURL, "search", query, emit)
	})
}

//export WudictGoBridgeBrowseStreamStart
func WudictGoBridgeBrowseStreamStart(method, query *C.char) C.int {
	if method == nil || query == nil {
		return -1
	}
	name, values := C.GoString(method), C.GoString(query)
	if name != "browse" && name != "browseFind" {
		return -1
	}
	return startAndroidStream(func(ctx context.Context, app *server.Runtime, emit func([]byte)) error {
		return app.Server.DesktopStream(ctx, "", name, values, emit)
	})
}

func startAndroidStream(run func(context.Context, *server.Runtime, func([]byte)) error) C.int {
	nativeRuntime.Lock()
	app := nativeRuntime.app
	nativeRuntime.Unlock()
	if app == nil {
		return -1
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &dictionaryStream{
		cancel:   cancel,
		messages: make(chan json.RawMessage, 32),
		done:     make(chan struct{}),
	}
	dictionaryStreams.Lock()
	dictionaryStreams.next++
	id := dictionaryStreams.next
	dictionaryStreams.active[id] = stream
	dictionaryStreams.Unlock()
	go func() {
		err := run(ctx, app, func(message []byte) {
			select {
			case stream.messages <- json.RawMessage(message):
			case <-ctx.Done():
			}
		})
		if err != nil {
			stream.err = err.Error()
		}
		close(stream.done)
	}()
	return C.int(id)
}

//export WudictGoBridgeDictStreamPoll
func WudictGoBridgeDictStreamPoll(rawID C.int) *C.char {
	id := int(rawID)
	dictionaryStreams.Lock()
	stream := dictionaryStreams.active[id]
	dictionaryStreams.Unlock()
	if stream == nil {
		return C.CString(`{"messages":[],"done":true,"error":"dictionary stream is unavailable"}`)
	}
	result := struct {
		Messages []json.RawMessage `json:"messages"`
		Done     bool              `json:"done"`
		Error    string            `json:"error,omitempty"`
	}{Messages: make([]json.RawMessage, 0, 32)}
	for len(result.Messages) < 32 {
		select {
		case message := <-stream.messages:
			result.Messages = append(result.Messages, message)
		default:
			goto drained
		}
	}
drained:
	if len(stream.messages) == 0 {
		select {
		case <-stream.done:
			result.Done = true
			result.Error = stream.err
			dictionaryStreams.Lock()
			if dictionaryStreams.active[id] == stream {
				delete(dictionaryStreams.active, id)
			}
			dictionaryStreams.Unlock()
			stream.cancel()
		default:
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return C.CString(`{"messages":[],"done":true,"error":"could not encode dictionary stream"}`)
	}
	return C.CString(string(data))
}

//export WudictGoBridgeDictStreamCancel
func WudictGoBridgeDictStreamCancel(rawID C.int) {
	dictionaryStreams.Lock()
	stream := dictionaryStreams.active[int(rawID)]
	delete(dictionaryStreams.active, int(rawID))
	dictionaryStreams.Unlock()
	if stream != nil {
		stream.cancel()
	}
}

func cancelDictionaryStreams() {
	dictionaryStreams.Lock()
	active := dictionaryStreams.active
	dictionaryStreams.active = make(map[int]*dictionaryStream)
	dictionaryStreams.Unlock()
	for _, stream := range active {
		stream.cancel()
	}
}
