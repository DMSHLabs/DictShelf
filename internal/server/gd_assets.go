// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"embed"
	"net/http"
	"strings"
)

// Only the unmodified Quivira font is distributed with the application.
// Unique family names avoid changing the UI or ordinary dictionary articles.
//
//go:embed web/fonts/Quivira.otf
var gdFonts embed.FS

func (s *Server) handleGDFont(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/assets/gd/fonts/")
	body, ok := gdFontAsset(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/otf")
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	_, _ = w.Write(body)
}

func gdFontAsset(name string) ([]byte, bool) {
	if name != "Quivira.otf" {
		return nil, false
	}
	body, err := gdFonts.ReadFile("web/fonts/" + name)
	if err != nil {
		return nil, false
	}
	return body, true
}
