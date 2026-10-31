// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/search"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

var browseFindStreamsActive atomic.Int64
var errIndexingModeBusy = errors.New("cannot change indexing mode while indexing is active")
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
	logx.SystemDiagnostic("browse_memory event=%s active=%d completed=%d dictionaries=%d go_heap_objects_bytes=%d go_total_bytes=%d go_released_bytes=%d go_gc=%d server_working_bytes=%d server_private_bytes=%d app_webview_processes=%d app_webview_working_bytes=%d app_webview_private_bytes=%d all_webview_processes=%d all_webview_working_bytes=%d all_webview_private_bytes=%d process_sample_error=%q",
		event, active, completed, dictionaries, mem[0].Value.Uint64(), mem[1].Value.Uint64(), mem[2].Value.Uint64(), mem[3].Value.Uint64(),
		process.serverWorking, process.serverPrivate, process.appWebviewCount, process.appWebviewWorking, process.appWebviewPrivate,
		process.webviewCount, process.webviewWorking, process.webviewPrivate, process.err)
}

type browseDiagnostic struct {
	SwitchID               int    `json:"switchId"`
	RunID                  int    `json:"runId"`
	Query                  string `json:"query"`
	Mode                   string `json:"mode"`
	Name                   string `json:"name"`
	ResultCount            int    `json:"resultCount"`
	Group                  string `json:"group"`
	FTS                    bool   `json:"fts"`
	Trigram                bool   `json:"trigram"`
	Detail                 string `json:"detail"`
	SpinnerVisible         bool   `json:"spinnerVisible"`
	NoticeVisible          bool   `json:"noticeVisible"`
	BusyWanted             bool   `json:"busyWanted"`
	MergedView             bool   `json:"mergedView"`
	NoticeText             string `json:"noticeText"`
	Event                  string `json:"event"`
	Stage                  string `json:"stage"`
	URL                    string `json:"url"`
	Referrer               string `json:"referrer"`
	ReturnParam            string `json:"returnParam"`
	OriginURL              string `json:"originURL"`
	TargetURL              string `json:"targetURL"`
	HasOrigin              bool   `json:"hasOrigin"`
	HasFrom                bool   `json:"hasFrom"`
	Chooser                bool   `json:"chooser"`
	Slots                  int    `json:"slots"`
	Error                  string `json:"error"`
	Skipped                bool   `json:"skipped"`
	AnyResults             bool   `json:"anyResults"`
	Resource               string `json:"resource"`
	Dict                   string `json:"dict"`
	Page                   int    `json:"page"`
	Total                  int    `json:"total"`
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
	Measured               int    `json:"measured"`
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
	Enabled                bool   `json:"enabled"`
	Active                 int    `json:"active"`
	Completed              int    `json:"completed"`
	Dictionaries           int    `json:"dictionaries"`
	ProgressEvents         int    `json:"progressEvents"`
	ProgressBytes          int    `json:"progressBytes"`
	ProgressRenders        int    `json:"progressRenders"`
	ProgressRenderMS       int    `json:"progressRenderMs"`
	ProgressRenderMaxMS    int    `json:"progressRenderMaxMs"`
	ProgressTaskUpdates    int    `json:"progressTaskUpdates"`
	ProgressTaskUpdateMS   int    `json:"progressTaskUpdateMs"`
	ProgressTaskMaxMS      int    `json:"progressTaskMaxMs"`
	ProgressPublished      int    `json:"progressPublished"`
	ProgressScanned        int    `json:"progressScanned"`
	ProgressScanTotal      int    `json:"progressScanTotal"`
	ViewportWidth          int    `json:"viewportWidth"`
	ViewportHeight         int    `json:"viewportHeight"`
	ResizeEvents           int    `json:"resizeEvents"`
	HandlerDelayMaxMS      int    `json:"handlerDelayMaxMs"`
	FrameDelayMaxMS        int    `json:"frameDelayMaxMs"`
	PaintDelayMaxMS        int    `json:"paintDelayMaxMs"`
	ActiveTasks            int    `json:"activeTasks"`
	RibbonItems            int    `json:"ribbonItems"`
	RibbonSections         int    `json:"ribbonSections"`
	RibbonVisible          int    `json:"ribbonVisible"`
	RibbonAnchor           int    `json:"ribbonAnchor"`
	RibbonAdapterMS        int    `json:"ribbonAdapterMs"`
	RibbonBuildMS          int    `json:"ribbonBuildMs"`
	RibbonPaintMS          int    `json:"ribbonPaintMs"`
	ResizeStartWidth       int    `json:"resizeStartWidth"`
	ResizeBurstMS          int    `json:"resizeBurstMs"`
	RibbonMetricsChanged   bool   `json:"ribbonMetricsChanged"`
	RibbonPaintDeferred    bool   `json:"ribbonPaintDeferred"`
}

