// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"embed"
	"net/http"
	"strings"
)

// The original fonts are provided with the user's GoldenDict stylesheet.
// Unique family names avoid changing the UI or ordinary dictionary articles.
//
//go:embed web/fonts/*.ttf
var gdFonts embed.FS

func (s *Server) handleGDFont(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/assets/gd/fonts/")
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") || !strings.HasSuffix(name, ".ttf") {
		http.NotFound(w, r)
		return
	}
	body, err := gdFonts.ReadFile("web/fonts/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/ttf")
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	_, _ = w.Write(body)
}
