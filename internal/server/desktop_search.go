// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
)

// DesktopStream runs an existing streaming route in-process and forwards each
// NDJSON record to the desktop WebView bridge. The HTTP routes remain the
// browser and non-Windows transport; both paths enter the same handlers.
func (s *Server) DesktopStream(ctx context.Context, baseURL, method, query string, emit func([]byte)) error {
	path := ""
	switch method {
	case "search":
		path = "/api/search"
	case "dicts":
		path = "/api/dicts"
	case "ingest":
		path = "/api/ingest"
	default:
		return fmt.Errorf("unsupported desktop stream %q", method)
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return errors.New("invalid desktop server address")
	}
	base.Path, base.RawPath, base.RawQuery, base.Fragment = path, "", query, ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return err
	}
	req.Host = base.Host
	req.RemoteAddr = "127.0.0.1:0"
	w := &desktopStreamWriter{header: make(http.Header), emit: emit}
	s.ServeHTTP(w, req)
	return w.finish()
}

// DesktopSearch remains a narrow convenience for callers that only need the
// search stream.
func (s *Server) DesktopSearch(ctx context.Context, baseURL, query string, emit func([]byte)) error {
	return s.DesktopStream(ctx, baseURL, "search", query, emit)
}

// DesktopJSON calls one read-only JSON endpoint in-process for the desktop
// bridge. The endpoint still runs through the same server middleware and
// handler as an HTTP client.
func (s *Server) DesktopJSON(baseURL, method, argument string) (any, error) {
	path := ""
	query := ""
	switch method {
	case "groups":
		path = "/api/groups"
	case "userGroups":
		path = "/api/user-groups"
	case "about":
		path = "/api/about"
		query = url.Values{"dict": []string{argument}}.Encode()
	case "reindex":
		path = "/api/reindex"
	case "indexWork":
		path = "/api/index-work"
		if argument != "" {
			query = url.Values{"id": []string{argument}}.Encode()
		}
	default:
		return nil, fmt.Errorf("unsupported desktop request %q", method)
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return nil, errors.New("invalid desktop server address")
	}
	base.Path, base.RawPath, base.RawQuery, base.Fragment = path, "", query, ""
	req, err := http.NewRequest(http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Host = base.Host
	req.RemoteAddr = "127.0.0.1:0"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code >= 400 {
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &payload) == nil && payload.Error != "" {
			return nil, errors.New(payload.Error)
		}
		return nil, errors.New(strings.TrimSpace(rec.Body.String()))
	}
	var result any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	return result, nil
}

type desktopStreamWriter struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	line      []byte
	event     string
	eventData []string
	emit      func([]byte)
}

func (w *desktopStreamWriter) Header() http.Header { return w.header }

func (w *desktopStreamWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *desktopStreamWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	contentType := w.header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/x-ndjson") {
		w.writeNDJSON(p)
		return len(p), nil
	}
	if strings.HasPrefix(contentType, "text/event-stream") {
		w.writeSSE(p)
		return len(p), nil
	}
	return w.body.Write(p)
}

func (w *desktopStreamWriter) writeNDJSON(p []byte) {
	w.line = append(w.line, p...)
	for {
		i := bytes.IndexByte(w.line, '\n')
		if i < 0 {
			return
		}
		line := bytes.TrimSpace(w.line[:i])
		if len(line) > 0 && w.emit != nil {
			w.emit(bytes.Clone(line))
		}
		w.line = w.line[i+1:]
	}
}

func (w *desktopStreamWriter) writeSSE(p []byte) {
	w.line = append(w.line, p...)
	for {
		i := bytes.IndexByte(w.line, '\n')
		if i < 0 {
			return
		}
		line := bytes.TrimSuffix(w.line[:i], []byte{'\r'})
		w.line = w.line[i+1:]
		if len(line) == 0 {
			w.emitSSE()
			continue
		}
		if line[0] == ':' { // SSE comment/keepalive
			continue
		}
		field, value, _ := bytes.Cut(line, []byte{':'})
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch string(field) {
		case "event":
			w.event = string(value)
		case "data":
			w.eventData = append(w.eventData, string(value))
		}
	}
}

func (w *desktopStreamWriter) emitSSE() {
	if len(w.eventData) == 0 {
		w.event = ""
		return
	}
	data := []byte(strings.Join(w.eventData, "\n"))
	if !json.Valid(data) {
		data, _ = json.Marshal(string(data))
	}
	if w.event == "" {
		w.event = "message"
	}
	message, err := json.Marshal(struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}{Event: w.event, Data: data})
	if err == nil && w.emit != nil {
		w.emit(message)
	}
	w.event = ""
	w.eventData = nil
}

func (w *desktopStreamWriter) Flush() {}

func (w *desktopStreamWriter) finish() error {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	switch {
	case strings.HasPrefix(w.header.Get("Content-Type"), "application/x-ndjson"):
		if len(bytes.TrimSpace(w.line)) > 0 && w.emit != nil {
			w.emit(bytes.Clone(bytes.TrimSpace(w.line)))
		}
	case strings.HasPrefix(w.header.Get("Content-Type"), "text/event-stream"):
		if len(w.line) > 0 {
			w.writeSSE([]byte{'\n'})
		}
		w.emitSSE()
	}
	if w.status < 400 {
		return nil
	}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(w.body.Bytes(), &payload) == nil && payload.Error != "" {
		return errors.New(payload.Error)
	}
	return fmt.Errorf("request failed (HTTP %d)", w.status)
}
