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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/wuweidict/wudict/internal/logx"
)

// DesktopStream dispatches desktop streams. Search calls the shared Go
// operation directly; the other streams still enter their existing routes.
func (s *Server) DesktopStream(ctx context.Context, baseURL, method, query string, emit func([]byte)) error {
	if method == "search" {
		var request SearchRequest
		if err := json.Unmarshal([]byte(query), &request); err != nil {
			return fmt.Errorf("invalid native search request: %w", err)
		}
		return s.desktopSearchRequest(ctx, baseURL, request, emit)
	}
	path := ""
	verb := http.MethodGet
	var body io.Reader
	switch method {
	case "dicts":
		path = "/api/dicts"
	case "ingest":
		path = "/api/ingest"
	case "rescan":
		path = "/api/rescan"
	case "rescanIndexes":
		path, verb = "/api/rescan", http.MethodPost
		body = strings.NewReader(query)
		query = "stream=1"
	default:
		return fmt.Errorf("unsupported desktop stream %q", method)
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return errors.New("invalid desktop server address")
	}
	base.Path, base.RawPath, base.RawQuery, base.Fragment = path, "", query, ""
	req, err := http.NewRequestWithContext(ctx, verb, base.String(), body)
	if err != nil {
		return err
	}
	req.Host = base.Host
	req.RemoteAddr = "127.0.0.1:0"
	if verb == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	w := &desktopStreamWriter{header: make(http.Header), emit: emit}
	s.ServeHTTP(w, req)
	return w.finish()
}

// DesktopSearch accepts a legacy query string and calls the shared search
// operation directly. WebView2 uses the typed JSON request via DesktopStream.
func (s *Server) DesktopSearch(ctx context.Context, baseURL, query string, emit func([]byte)) error {
	parameters, err := url.ParseQuery(query)
	if err != nil {
		return err
	}
	return s.desktopSearchRequest(ctx, baseURL, searchRequestFromQuery(parameters), emit)
}

// Search enters the shared Go operation directly. Encoding events is the
// bridge's only output adaptation; no request, router or NDJSON parser runs.
func (s *Server) desktopSearchRequest(ctx context.Context, baseURL string, request SearchRequest, emit func([]byte)) (resultErr error) {
	defer func() {
		if p := recover(); p != nil {
			logx.Warn("panic in native search: %v", p)
			resultErr = fmt.Errorf("internal error: %v", p)
		}
	}()
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return errors.New("invalid desktop server address")
	}
	request.Origin = base.Scheme + "://" + base.Host
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	return s.Search(ctx, request, func(event SearchEvent) error {
		buffer.Reset()
		if err := encoder.Encode(event); err != nil {
			return err
		}
		if emit != nil {
			emit(bytes.Clone(bytes.TrimSpace(buffer.Bytes())))
		}
		return nil
	})
}

// DesktopJSON calls an explicitly supported JSON endpoint in-process for the desktop
// bridge. The endpoint still runs through the same server middleware and
// handler as an HTTP client.
func (s *Server) DesktopJSON(baseURL, method, argument string) (any, error) {
	path := ""
	query := ""
	verb := http.MethodGet
	var body io.Reader
	switch method {
	case "groups":
		path = "/api/groups"
	case "userGroups":
		path = "/api/user-groups"
	case "createGroup", "renameGroup", "deleteGroup", "setGroupMember", "replaceGroupMembership", "addGroupMembers", "linkGroup", "saveGroupOrder":
		route := map[string]struct{ path, verb string }{
			"createGroup":            {"/api/user-groups", http.MethodPost},
			"renameGroup":            {"/api/user-groups/name", http.MethodPut},
			"deleteGroup":            {"/api/user-groups", http.MethodDelete},
			"setGroupMember":         {"/api/user-groups/member", http.MethodPut},
			"replaceGroupMembership": {"/api/user-groups/membership", http.MethodPut},
			"addGroupMembers":        {"/api/user-groups/members", http.MethodPut},
			"linkGroup":              {"/api/user-groups/link", http.MethodPut},
			"saveGroupOrder":         {"/api/user-groups/order", http.MethodPut},
		}[method]
		path, verb, body = route.path, route.verb, strings.NewReader(argument)
	case "about":
		path = "/api/about"
		query = url.Values{"dict": []string{argument}}.Encode()
	case "reindex":
		path = "/api/reindex"
	case "startReindex", "stopReindex":
		path = "/api/reindex"
		verb = http.MethodPost
		if method == "stopReindex" {
			verb = http.MethodDelete
		}
	case "startIndexWork", "startSingleIndexWork":
		path, verb = "/api/index-work", http.MethodPost
		body = strings.NewReader(argument)
		if method == "startSingleIndexWork" {
			query = "single=1"
		}
	case "stopIndexWork":
		path, verb = "/api/index-work", http.MethodDelete
		query = url.Values{"id": []string{"all"}}.Encode()
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
	req, err := http.NewRequest(verb, base.String(), body)
	if err != nil {
		return nil, err
	}
	req.Host = base.Host
	req.RemoteAddr = "127.0.0.1:0"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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
