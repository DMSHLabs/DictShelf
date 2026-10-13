// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"runtime/metrics"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wuweidict/wudict/internal/logx"
)

var browseFindStreamsActive atomic.Int64
var browseMemoryMonitor struct {
	sync.Mutex
	stop chan struct{}
}

func startBrowseMemoryMonitor(dictionaries int) {
	browseMemoryMonitor.Lock()
	if browseMemoryMonitor.stop != nil {
		close(browseMemoryMonitor.stop)
	}
	stop := make(chan struct{})
	browseMemoryMonitor.stop = stop
	browseMemoryMonitor.Unlock()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		deadline := time.NewTimer(10 * time.Minute)
		defer deadline.Stop()
		for {
			select {
			case <-stop:
				return
			case <-deadline.C:
				browseMemoryMonitor.Lock()
				if browseMemoryMonitor.stop == stop {
					close(stop)
					browseMemoryMonitor.stop = nil
				}
				browseMemoryMonitor.Unlock()
				return
			case <-ticker.C:
				logBrowseMemory("wildcard_memory_sample", int(browseFindStreamsActive.Load()), 0, dictionaries)
			}
		}
	}()
}

func stopBrowseMemoryMonitor() {
	browseMemoryMonitor.Lock()
	if browseMemoryMonitor.stop != nil {
		close(browseMemoryMonitor.stop)
		browseMemoryMonitor.stop = nil
	}
	browseMemoryMonitor.Unlock()
}

func logBrowseMemory(event string, active, completed, dictionaries int) {
	mem := []metrics.Sample{
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
		{Name: "/gc/cycles/total:gc-cycles"},
	}
	metrics.Read(mem)
	process := browseProcessMemory()
	logx.System("browse_memory event=%s active=%d completed=%d dictionaries=%d go_heap_objects_bytes=%d go_total_bytes=%d go_released_bytes=%d go_gc=%d server_working_bytes=%d server_private_bytes=%d app_webview_processes=%d app_webview_working_bytes=%d app_webview_private_bytes=%d all_webview_processes=%d all_webview_working_bytes=%d all_webview_private_bytes=%d process_sample_error=%q",
		event, active, completed, dictionaries, mem[0].Value.Uint64(), mem[1].Value.Uint64(), mem[2].Value.Uint64(), mem[3].Value.Uint64(),
		process.serverWorking, process.serverPrivate, process.appWebviewCount, process.appWebviewWorking, process.appWebviewPrivate,
		process.webviewCount, process.webviewWorking, process.webviewPrivate, process.err)
}

type browseDiagnostic struct {
	Event                  string `json:"event"`
	Dict                   string `json:"dict"`
	Page                   int    `json:"page"`
	Words                  int    `json:"words"`
	PageBytes              int    `json:"pageBytes"`
	PageJSONBytes          int    `json:"pageJsonBytes"`
	CachePages             int    `json:"cachePages"`
	CacheBytes             int    `json:"cacheBytes"`
	CacheJSONBytes         int    `json:"cacheJsonBytes"`
	CacheStoredPages       int    `json:"cacheStoredPages"`
	CacheStoredBytes       int    `json:"cacheStoredBytes"`
	TotalPages             int    `json:"totalPages"`
	EstimatedAllPagesBytes int    `json:"estimatedAllPagesBytes"`
	Letters                int    `json:"letters"`
	Bytes                  int    `json:"bytes"`
	ReadMS                 int    `json:"readMs"`
	FetchMS                int    `json:"fetchMs"`
	TotalMS                int    `json:"totalMs"`
	WordsMS                int    `json:"wordsMs"`
	StripBuildMS           int    `json:"stripBuildMs"`
	StripMarkMS            int    `json:"stripMarkMs"`
	Status                 int    `json:"status"`
	Restored               bool   `json:"restored"`
	StreamAlphabet         bool   `json:"streamAlphabet"`
	Active                 int    `json:"active"`
	Completed              int    `json:"completed"`
	Dictionaries           int    `json:"dictionaries"`
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
	case "alphabet_cache_miss", "alphabet_cache_invalid", "alphabet_cache_read_failed", "alphabet_cache_restore", "alphabet_cache_saved", "alphabet_cache_save_failed", "page_request", "page_response", "page_ready", "render", "page_cache_saved", "page_cache_restored", "wildcard_cache_checkpoint", "wildcard_cache_complete", "wildcard_cache_restored", "wildcard_memory_start", "wildcard_memory_sample", "wildcard_memory_end":
	default:
		http.Error(w, "invalid diagnostic", http.StatusBadRequest)
		return
	}
	if len(d.Dict) > 256 || strings.ContainsAny(d.Dict, "\r\n") ||
		!bounded(d.Page, 1_000_000) || !bounded(d.Words, 1_000_000) || !bounded(d.PageBytes, 1_000_000_000) || !bounded(d.PageJSONBytes, 100_000_000) ||
		!bounded(d.CachePages, 1_000_000) || !bounded(d.CacheBytes, 1_000_000_000_000) || !bounded(d.CacheJSONBytes, 100_000_000) ||
		!bounded(d.CacheStoredPages, 1_000_000) || !bounded(d.CacheStoredBytes, 100_000_000) ||
		!bounded(d.TotalPages, 1_000_000) || !bounded(d.EstimatedAllPagesBytes, 1_000_000_000_000) || !bounded(d.Letters, 1_000_000) || !bounded(d.Bytes, 10_000_000) ||
		!bounded(d.ReadMS, 600_000) || !bounded(d.FetchMS, 600_000) || !bounded(d.TotalMS, 600_000) || !bounded(d.WordsMS, 600_000) ||
		!bounded(d.StripBuildMS, 600_000) || !bounded(d.StripMarkMS, 600_000) || !bounded(d.Status, 599) ||
		!bounded(d.Active, 100_000) || !bounded(d.Completed, 100_000) || !bounded(d.Dictionaries, 100_000) {
		http.Error(w, "invalid diagnostic", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(d.Event, "wildcard_memory_") {
		if d.Event == "wildcard_memory_start" {
			startBrowseMemoryMonitor(d.Dictionaries)
		} else if d.Event == "wildcard_memory_end" {
			stopBrowseMemoryMonitor()
		}
		logBrowseMemory(d.Event, d.Active, d.Completed, d.Dictionaries)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	logx.System("browse_diag event=%s dict=%q page=%d words=%d page_heap_est_bytes=%d page_json_bytes=%d cache_pages=%d cache_heap_est_bytes=%d cache_json_bytes=%d cache_stored_pages=%d cache_stored_bytes=%d total_pages=%d all_pages_heap_est_bytes=%d letters=%d bytes=%d read_ms=%d fetch_ms=%d total_ms=%d words_ms=%d strip_build_ms=%d strip_mark_ms=%d status=%d restored=%t stream_alphabet=%t",
		d.Event, d.Dict, d.Page, d.Words, d.PageBytes, d.PageJSONBytes, d.CachePages, d.CacheBytes, d.CacheJSONBytes, d.CacheStoredPages, d.CacheStoredBytes, d.TotalPages, d.EstimatedAllPagesBytes, d.Letters, d.Bytes, d.ReadMS, d.FetchMS, d.TotalMS, d.WordsMS, d.StripBuildMS, d.StripMarkMS, d.Status, d.Restored, d.StreamAlphabet)
	w.WriteHeader(http.StatusNoContent)
}

func bounded(n, max int) bool { return n >= 0 && n <= max }
