// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wuweidict/wudict/internal/logx"
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

// DesktopResource adapts GET requests from the in-process WebView to Go
// operations. Mutating operations use the native bridge rather than HTTP.
func (s *Server) DesktopResource(method, rawURL string, requestHeaders map[string]string, spill func() (io.WriteSeeker, error)) (body []byte, status int, headers string, handled bool, spilled bool) {
	if method != http.MethodGet {
		return nil, 0, "", false, false
	}
	u, err := url.Parse(rawURL)
	legacyVirtualOrigin := err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, "wudict2.local")
	appVirtualOrigin := err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, "wudict2.localhost")
	virtualOrigin := appVirtualOrigin ||
		err == nil && u.Scheme == "http" && strings.EqualFold(u.Host, "wudict2.localhost") ||
		legacyVirtualOrigin
	if err != nil || u.Host == "" || u.User != nil ||
		u.Scheme != "http" && !legacyVirtualOrigin && !appVirtualOrigin {
		return nil, 0, "", false, false
	}
	localDesktopOrigin := u.Scheme == "http" && u.Hostname() == "127.0.0.1"
	if !virtualOrigin && !localDesktopOrigin && !desktopWebResourceRoute(u.Path) {
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
	if virtualOrigin {
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
		for _, name := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match", "If-Unmodified-Since", "If-Match", "Accept-Language"} {
			if value := requestHeaders[name]; value != "" {
				req.Header.Set(name, value)
			}
		}
	}
	response := newDesktopResourceResponse(spill)
	response.header.Set("Referrer-Policy", "no-referrer")
	if localDesktopOrigin {
		// The old loopback origin is used once to read browser storage during
		// the origin migration. Give it an inert document; application pages
		// and APIs are served only on the in-process origin and native bridge.
		response.header.Set("Cache-Control", "no-store")
		if u.Path == "/__storage-migration" {
			response.header.Set("Content-Type", "text/html; charset=utf-8")
			_, _ = response.Write([]byte(desktopStorageMigrationPage))
		} else if u.Path == "/" {
			response.header.Set("Content-Type", "text/html; charset=utf-8")
			_, _ = response.Write([]byte("<!doctype html><html><head><meta charset=utf-8></head><body></body></html>"))
		} else {
			http.NotFound(response, req)
		}
	} else if legacyVirtualOrigin {
		// Keep the former application origin only for a retried storage import.
		response.header.Set("Cache-Control", "no-store")
		if u.Path == "/__storage-migration" {
			response.header.Set("Content-Type", "text/html; charset=utf-8")
			_, _ = response.Write([]byte(desktopStorageMigrationPage))
		} else {
			http.NotFound(response, req)
		}
	} else if u.Path == "/__storage-migration" {
		response.header.Set("Content-Type", "text/html; charset=utf-8")
		response.header.Set("Cache-Control", "no-store")
		_, _ = response.Write([]byte(desktopStorageMigrationPage))
	} else if page, etag, cache, ok := s.desktopPage(u.Path, u.Query(), requestHeaders["Accept-Language"]); ok {
		response.header.Set("Content-Type", "text/html; charset=utf-8")
		response.header.Set("Cache-Control", cache)
		response.header.Add("Vary", "Accept-Language")
		if etag != "" {
			response.header.Set("ETag", etag)
			http.ServeContent(response, req, "", time.Time{}, bytes.NewReader(page))
		} else {
			_, _ = response.Write(page)
		}
	} else if strings.HasPrefix(u.Path, "/res/") {
		// Article media is already an in-process WebView request. Dispatch it
		// to its resource operation directly; do not route through the HTTP
		// handler or its middleware chain.
		s.callDesktopOperation(response, req, func() { s.desktopDictionaryResource(response, req) })
	} else if strings.HasPrefix(u.Path, "/assets/") || u.Path == "/favicon.ico" {
		if asset, ok := desktopAsset(u.Path); ok {
			response.header.Set("Content-Type", asset.contentType)
			response.header.Set("Cache-Control", asset.cacheControl)
			_, _ = response.Write(asset.body)
		} else {
			http.NotFound(response, req)
		}
	} else if strings.HasPrefix(u.Path, "/style/") {
		s.callDesktopOperation(response, req, func() {
			s.serveUserCSS(response, req, strings.TrimPrefix(u.Path, "/style/"))
		})
	} else if strings.HasPrefix(u.Path, "/files/") {
		name := strings.TrimPrefix(u.Path, userFileURL)
		file, info := s.openUserFile(name)
		if file == nil {
			http.NotFound(response, req)
		} else {
			defer file.Close()
			response.header.Set("Content-Type", resolveMIME(name, "application/octet-stream"))
			response.header.Set("X-Content-Type-Options", "nosniff")
			response.header.Set("Cache-Control", "no-cache")
			http.ServeContent(response, req, name, info.ModTime(), file)
		}
	} else {
		// The secure WebView origin has no HTTP API fallback. Its operations
		// must be explicitly wired to the native bridge above.
		http.NotFound(response, req)
	}
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

const desktopStorageMigrationPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Starting wuDict2</title><style>
*{box-sizing:border-box}html,body{width:100%;height:100%;margin:0}body{display:grid;place-items:center;background:#f4ecd9;color:#332c20;font:16px "Segoe UI",sans-serif}
main{width:min(440px,calc(100% - 40px));padding:30px;border:1px solid #d3c5a7;border-radius:12px;background:#fffaf0;box-shadow:0 8px 28px #332c201a}
h1{margin:0 0 10px;font-size:22px}p{min-height:48px;margin:0 0 22px;color:#615746;line-height:1.5}
.track{height:12px;overflow:hidden;border-radius:8px;background:#e5dac2}.bar{width:38%;height:100%;border-radius:8px;background:#d99000;animation:move 1.4s ease-in-out infinite alternate}
@keyframes move{from{transform:translateX(-5%)}to{transform:translateX(165%)}}@media(prefers-reduced-motion:reduce){.bar{animation:none;width:100%}}
</style></head><body><main><h1>Starting wuDict2</h1><p id="status" role="status" aria-live="polite">Checking your saved settings…</p><div class="track" role="progressbar" aria-label="Transferring settings"><div class="bar"></div></div></main>
<script>window.wudictMigrationStatus=function(message){var node=document.getElementById("status");if(node)node.textContent=message}</script></body></html>`

func (s *Server) desktopDictionaryResource(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/res/")
	id, name, ok := strings.Cut(rest, "/")
	if !ok || name == "" {
		httpErr(w, http.StatusBadRequest, "usage: /res/{dict}/{name}")
		return
	}
	e, err := s.reg.get(id)
	if err != nil {
		httpErr(w, http.StatusNotFound, "%v", err)
		return
	}
	s.serveDictionaryResource(w, r, e, id, name)
}

func (s *Server) callDesktopOperation(w http.ResponseWriter, r *http.Request, operation func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logx.Warn("panic serving in-process resource %s: %v", r.URL.Path, recovered)
			httpErr(w, http.StatusInternalServerError, "internal error: %v", fmt.Sprint(recovered))
		}
	}()
	operation()
}

func (s *Server) desktopPage(path string, query url.Values, acceptLanguage string) ([]byte, string, string, bool) {
	switch path {
	case "/":
		page, etag := s.indexPageFor(query.Get("style") == "off", acceptLanguage)
		return page, etag, "no-cache", true
	case "/setup":
		return s.setupPageFor(acceptLanguage), "", "no-cache", true
	case "/lemmas":
		return s.lemmasPageFor(acceptLanguage), "", "no-cache", true
	case "/groups":
		return s.groupsPageHTMLFor(acceptLanguage), "", "no-cache", true
	case "/browse":
		return s.browsePageHTMLFor(acceptLanguage), "", "no-cache", true
	case "/double-tap-probe":
		return doubleTapProbe, "", "no-store", true
	default:
		return nil, "", "", false
	}
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
