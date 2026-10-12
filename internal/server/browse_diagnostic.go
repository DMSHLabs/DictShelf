// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/wuweidict/wudict/internal/logx"
)

type browseDiagnostic struct {
	Event          string `json:"event"`
	Page           int    `json:"page"`
	Words          int    `json:"words"`
	Letters        int    `json:"letters"`
	Bytes          int    `json:"bytes"`
	ReadMS         int    `json:"readMs"`
	FetchMS        int    `json:"fetchMs"`
	TotalMS        int    `json:"totalMs"`
	WordsMS        int    `json:"wordsMs"`
	StripBuildMS   int    `json:"stripBuildMs"`
	StripMarkMS    int    `json:"stripMarkMs"`
	Status         int    `json:"status"`
	Restored       bool   `json:"restored"`
	StreamAlphabet bool   `json:"streamAlphabet"`
}

func (s *Server) handleBrowseDiagnostic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var d browseDiagnostic
	if err := dec.Decode(&d); err != nil {
		status := http.StatusBadRequest
		if _, ok := err.(*http.MaxBytesError); ok {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "invalid diagnostic", status)
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid diagnostic", http.StatusBadRequest)
		return
	}
	switch d.Event {
	case "alphabet_cache_miss", "alphabet_cache_invalid", "alphabet_cache_read_failed", "alphabet_cache_restore", "alphabet_cache_saved", "alphabet_cache_save_failed", "page_request", "page_response", "page_ready", "render":
	default:
		http.Error(w, "invalid diagnostic", http.StatusBadRequest)
		return
	}
	if !bounded(d.Page, 1_000_000) || !bounded(d.Words, 1_000_000) || !bounded(d.Letters, 1_000_000) || !bounded(d.Bytes, 10_000_000) ||
		!bounded(d.ReadMS, 600_000) || !bounded(d.FetchMS, 600_000) || !bounded(d.TotalMS, 600_000) || !bounded(d.WordsMS, 600_000) ||
		!bounded(d.StripBuildMS, 600_000) || !bounded(d.StripMarkMS, 600_000) || !bounded(d.Status, 599) {
		http.Error(w, "invalid diagnostic", http.StatusBadRequest)
		return
	}
	logx.System("browse_diag event=%s page=%d words=%d letters=%d bytes=%d read_ms=%d fetch_ms=%d total_ms=%d words_ms=%d strip_build_ms=%d strip_mark_ms=%d status=%d restored=%t stream_alphabet=%t",
		d.Event, d.Page, d.Words, d.Letters, d.Bytes, d.ReadMS, d.FetchMS, d.TotalMS, d.WordsMS, d.StripBuildMS, d.StripMarkMS, d.Status, d.Restored, d.StreamAlphabet)
	w.WriteHeader(http.StatusNoContent)
}

func bounded(n, max int) bool { return n >= 0 && n <= max }
