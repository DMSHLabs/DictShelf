// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"errors"
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
	Stage                  string `json:"stage"`
	Resource               string `json:"resource"`
	Dict                   string `json:"dict"`
	Page                   int    `json:"page"`
	StartedAt              int64  `json:"startedAt"`
	ElapsedMS              int    `json:"elapsedMs"`
	DurationMS             int    `json:"durationMs"`
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
	if err := recordBrowseDiagnostic(d); err != nil {
		http.Error(w, "invalid diagnostic", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func recordBrowseDiagnostic(d browseDiagnostic) error {
	switch d.Event {
	case "alphabet_cache_miss", "alphabet_cache_invalid", "alphabet_cache_read_failed", "alphabet_cache_restore", "alphabet_cache_saved", "alphabet_cache_save_failed", "alphabet_progress", "alphabet_done", "alphabet_aborted", "alphabet_error", "page_request", "page_response", "page_ready", "page_fetch_failed", "render", "page_cache_saved", "page_cache_restored", "wildcard_cache_checkpoint", "wildcard_cache_complete", "wildcard_cache_restored", "wildcard_memory_start", "wildcard_memory_sample", "wildcard_memory_end", "dictionary_picker_open", "dictionary_list_client", "article_transition", "browse_navigation", "navigation_resource", "storage_bootstrap":
	default:
		return errors.New("invalid diagnostic")
	}
	if len(d.Dict) > 256 || strings.ContainsAny(d.Dict, "\r\n") || len(d.Resource) > 256 || strings.ContainsAny(d.Resource, "\r\n") ||
		!bounded(d.Page, 1_000_000) || !bounded(d.Words, 1_000_000) || !bounded(d.PageBytes, 1_000_000_000) || !bounded(d.PageJSONBytes, 100_000_000) ||
		!bounded(d.CachePages, 1_000_000) || !bounded(d.CacheBytes, 1_000_000_000_000) || !bounded(d.CacheJSONBytes, 100_000_000) ||
		!bounded(d.CacheStoredPages, 1_000_000) || !bounded(d.CacheStoredBytes, 100_000_000) ||
		!bounded(d.TotalPages, 1_000_000) || !bounded(d.EstimatedAllPagesBytes, 1_000_000_000_000) || !bounded(d.Letters, 1_000_000) || !bounded(d.Bytes, 10_000_000) ||
		!bounded(d.ReadMS, 600_000) || !bounded(d.FetchMS, 600_000) || !bounded(d.TotalMS, 600_000) || !bounded(d.WordsMS, 600_000) ||
		!bounded(d.StripBuildMS, 600_000) || !bounded(d.StripMarkMS, 600_000) || !bounded(d.Status, 599) ||
		!bounded(d.ElapsedMS, 86_400_000) || !bounded(d.DurationMS, 86_400_000) || d.StartedAt < 0 || d.StartedAt > 10_000_000_000_000 ||
		!bounded(d.Active, 100_000) || !bounded(d.Completed, 100_000) || !bounded(d.Dictionaries, 100_000) {
		return errors.New("invalid diagnostic")
	}
	if d.Event == "article_transition" {
		switch d.Stage {
		case "document_response_start", "document_response_end", "dom_interactive", "app_script_ready", "startup_dependencies_ready", "first_dictionary_row", "dictionary_list_ready", "search_start", "search_stream_begin", "first_search_response", "first_article_painted", "search_end", "words_click", "browse_return_script_ready", "browse_return_page_ready", "browse_return_ready", "browse_return_restore_position", "browse_return_bfcache":
		default:
			return errors.New("invalid diagnostic")
		}
		logx.System("browse_article_transition nav_start_ms=%d stage=%s dict=%q browse_page=%d elapsed_ms=%d duration_ms=%d",
			d.StartedAt, d.Stage, d.Dict, d.Page, d.ElapsedMS, d.DurationMS)
		return nil
	}
	if d.Event == "browse_navigation" {
		switch d.Stage {
		case "document_response_start", "document_response_end", "dom_interactive", "script_ready", "chooser_request", "chooser_response", "chooser_ready", "page_request", "page_response", "page_cache_restored", "first_words_painted", "page_ready", "alphabet_ready", "load_failed":
		default:
			return errors.New("invalid diagnostic")
		}
		logx.System("browse_navigation nav_start_ms=%d stage=%s dict=%q browse_page=%d elapsed_ms=%d duration_ms=%d words=%d status=%d",
			d.StartedAt, d.Stage, d.Dict, d.Page, d.ElapsedMS, d.DurationMS, d.Words, d.Status)
		return nil
	}
	if d.Event == "navigation_resource" {
		if d.Resource == "" || !strings.HasPrefix(d.Resource, "/assets/") && !strings.HasPrefix(d.Resource, "/style/") {
			return errors.New("invalid diagnostic")
		}
		logx.System("navigation_resource nav_start_ms=%d resource=%q start_ms=%d duration_ms=%d response_end_ms=%d elapsed_ms=%d",
			d.StartedAt, d.Resource, d.FetchMS, d.DurationMS, d.TotalMS, d.ElapsedMS)
		return nil
	}
	if d.Event == "storage_bootstrap" {
		logx.System("storage_bootstrap nav_start_ms=%d elapsed_ms=%d first_document_script_ms=%d document_script_start_ms=%d bootstrap_start_ms=%d i18n_markup_ms=%d duration_ms=%d restored=%t",
			d.StartedAt, d.ElapsedMS, d.StripBuildMS, d.ReadMS, d.FetchMS, d.WordsMS, d.DurationMS, d.Restored)
		return nil
	}
	if strings.HasPrefix(d.Event, "wildcard_memory_") {
		if d.Event == "wildcard_memory_start" {
			startBrowseMemoryMonitor(d.Dictionaries)
		} else if d.Event == "wildcard_memory_end" {
			stopBrowseMemoryMonitor()
		}
		logBrowseMemory(d.Event, d.Active, d.Completed, d.Dictionaries)
		return nil
	}
	if d.Event == "dictionary_picker_open" {
		logx.System("dictionary_picker_diag group_fetch_ms=%d rows_render_ms=%d rows=%d total_ms=%d",
			d.FetchMS, d.WordsMS, d.Words, d.TotalMS)
		return nil
	}
	if d.Event == "dictionary_list_client" {
		logx.System("dictionary_list_client dictionaries=%d begin_ms=%d first_row_ms=%d stream_ms=%d render_ms=%d",
			d.Words, d.FetchMS, d.WordsMS, d.TotalMS, d.StripBuildMS)
		return nil
	}
	logx.System("browse_diag event=%s dict=%q page=%d words=%d page_heap_est_bytes=%d page_json_bytes=%d cache_pages=%d cache_heap_est_bytes=%d cache_json_bytes=%d cache_stored_pages=%d cache_stored_bytes=%d total_pages=%d all_pages_heap_est_bytes=%d letters=%d bytes=%d read_ms=%d fetch_ms=%d total_ms=%d words_ms=%d strip_build_ms=%d strip_mark_ms=%d status=%d restored=%t stream_alphabet=%t",
		d.Event, d.Dict, d.Page, d.Words, d.PageBytes, d.PageJSONBytes, d.CachePages, d.CacheBytes, d.CacheJSONBytes, d.CacheStoredPages, d.CacheStoredBytes, d.TotalPages, d.EstimatedAllPagesBytes, d.Letters, d.Bytes, d.ReadMS, d.FetchMS, d.TotalMS, d.WordsMS, d.StripBuildMS, d.StripMarkMS, d.Status, d.Restored, d.StreamAlphabet)
	return nil
}

func bounded(n, max int) bool { return n >= 0 && n <= max }
