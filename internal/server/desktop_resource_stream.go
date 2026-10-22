// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type desktopStreamMeta struct {
	status  int
	headers string
}

type desktopStreamResponse struct {
	header http.Header
	writer *io.PipeWriter
	meta   chan desktopStreamMeta
	wrote  bool
}

func (w *desktopStreamResponse) Header() http.Header { return w.header }

func (w *desktopStreamResponse) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	var header bytes.Buffer
	_ = w.header.Write(&header)
	w.meta <- desktopStreamMeta{status: status, headers: strings.TrimRight(header.String(), "\r\n")}
}

func (w *desktopStreamResponse) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.writer.Write(p)
}

// DesktopMediaStream serves a resource through an in-process pipe. The caller
// receives headers before the body is copied, so even a multi-gigabyte open
// range can start playing without first materializing the entire response.
func (s *Server) DesktopMediaStream(rawURL string, requestHeaders map[string]string) (io.ReadCloser, int, string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "dictshelf.localhost") || u.User != nil ||
		!strings.HasPrefix(u.Path, "/res/") && !strings.HasPrefix(u.Path, "/files/") {
		return nil, 0, "", false
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, "", false
	}
	req.Body = http.NoBody
	req.Host = "127.0.0.1"
	req.RemoteAddr = "127.0.0.1:0"
	if s.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.AuthToken)
	}
	for _, name := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match", "If-Unmodified-Since", "If-Match"} {
		if value := requestHeaders[name]; value != "" {
			req.Header.Set(name, value)
		}
	}
	reader, writer := io.Pipe()
	response := &desktopStreamResponse{header: make(http.Header), writer: writer, meta: make(chan desktopStreamMeta, 1)}
	response.header.Set("Referrer-Policy", "no-referrer")
	go func() {
		defer writer.Close()
		defer func() {
			if !response.wrote {
				response.WriteHeader(http.StatusOK)
			}
		}()
		if strings.HasPrefix(u.Path, "/res/") {
			s.callDesktopOperation(response, req, func() { s.desktopDictionaryResource(response, req) })
			return
		}
		name := strings.TrimPrefix(u.Path, userFileURL)
		file, info := s.openUserFile(name)
		if file == nil {
			http.NotFound(response, req)
			return
		}
		defer file.Close()
		response.header.Set("Content-Type", resolveMIME(name, "application/octet-stream"))
		response.header.Set("X-Content-Type-Options", "nosniff")
		response.header.Set("Cache-Control", "no-cache")
		http.ServeContent(response, req, name, info.ModTime(), file)
	}()
	meta := <-response.meta
	return reader, meta.status, meta.headers, true
}
