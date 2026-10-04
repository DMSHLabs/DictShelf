// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// respStream is a response written record by record, each flushed as it is
// written: the dictionary list and search results as NDJSON (D12), an ingest's
// progress as server-sent events. One place sets what every stream needs - no
// caching, and no buffering by a reverse proxy (X-Accel-Buffering, honoured by
// nginx and Caddy), without which progress arrives all at once at the end.
// Writes are serialised, so concurrent workers may share one stream.
type respStream struct {
	mu  sync.Mutex
	w   http.ResponseWriter
	fl  http.Flusher
	enc *json.Encoder
}

// newStream starts a stream of contentType on w; false when w cannot flush,
// in which case it has answered 500 and the caller returns.
func newStream(w http.ResponseWriter, contentType string) (*respStream, bool) {
	fl, ok := w.(http.Flusher)
	if !ok {
		httpErr(w, 500, "streaming unsupported")
		return nil, false
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &respStream{w: w, fl: fl, enc: enc}, true
}

// line writes v as one NDJSON record.
func (s *respStream) line(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(v) // Encode appends '\n': one record per line
	s.fl.Flush()
}

// event writes v as one server-sent event named name.
func (s *respStream) event(name string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.w, "event: %s\ndata: ", name)
	_ = s.enc.Encode(v)   // the data line, ended by Encode's '\n'
	fmt.Fprint(s.w, "\n") // the blank line that ends the event
	s.fl.Flush()
}
