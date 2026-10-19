// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http"
	"path"
	"strings"
)

func (s *Server) handleArticleFont(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.URL.Path)
	if !strings.HasSuffix(name, ".woff2") && name != "Quivira.otf" {
		http.NotFound(w, r)
		return
	}
	data, err := articleFontFiles.ReadFile("web/fonts/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := "font/woff2"
	if name == "Quivira.otf" {
		contentType = "font/otf"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	_, _ = w.Write(data)
}