func (s *Server) handleBrowseDiagnostic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
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
	if err := s.recordBrowseDiagnostic(d); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errIndexingModeBusy) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func recordBrowseDiagnostic(d browseDiagnostic) error {
	if d.Event == "indexing_mode" {
		return setIndexingMode(d.Mode, nil)
	}
	if d.Event == "index_parallelism_diagnostics" {
		store.SetIngestParallelismDiagnostics(d.Enabled)
		return nil
	}
	if d.Event == "search_parallelism_diagnostics" {
		search.SetDiagnostics(d.Enabled)
		setWildcardParallelismDiagnostics(d.Enabled)
		logx.SystemDiagnostic("search_parallelism enabled=%t workers=%d", d.Enabled, search.Workers())
		return nil
	}
	if d.Event == "browse_return_route" {
		switch d.Stage {
		case "browse_page_loaded", "close_clicked", "main_page_loaded", "apply_url_has_query", "apply_url_no_query",
			"first_dictionary_apply_url", "dictionary_list_ready_apply_url", "search_skipped_empty_query", "search_started",
			"search_stream_begin", "first_search_hit", "search_stream_end":
		default:
			return errors.New("invalid diagnostic")
		}
		if d.StartedAt < 0 || d.StartedAt > 10_000_000_000_000 || !bounded(d.ElapsedMS, 86_400_000) ||
			len(d.URL) > 4096 || strings.ContainsAny(d.URL, "\r\n") ||
			len(d.Referrer) > 4096 || strings.ContainsAny(d.Referrer, "\r\n") ||
			len(d.ReturnParam) > 4096 || strings.ContainsAny(d.ReturnParam, "\r\n") ||
			len(d.OriginURL) > 4096 || strings.ContainsAny(d.OriginURL, "\r\n") ||
			len(d.TargetURL) > 4096 || strings.ContainsAny(d.TargetURL, "\r\n") ||
			len(d.Query) > 1024 || strings.ContainsAny(d.Query, "\r\n") ||
			len(d.Mode) > 32 || strings.ContainsAny(d.Mode, "\r\n") ||
			len(d.Dict) > 256 || strings.ContainsAny(d.Dict, "\r\n") ||
			len(d.Error) > 1024 || strings.ContainsAny(d.Error, "\r\n") ||
			!bounded(d.Total, 100_000) || !bounded(d.Slots, 100_000) || !bounded(d.ResultCount, 1_000_000) {
			return errors.New("invalid diagnostic")
		}
		logx.SystemDiagnostic("browse_return_route stage=%s elapsed_ms=%d url=%q referrer=%q return_param=%q origin_url=%q target_url=%q query=%q mode=%q dict=%q has_origin=%t has_from=%t chooser=%t slots=%d result_count=%d any_results=%t error=%q skipped=%t",
			d.Stage, d.ElapsedMS, d.URL, d.Referrer, d.ReturnParam, d.OriginURL, d.TargetURL,
			d.Query, d.Mode, d.Dict, d.HasOrigin, d.HasFrom, d.Chooser,
			max(d.Slots, d.Total), d.ResultCount, d.AnyResults, d.Error, d.Skipped)
		return nil
	}
	if d.Event == "search_article" {
		switch d.Stage {
		case "search_start", "search_empty_group", "search_stream_begin", "dictionary_response", "article_open", "return_target_open", "return_target_after_paint", "return_target_search_end", "return_target_after_search_paint", "first_article_painted", "search_end", "search_aborted", "search_error":
		default:
			return errors.New("invalid diagnostic")
		}
		if !bounded(d.RunID, 1_000_000_000) || !bounded(d.ElapsedMS, 86_400_000) || !bounded(d.Page, 100_000) || !bounded(d.Completed, 100_000) || !bounded(d.Total, 100_000) || !bounded(d.ResultCount, 1_000_000) ||
			len(d.Query) > 1024 || strings.ContainsAny(d.Query, "\r\n") || len(d.Mode) > 32 || strings.ContainsAny(d.Mode, "\r\n") ||
			len(d.Group) > 256 || strings.ContainsAny(d.Group, "\r\n") || len(d.Dict) > 256 || strings.ContainsAny(d.Dict, "\r\n") ||
			len(d.Name) > 256 || strings.ContainsAny(d.Name, "\r\n") || len(d.Detail) > 1024 || strings.ContainsAny(d.Detail, "\r\n") {
			return errors.New("invalid diagnostic")
		}
		logx.SystemDiagnostic("search_article run=%d stage=%s query=%q mode=%q group=%q slot=%d dict=%q name=%q elapsed_ms=%d completed=%d total=%d results=%d detail=%q",
			d.RunID, d.Stage, d.Query, d.Mode, d.Group, d.Page, d.Dict, d.Name, d.ElapsedMS, d.Completed, d.Total, d.ResultCount, d.Detail)
		return nil
	}
	if d.Event == "wildcard_view" {
		if !bounded(d.SwitchID, 1_000_000_000) || !bounded(d.RunID, 1_000_000_000) || !bounded(d.ElapsedMS, 86_400_000) ||
			!bounded(d.Active, 100_000) || !bounded(d.Completed, 100_000) || !bounded(d.Dictionaries, 100_000) ||
			len(d.NoticeText) > 1024 || strings.ContainsAny(d.NoticeText, "\r\n") || len(d.Stage) > 64 || strings.ContainsAny(d.Stage, "\r\n") || len(d.Resource) > 256 || strings.ContainsAny(d.Resource, "\r\n") || len(d.Dict) > 256 || strings.ContainsAny(d.Dict, "\r\n") ||
			len(d.Query) > 1024 || strings.ContainsAny(d.Query, "\r\n") || len(d.Group) > 256 || strings.ContainsAny(d.Group, "\r\n") || len(d.Detail) > 1024 || strings.ContainsAny(d.Detail, "\r\n") {
			return errors.New("invalid diagnostic")
		}
		logx.SystemDiagnostic("wildcard_view switch=%d run=%d stage=%q source=%q dict=%q query=%q group=%q fts=%t trigram=%t detail=%q elapsed_ms=%d merged=%t spinner=%t notice=%t busy_wanted=%t tasks=%d ready=%d dictionaries=%d notice_text=%q", d.SwitchID, d.RunID, d.Stage, d.Resource, d.Dict, d.Query, d.Group, d.FTS, d.Trigram, d.Detail, d.ElapsedMS, d.MergedView, d.SpinnerVisible, d.NoticeVisible, d.BusyWanted, d.Active, d.Completed, d.Dictionaries, d.NoticeText)
		return nil
	}
	switch d.Event {
	case "alphabet_cache_miss", "alphabet_cache_invalid", "alphabet_cache_read_failed", "alphabet_cache_restore", "alphabet_cache_saved", "alphabet_cache_save_failed", "alphabet_progress", "alphabet_done", "alphabet_measure_done", "alphabet_aborted", "alphabet_error", "alphabet_task", "page_request", "page_response", "page_ready", "page_fetch_failed", "render", "page_cache_saved", "page_cache_restored", "wildcard_cache_checkpoint", "wildcard_cache_complete", "wildcard_cache_restored", "wildcard_memory_start", "wildcard_memory_sample", "wildcard_memory_end", "wildcard_progress", "wildcard_ribbon", "dictionary_picker_open", "dictionary_picker_stage", "dictionary_list_client", "article_transition", "browse_navigation", "navigation_resource", "storage_bootstrap", "work_resize", "ribbon_guard":
	default:
		return errors.New("invalid diagnostic")
	}
	if len(d.Stage) > 64 || strings.ContainsAny(d.Stage, "\r\n") || len(d.Dict) > 256 || strings.ContainsAny(d.Dict, "\r\n") ||
		len(d.Resource) > 256 || strings.ContainsAny(d.Resource, "\r\n") ||
		!bounded(d.Page, 1_000_000) || !bounded(d.Words, 1_000_000_000) || !bounded(d.PageBytes, 1_000_000_000) || !bounded(d.PageJSONBytes, 100_000_000) ||
		!bounded(d.CachePages, 1_000_000) || !bounded(d.CacheBytes, 1_000_000_000_000) || !bounded(d.CacheJSONBytes, 100_000_000) ||
		!bounded(d.CacheStoredPages, 1_000_000) || !bounded(d.CacheStoredBytes, 100_000_000) ||
		!bounded(d.TotalPages, 1_000_000) || !bounded(d.EstimatedAllPagesBytes, 1_000_000_000_000) || !bounded(d.Letters, 1_000_000) || !bounded(d.Measured, 1_000_000) || !bounded(d.Bytes, 1_000_000_000) ||
		!bounded(d.ReadMS, 600_000) || !bounded(d.FetchMS, 600_000) || !bounded(d.TotalMS, 600_000) || !bounded(d.WordsMS, 600_000) ||
		!bounded(d.StripBuildMS, 600_000) || !bounded(d.StripMarkMS, 600_000) || !bounded(d.Status, 599) ||
		!bounded(d.ElapsedMS, 86_400_000) || !bounded(d.DurationMS, 86_400_000) || d.StartedAt < 0 || d.StartedAt > 10_000_000_000_000 ||
		!bounded(d.Active, 100_000) || !bounded(d.Completed, 100_000) || !bounded(d.Dictionaries, 100_000) ||
		!bounded(d.ProgressEvents, 10_000_000) || !bounded(d.ProgressBytes, 10_000_000_000) ||
		!bounded(d.ProgressRenders, 10_000_000) || !bounded(d.ProgressRenderMS, 86_400_000) ||
		!bounded(d.ProgressRenderMaxMS, 86_400_000) || !bounded(d.ProgressTaskUpdates, 10_000_000) ||
		!bounded(d.ProgressTaskUpdateMS, 86_400_000) || !bounded(d.ProgressTaskMaxMS, 86_400_000) ||
		!bounded(d.ProgressPublished, 10_000_000) || !bounded(d.ProgressScanned, 1_000_000_000) ||
		!bounded(d.ProgressScanTotal, 1_000_000_000) || !bounded(d.ViewportWidth, 100_000) ||
		!bounded(d.ViewportHeight, 100_000) || !bounded(d.ResizeEvents, 1_000_000) ||
		!bounded(d.HandlerDelayMaxMS, 86_400_000) || !bounded(d.FrameDelayMaxMS, 86_400_000) ||
		!bounded(d.PaintDelayMaxMS, 86_400_000) || !bounded(d.ActiveTasks, 100_000) ||
		!bounded(d.RibbonItems, 1_000_000) || !bounded(d.RibbonSections, 1_000_000) ||
		!bounded(d.RibbonVisible, 100_000) || !bounded(d.RibbonAnchor, 1_000_000) ||
		!bounded(d.RibbonAdapterMS, 600_000) || !bounded(d.RibbonBuildMS, 600_000) ||
		!bounded(d.RibbonPaintMS, 600_000) || !bounded(d.ResizeStartWidth, 100_000) ||
		!bounded(d.ResizeBurstMS, 600_000) {
		return errors.New("invalid diagnostic")
	}
	if d.Event == "article_transition" {
		switch d.Stage {
		case "document_response_start", "document_response_end", "dom_interactive", "app_script_ready", "startup_dependencies_ready", "first_dictionary_row", "dictionary_list_ready", "search_start", "search_stream_begin", "first_search_response", "first_article_painted", "search_end", "words_click", "browse_return_script_ready", "browse_return_page_ready", "browse_return_ready", "browse_return_restore_position", "browse_return_bfcache":
		default:
			return errors.New("invalid diagnostic")
		}
		logx.SystemDiagnostic("browse_article_transition nav_start_ms=%d stage=%s dict=%q browse_page=%d elapsed_ms=%d duration_ms=%d",
			d.StartedAt, d.Stage, d.Dict, d.Page, d.ElapsedMS, d.DurationMS)
		return nil
	}
	if d.Event == "browse_navigation" {
		switch d.Stage {
		case "document_response_start", "document_response_end", "dom_interactive", "script_ready", "chooser_request", "chooser_response", "chooser_ready", "page_request", "page_response", "page_cache_restored", "first_words_painted", "page_ready", "alphabet_ready", "load_failed":
		default:
			return errors.New("invalid diagnostic")
		}
		logx.SystemDiagnostic("browse_navigation nav_start_ms=%d stage=%s dict=%q browse_page=%d elapsed_ms=%d duration_ms=%d words=%d status=%d",
			d.StartedAt, d.Stage, d.Dict, d.Page, d.ElapsedMS, d.DurationMS, d.Words, d.Status)
		return nil
	}
	if d.Event == "navigation_resource" {
		if d.Resource == "" || !strings.HasPrefix(d.Resource, "/assets/") && !strings.HasPrefix(d.Resource, "/style/") {
			return errors.New("invalid diagnostic")
		}
		logx.SystemDiagnostic("navigation_resource nav_start_ms=%d resource=%q start_ms=%d duration_ms=%d response_end_ms=%d elapsed_ms=%d",
			d.StartedAt, d.Resource, d.FetchMS, d.DurationMS, d.TotalMS, d.ElapsedMS)
		return nil
	}
	if d.Event == "storage_bootstrap" {
		logx.SystemDiagnostic("storage_bootstrap nav_start_ms=%d elapsed_ms=%d first_document_script_ms=%d document_script_start_ms=%d bootstrap_start_ms=%d i18n_markup_ms=%d duration_ms=%d restored=%t",
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
	if d.Event == "wildcard_progress" {
		logx.SystemDiagnostic("wildcard_progress dict=%q stage=%s elapsed_ms=%d stream_events=%d stream_bytes=%d published_updates=%d ui_renders=%d ui_render_ms=%d ui_render_max_ms=%d task_updates=%d task_update_ms=%d task_update_max_ms=%d scanned=%d scan_total=%d",
			d.Dict, d.Stage, d.ReadMS, d.ProgressEvents, d.ProgressBytes, d.ProgressPublished, d.ProgressRenders,
			d.ProgressRenderMS, d.ProgressRenderMaxMS, d.ProgressTaskUpdates, d.ProgressTaskUpdateMS, d.ProgressTaskMaxMS,
			d.ProgressScanned, d.ProgressScanTotal)
		return nil
	}
	if d.Event == "wildcard_ribbon" {
		switch d.Stage {
		case "build", "resize":
		default:
			return errors.New("invalid diagnostic")
		}
		logx.SystemDiagnostic("wildcard_ribbon stage=%s dict=%q items=%d sections=%d visible=%d anchor=%d adapter_ms=%d build_ms=%d paint_ms=%d viewport_width=%d viewport_height=%d resize_events=%d resize_start_width=%d resize_burst_ms=%d metrics_changed=%t paint_deferred=%t",
			d.Stage, d.Dict, d.RibbonItems, d.RibbonSections, d.RibbonVisible, d.RibbonAnchor,
			d.RibbonAdapterMS, d.RibbonBuildMS, d.RibbonPaintMS, d.ViewportWidth, d.ViewportHeight,
			d.ResizeEvents, d.ResizeStartWidth, d.ResizeBurstMS, d.RibbonMetricsChanged, d.RibbonPaintDeferred)
		return nil
	}
	if d.Event == "dictionary_picker_open" {
		logx.SystemDiagnostic("dictionary_picker_diag group_fetch_ms=%d rows_render_ms=%d rows=%d total_ms=%d",
			d.FetchMS, d.WordsMS, d.Words, d.TotalMS)
		return nil
	}
	if d.Event == "dictionary_picker_stage" {
		logx.SystemDiagnostic("dictionary_picker_stage stage=%s context=%q elapsed_ms=%d duration_ms=%d rows=%d", d.Stage, d.Resource, d.ElapsedMS, d.DurationMS, d.Words)
		return nil
	}
	if d.Event == "alphabet_task" {
		logx.SystemDiagnostic("browse_alphabet_task dict=%q stage=%s active_tasks=%d panel_hidden=%t", d.Dict, d.Stage, d.Words, d.Status == 1)
		return nil
	}
	if d.Event == "dictionary_list_client" {
		logx.SystemDiagnostic("dictionary_list_client dictionaries=%d begin_ms=%d first_row_ms=%d stream_ms=%d render_ms=%d",
			d.Words, d.FetchMS, d.WordsMS, d.TotalMS, d.StripBuildMS)
		return nil
	}
	if d.Event == "alphabet_measure_done" {
		logx.SystemDiagnostic("browse_alphabet_measure dict=%q letters=%d measured=%d chunks=%d elapsed_ms=%d", d.Dict, d.Letters, d.Measured, d.Words, d.StripBuildMS)
		return nil
	}
	if d.Event == "work_resize" {
		logx.SystemDiagnostic("work_resize width=%d height=%d events=%d burst_ms=%d handler_delay_max_ms=%d frame_delay_max_ms=%d paint_delay_max_ms=%d active_tasks=%d",
			d.ViewportWidth, d.ViewportHeight, d.ResizeEvents, d.DurationMS, d.HandlerDelayMaxMS, d.FrameDelayMaxMS, d.PaintDelayMaxMS, d.ActiveTasks)
		return nil
	}
	if d.Event == "ribbon_guard" {
		if d.Resource != "browse" && d.Resource != "wildcards" || d.Stage != "geometry" && d.Stage != "entry" {
			return errors.New("invalid diagnostic")
		}
		logx.SystemError("ribbon_guard mode=%q reason=%q items=%d offsets=%d widths=%d from=%d to=%d",
			d.Resource, d.Stage, d.Letters, d.Measured, d.Words, d.Page, d.TotalPages)
		return nil
	}
	logx.SystemDiagnostic("browse_diag event=%s dict=%q page=%d words=%d page_heap_est_bytes=%d page_json_bytes=%d cache_pages=%d cache_heap_est_bytes=%d cache_json_bytes=%d cache_stored_pages=%d cache_stored_bytes=%d total_pages=%d all_pages_heap_est_bytes=%d letters=%d bytes=%d read_ms=%d fetch_ms=%d total_ms=%d words_ms=%d strip_build_ms=%d strip_mark_ms=%d status=%d restored=%t stream_alphabet=%t",
		d.Event, d.Dict, d.Page, d.Words, d.PageBytes, d.PageJSONBytes, d.CachePages, d.CacheBytes, d.CacheJSONBytes, d.CacheStoredPages, d.CacheStoredBytes, d.TotalPages, d.EstimatedAllPagesBytes, d.Letters, d.Bytes, d.ReadMS, d.FetchMS, d.TotalMS, d.WordsMS, d.StripBuildMS, d.StripMarkMS, d.Status, d.Restored, d.StreamAlphabet)
	return nil
}

func validIndexingMode(mode string) bool {
	return mode == "parallel-dictionaries-single-worker" || mode == "adaptive"
}

func (s *Server) recordBrowseDiagnostic(d browseDiagnostic) error {
	if d.Event != "indexing_mode" {
		return recordBrowseDiagnostic(d)
	}
	var prefs *Prefs
	if s.User != "" && s.reg != nil {
		prefs = s.reg.prefs
	}
	return setIndexingMode(d.Mode, prefs)
}

func setIndexingMode(mode string, prefs *Prefs) error {
	if !validIndexingMode(mode) {
		return errors.New("invalid indexing mode")
	}
	indexModeMu.Lock()
	defer indexModeMu.Unlock()
	if indexTaskQueuesActive.Load() != 0 {
		return errIndexingModeBusy
	}
	previous := indexconst.ModeName()
	changed, busy := indexconst.SetMode(mode)
	if busy {
		return errIndexingModeBusy
	}
	if !changed {
		return errors.New("invalid indexing mode")
	}
	if prefs != nil {
		if err := prefs.setIndexingMode(mode); err != nil {
			indexconst.SetMode(previous)
			return err
		}
	}
	logx.System("indexing mode changed mode=%s dictionary_worker_limit=%d logical_cpus=%d gomaxprocs=%d",
		indexconst.ModeName(), currentIndexLimit().Limit(), currentIndexCPUCount(), runtime.GOMAXPROCS(0))
	return nil
}

func bounded(n int, max int64) bool { return n >= 0 && int64(n) <= max }
