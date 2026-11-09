// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"compress/gzip"
	"net"
	"net/http"
	"strings"
	"sync"
)

// The page is one 845 kB document; a search page is 4.5-7.8 MB of NDJSON. On a
// network they are worth compressing on the wire, and on loopback they are not:
// the bytes never leave the machine, and gzip would only cost CPU at both ends.
// So the static page is compressed once and cached per ETag, and a stream is
// compressed only for a peer that is not loopback.

var gzipPool = sync.Pool{New: func() any {
	zw, _ := gzip.NewWriterLevel(new(bytes.Buffer), gzip.BestSpeed)
	return zw
}}

type gzipResponseWriter struct {
	http.ResponseWriter
	zw *gzip.Writer
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) { return g.zw.Write(p) }

// Flush keeps an NDJSON stream a stream: each record reaches the client as it
// is written, through the compressor.
func (g *gzipResponseWriter) Flush() {
	_ = g.zw.Flush()
	if fl, ok := g.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]), "gzip") {
			return true
		}
	}
	return false
}

// remotePeer is false for an in-process or localhost request. An empty address
// (an adapter that never went through a socket) is treated as local: it is the
// desktop's in-process path, where compression can only lose.
func remotePeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && !ip.IsLoopback()
}

// compressStream wraps a stream writer for a remote, gzip-capable client. The
// returned function must be deferred: it finishes the compressed stream.
func compressStream(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, func()) {
	if !acceptsGzip(r) || !remotePeer(r) {
		return w, func() {}
	}
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Add("Vary", "Accept-Encoding")
	w.Header().Del("Content-Length")
	zw := gzipPool.Get().(*gzip.Writer)
	zw.Reset(w)
	return &gzipResponseWriter{ResponseWriter: w, zw: zw}, func() {
		_ = zw.Close()
		gzipPool.Put(zw)
	}
}

// The gzipped page is kept, not recomputed: the bytes are fixed for a given
// ETag, and a page load is not the place to spend 10 ms of CPU per request.
var pageGzipCache struct {
	sync.Mutex
	pages map[string][]byte
}

func gzippedPage(etag string, page []byte) []byte {
	pageGzipCache.Lock()
	defer pageGzipCache.Unlock()
	if cached, ok := pageGzipCache.pages[etag]; ok {
		return cached
	}
	var buffer bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
	if err != nil {
		return nil
	}
	if _, err = zw.Write(page); err != nil {
		return nil
	}
	if err = zw.Close(); err != nil {
		return nil
	}
	if pageGzipCache.pages == nil {
		pageGzipCache.pages = make(map[string][]byte)
	}
	if len(pageGzipCache.pages) >= 4 {
		for key := range pageGzipCache.pages {
			delete(pageGzipCache.pages, key)
			break
		}
	}
	pageGzipCache.pages[etag] = buffer.Bytes()
	return buffer.Bytes()
}

// gzipPageETag names the compressed representation: an ETag identifies bytes,
// and the compressed bytes are not the ones the plain tag described.
func gzipPageETag(etag string) string {
	return strings.TrimSuffix(etag, `"`) + `-gzip"`
}
