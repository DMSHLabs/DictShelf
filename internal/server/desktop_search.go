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
	"net/url"
	"strings"
)

// DesktopSearch runs the existing search route in-process and forwards each
// NDJSON record to the desktop WebView bridge. The HTTP route remains the
// browser and non-Windows transport; both paths enter the same handler.
func (s *Server) DesktopSearch(ctx context.Context, baseURL, query string, emit func([]byte)) error {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return errors.New("invalid desktop server address")
	}
	base.Path, base.RawPath, base.RawQuery, base.Fragment = "/api/search", "", query, ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return err
	}
	req.Host = base.Host
	req.RemoteAddr = "127.0.0.1:0"
	w := &desktopSearchWriter{header: make(http.Header), emit: emit}
	s.ServeHTTP(w, req)
	return w.finish()
}

type desktopSearchWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
	line   []byte
	emit   func([]byte)
}

func (w *desktopSearchWriter) Header() http.Header { return w.header }

func (w *desktopSearchWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *desktopSearchWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !strings.HasPrefix(w.header.Get("Content-Type"), "application/x-ndjson") {
		return w.body.Write(p)
	}
	w.line = append(w.line, p...)
	for {
		i := bytes.IndexByte(w.line, '\n')
		if i < 0 {
			break
		}
		line := bytes.TrimSpace(w.line[:i])
		if len(line) > 0 && w.emit != nil {
			w.emit(bytes.Clone(line))
		}
		w.line = w.line[i+1:]
	}
	return len(p), nil
}

func (w *desktopSearchWriter) Flush() {}

func (w *desktopSearchWriter) finish() error {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if len(bytes.TrimSpace(w.line)) > 0 && w.emit != nil {
		w.emit(bytes.Clone(bytes.TrimSpace(w.line)))
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
	return fmt.Errorf("search failed (HTTP %d)", w.status)
}
