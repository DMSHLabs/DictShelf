// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const desktopMemoryThreshold = 8 << 20

type desktopResourceResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
	spill  func() (io.WriteSeeker, error)
	file   io.WriteSeeker
	size   int64
	err    error
}

func newDesktopResourceResponse(spill func() (io.WriteSeeker, error)) *desktopResourceResponse {
	return &desktopResourceResponse{header: make(http.Header), spill: spill}
}

func (w *desktopResourceResponse) Header() http.Header { return w.header }

// Flush lets the HTTP adapters use their normal streaming writers. WebView2
// receives the completed response after ServeHTTP returns, so flushing here
// only marks the in-memory response as flush-capable.
func (w *desktopResourceResponse) Flush() {}

func (w *desktopResourceResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *desktopResourceResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.err != nil {
		return 0, w.err
	}
	if w.file == nil && w.body.Len()+len(p) > desktopMemoryThreshold {
		if w.spill == nil {
			w.err = io.ErrShortWrite
			return 0, w.err
		}
		w.file, w.err = w.spill()
		if w.err != nil {
			return 0, w.err
		}
		if _, w.err = w.file.Write(w.body.Bytes()); w.err != nil {
			return 0, w.err
		}
		w.body.Reset()
	}
	var n int
	if w.file != nil {
		n, w.err = w.file.Write(p)
	} else {
		n, w.err = w.body.Write(p)
	}
	w.size += int64(n)
	return n, w.err
}

// DesktopResource serves the embedded page and asset routes to a native
// WebView without using the loopback listener. Other requests continue through
// the normal WebView network path until their direct bridge is in place.
func (s *Server) DesktopResource(method, rawURL string, requestHeaders map[string]string, spill func() (io.WriteSeeker, error)) (body []byte, status int, headers string, handled bool, spilled bool) {
	if method != http.MethodGet {
		return nil, 0, "", false, false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.User != nil ||
		u.Scheme != "http" && !(u.Scheme == "https" && strings.EqualFold(u.Host, "wudict2.local")) {
		return nil, 0, "", false, false
	}
	virtualOrigin := u.Scheme == "https" && strings.EqualFold(u.Host, "wudict2.local")
	if !virtualOrigin && !desktopWebResourceRoute(u.Path) {
		return nil, 0, "", false, false
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, "", false, false
	}
	// An incoming server request always has a non-nil Body. Keep that
	// contract for handlers that read the body even on GET requests.
	req.Body = http.NoBody
	req.Host = u.Host
	if u.Scheme == "https" {
		// The virtual host is a trusted in-process origin; route authorization
		// still sees a loopback Host and the desktop capability token below.
		req.Host = "127.0.0.1"
	}
	req.RemoteAddr = "127.0.0.1:0"
	// Native WebView requests are trusted in-process calls. Preserve the
	// server's normal route authorization by presenting its capability token.
	if s.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.AuthToken)
	}
	if requestHeaders != nil {
		for _, name := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match", "If-Unmodified-Since", "If-Match"} {
			if value := requestHeaders[name]; value != "" {
				req.Header.Set(name, value)
			}
		}
	}
	response := newDesktopResourceResponse(spill)
	s.ServeHTTP(response, req)
	if response.err != nil {
		return nil, 0, "", false, false
	}
	if response.header.Get("Content-Length") == "" {
		response.header.Set("Content-Length", strconv.FormatInt(response.size, 10))
	}
	var header bytes.Buffer
	if err := response.header.Write(&header); err != nil {
		return nil, 0, "", false, false
	}
	return response.body.Bytes(), response.status, strings.TrimRight(header.String(), "\r\n"), true, response.file != nil
}

func desktopWebResourceRoute(path string) bool {
	switch path {
	case "/", "/setup", "/groups", "/lemmas", "/browse", "/double-tap-probe", "/favicon.ico":
		return true
	default:
		if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/style/") || strings.HasPrefix(path, "/files/") {
			return true
		}
		return strings.HasPrefix(path, "/res/")
	}
}
