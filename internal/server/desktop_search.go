// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/wuweidict/wudict/internal/logx"
)

// DesktopStream dispatches desktop streams. Search, dictionary listing, ingest
// and rescan index work call their shared Go operations directly; plain rescan
// still enters its route.
func (s *Server) DesktopStream(ctx context.Context, baseURL, method, query string, emit func([]byte)) error {
	if method == "search" {
		var request SearchRequest
		if err := json.Unmarshal([]byte(query), &request); err != nil {
			return fmt.Errorf("invalid native search request: %w", err)
		}
		return s.desktopSearchRequest(ctx, baseURL, request, emit)
	}
	if method == "dicts" {
		return s.desktopDictionaryList(ctx, emit)
	}
	if method == "rescanIndexes" {
		base, err := url.Parse(baseURL)
		if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
			return errors.New("invalid desktop server address")
		}
		return s.desktopRescanIndexes(ctx, []byte(query), emit)
	}
	if method == "ingest" {
		base, err := url.Parse(baseURL)
		if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
			return errors.New("invalid desktop server address")
		}
		return s.desktopIngest(ctx, baseURL, query, emit)
	}
	path := ""
	verb := http.MethodGet
	var body io.Reader
	switch method {
	case "rescan":
		path = "/api/rescan"
	case "browse":
		path = "/api/browse"
	case "browseFind":
		path = "/api/browse/find"
	default:
		return fmt.Errorf("unsupported desktop stream %q", method)
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return errors.New("invalid desktop server address")
	}
	base.Path, base.RawPath, base.RawQuery, base.Fragment = path, "", query, ""
	req, err := http.NewRequestWithContext(ctx, verb, base.String(), body)
	if err != nil {
		return err
	}
	req.Host = base.Host
	req.RemoteAddr = "127.0.0.1:0"
	if verb == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	w := &desktopStreamWriter{header: make(http.Header), emit: emit}
	s.ServeHTTP(w, req)
	if err := w.finish(); err != nil {
		if (method == "browse" || method == "browseFind") && w.status >= 400 {
			var failure struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(w.body.Bytes(), &failure)
			message, _ := json.Marshal(map[string]any{"type": "error", "status": w.status, "error": failure.Error})
			if emit != nil {
				emit(message)
			}
			return nil
		}
		return err
	}
	return nil
}

func (s *Server) desktopIngest(ctx context.Context, baseURL, query string, emit func([]byte)) error {
	values, err := url.ParseQuery(query)
	if err != nil {
		return fmt.Errorf("invalid ingest request: %w", err)
	}
	e, err := s.reg.get(values.Get("dict"))
	if err != nil {
		return err
	}
	key := ingestKey(e.ID)
	if status, ok := s.jobs.status(key); !ok || !status.Running || len(values) != 1 {
		release, admitted := s.admitWork(key)
		if !admitted {
			return errors.New("dictionary operations are running; wait for completion or stop them")
		}
		s.startIngest(e, values)
		release()
	}
	var sendErr error
	s.followIngestEvents(ctx, key, func(event string, data any) {
		if emit == nil {
			return
		}
		message, err := json.Marshal(struct {
			Event string `json:"event"`
			Data  any    `json:"data"`
		}{event, data})
		if err == nil {
			emit(message)
		}
	})
	return sendErr
}

// DesktopSearch accepts a legacy query string and calls the shared search
// operation directly. WebView2 uses the typed JSON request via DesktopStream.
func (s *Server) DesktopSearch(ctx context.Context, baseURL, query string, emit func([]byte)) error {
	parameters, err := url.ParseQuery(query)
	if err != nil {
		return err
	}
	return s.desktopSearchRequest(ctx, baseURL, searchRequestFromQuery(parameters), emit)
}

// Search enters the shared Go operation directly. Encoding events is the
// bridge's only output adaptation; no request, router or NDJSON parser runs.
func (s *Server) desktopSearchRequest(ctx context.Context, baseURL string, request SearchRequest, emit func([]byte)) (resultErr error) {
	defer func() {
		if p := recover(); p != nil {
			logx.Warn("panic in native search: %v", p)
			resultErr = fmt.Errorf("internal error: %v", p)
		}
	}()
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return errors.New("invalid desktop server address")
	}
	request.Origin = base.Scheme + "://" + base.Host
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	return s.Search(ctx, request, func(event SearchEvent) error {
		buffer.Reset()
		if err := encoder.Encode(event); err != nil {
			return err
		}
		if emit != nil {
			emit(bytes.Clone(bytes.TrimSpace(buffer.Bytes())))
		}
		return nil
	})
}

func (s *Server) desktopDictionaryList(ctx context.Context, emit func([]byte)) (resultErr error) {
	defer func() {
		if p := recover(); p != nil {
			logx.Warn("panic in native dictionary list: %v", p)
			resultErr = fmt.Errorf("internal error: %v", p)
		}
	}()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	_, err := s.listDictionaries(ctx, "", func(event dictMsg) error {
		buffer.Reset()
		if err := encoder.Encode(event); err != nil {
			return err
		}
		if emit != nil {
			emit(bytes.Clone(bytes.TrimSpace(buffer.Bytes())))
		}
		return nil
	})
	return err
}

// DesktopJSON dispatches explicitly supported JSON operations for the desktop
// bridge. It calls shared server methods directly; browser HTTP routes remain
// available and delegate to the same operations where they overlap.
func (s *Server) DesktopJSON(baseURL, method, argument string) (any, error) {
	switch method {
	case "saveGroups":
		const limit = 4*maxGroupsBytes + 4096
		if len(argument) > limit {
			return nil, fmt.Errorf("%s is larger than 256 KB", GroupsFileName)
		}
		var request struct {
			Text    string `json:"text"`
			Replace bool   `json:"replace"`
		}
		if err := json.Unmarshal([]byte(argument), &request); err != nil {
			return nil, fmt.Errorf("bad request: %w", err)
		}
		return s.saveGroups([]byte(request.Text), request.Replace)
	case "resetGroups":
		return s.resetGroups()
	case "style":
		return s.styleSnapshot(), nil
	case "presets":
		return s.presetPayload(), nil
	case "reveal":
		path, _, err := s.revealShownPath(argument)
		if err != nil {
			return nil, err
		}
		return map[string]any{"revealed": path}, nil
	case "savePreset":
		if len(argument) > 1<<12 {
			return nil, errors.New("preset request is too large")
		}
		var request struct {
			ID string `json:"id"`
			On bool `json:"on"`
		}
		if err := json.Unmarshal([]byte(argument), &request); err != nil {
			return nil, fmt.Errorf("bad preset request: %w", err)
		}
		return s.savePreset(request.ID, request.On)
	case "saveStyle":
		return s.desktopSaveStyle(argument), nil
	case "deleteGroup":
		var request deleteGroupRequest
		if len(argument) > 4096 || json.Unmarshal([]byte(argument), &request) != nil || request.Group == "" || request.Group == allDictionariesGroup {
			return nil, errors.New("Invalid group")
		}
		if err := s.deleteGroup(request); err != nil {
			return nil, err
		}
		return map[string]bool{"deleted": true}, nil
	case "createGroup":
		var request createGroupRequest
		if len(argument) > 1<<20 || json.Unmarshal([]byte(argument), &request) != nil {
			return nil, errors.New("Invalid group name")
		}
		return s.createGroup(request)
	case "renameGroup":
		var request renameGroupRequest
		if len(argument) > 4096 || json.Unmarshal([]byte(argument), &request) != nil {
			return nil, errors.New("Invalid group")
		}
		return s.renameGroup(request)
	case "saveGroupOrder":
		var request groupOrderRequest
		if len(argument) > 1<<20 || json.Unmarshal([]byte(argument), &request) != nil || request.Members == nil || request.Group == allDictionariesGroup {
			return nil, errors.New("Invalid group order")
		}
		if err := s.saveGroupOrder(request); err != nil {
			return nil, err
		}
		return map[string]bool{"saved": true}, nil
	case "demand":
		values, err := url.ParseQuery(argument)
		if err != nil {
			return nil, fmt.Errorf("invalid demand request: %w", err)
		}
		explicit, _ := queryFlag(values, "explicit")
		indexing, err := s.DemandDictionary(values.Get("dict"), explicit)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"indexing": indexing}, nil
	case "savePrefs":
		return s.SavePrefs(strings.NewReader(argument))
	case "setup":
		return s.DesktopSetup(argument)
	case "folderSetup":
		return s.DesktopFolderSetup(argument)
	case "indexDefaults":
		if len(argument) > 4096 {
			return nil, errors.New("invalid index defaults")
		}
		var next indexOptions
		if err := json.Unmarshal([]byte(argument), &next); err != nil {
			return nil, errors.New("invalid index defaults")
		}
		return s.saveIndexDefaults(next)
	case "groups":
		return s.groupsSnapshot(), nil
	case "userGroups":
		return s.userGroupsSnapshot()
	case "about":
		base, err := url.Parse(baseURL)
		if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
			return nil, errors.New("invalid desktop server address")
		}
		return s.aboutSnapshot(argument, base.Scheme+"://"+base.Host)
	case "library":
		return s.librarySnapshot()
	case "lemmas":
		return s.lemmaSnapshot(context.Background(), argument == "refresh"), nil
	case "installLemma":
		result, _, err := s.startLemmaInstall(context.Background(), argument)
		return result, err
	case "removeLemma":
		result, _, err := s.removeLemma(argument)
		return result, err
	case "orphans":
		return s.orphansSnapshot()
	case "resolveOrphans":
		if len(argument) > 1<<20 {
			return nil, errors.New("orphan response is too large")
		}
		var request struct {
			Delete []string `json:"delete"`
			Keep   []string `json:"keep"`
		}
		if err := json.Unmarshal([]byte(argument), &request); err != nil {
			return nil, fmt.Errorf("bad request: %w", err)
		}
		return s.resolveOrphanRequest(request.Delete, request.Keep)
	case "removeLibrary":
		values, err := url.ParseQuery(argument)
		if err != nil {
			return nil, fmt.Errorf("invalid library removal request: %w", err)
		}
		result, _, err := s.removeLibrary(values)
		return result, err
	case "reindex":
		return s.reindexStatus(), nil
	case "indexWork":
		return s.indexWorkStatus(argument), nil
	case "startIndexWork", "startSingleIndexWork":
		return s.startIndexWork([]byte(argument), method == "startSingleIndexWork", true)
	case "stopIndexWork":
		return s.stopIndexWork("all")
	case "stopIngest":
		return s.stopIngest(argument)
	case "startReindex":
		status, _, err := s.StartReindex()
		return status, err
	case "stopReindex":
		return s.StopReindex(), nil
	case "looks":
		return s.desktopLooks(argument)
	case "browsePage":
		values, err := url.ParseQuery(argument)
		if err != nil {
			return nil, fmt.Errorf("invalid browse request: %w", err)
		}
		base, err := url.Parse(baseURL)
		if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
			return nil, errors.New("invalid desktop server address")
		}
		base.Path, base.RawPath, base.RawQuery, base.Fragment = "/api/browse", "", values.Encode(), ""
		req, err := http.NewRequest(http.MethodGet, base.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Host = base.Host
		req.RemoteAddr = "127.0.0.1:0"
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		var response any
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			return nil, fmt.Errorf("invalid browse response: %w", err)
		}
		return map[string]any{"status": rec.Code, "body": response}, nil
	}
	path := ""
	query := ""
	verb := http.MethodGet
	var body io.Reader
	switch method {
	case "setGroupMember", "replaceGroupMembership", "addGroupMembers", "linkGroup":
		route := map[string]struct{ path, verb string }{
			"setGroupMember":         {"/api/user-groups/member", http.MethodPut},
			"replaceGroupMembership": {"/api/user-groups/membership", http.MethodPut},
			"addGroupMembers":        {"/api/user-groups/members", http.MethodPut},
			"linkGroup":              {"/api/user-groups/link", http.MethodPut},
		}[method]
		path, verb, body = route.path, route.verb, strings.NewReader(argument)
	default:
		return nil, fmt.Errorf("unsupported desktop request %q", method)
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil {
		return nil, errors.New("invalid desktop server address")
	}
	base.Path, base.RawPath, base.RawQuery, base.Fragment = path, "", query, ""
	req, err := http.NewRequest(verb, base.String(), body)
	if err != nil {
		return nil, err
	}
	req.Host = base.Host
	req.RemoteAddr = "127.0.0.1:0"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code >= 400 {
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &payload) == nil && payload.Error != "" {
			return nil, errors.New(payload.Error)
		}
		return nil, errors.New(strings.TrimSpace(rec.Body.String()))
	}
	var result any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	return result, nil
}

// desktopLooks keeps the saved-appearance UI off the loopback transport while
// reusing its HTTP adapters until their operation logic is split into shared
// methods. The route and method are restricted to the look endpoints.
func (s *Server) desktopLooks(argument string) (any, error) {
	if len(argument) > 6<<20 {
		return nil, errors.New("look request is too large")
	}
	var call struct {
		Path   string `json:"path"`
		Method string `json:"method"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal([]byte(argument), &call); err != nil {
		return nil, fmt.Errorf("invalid look request: %w", err)
	}
	allowed := map[string]map[string]bool{
		"/api/looks":       {http.MethodGet: true, http.MethodPost: true, http.MethodPut: true, http.MethodDelete: true},
		"/api/looks/apply": {http.MethodPost: true},
	}
	if !allowed[call.Path][call.Method] {
		return nil, errors.New("unsupported look request")
	}
	req := httptest.NewRequest(call.Method, call.Path, strings.NewReader(call.Body))
	req.RemoteAddr = "127.0.0.1:0"
	if call.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	s.ServeHTTP(response, req)
	if response.Code >= http.StatusBadRequest {
		return nil, errors.New(strings.TrimSpace(response.Body.String()))
	}
	var result any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("invalid look response: %w", err)
	}
	return result, nil
}

type desktopStyleReply struct {
	Status int            `json:"status"`
	Value  map[string]any `json:"value,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// desktopSaveStyle preserves the style API's status and error contract for
// the WebView while applying the same validation and file writes as HTTP.
func (s *Server) desktopSaveStyle(argument string) desktopStyleReply {
	const limit = 4*maxUserCSSBytes + (1 << 12)
	if len(argument) > limit*2 {
		return desktopStyleReply{Status: http.StatusRequestEntityTooLarge, Error: "the request is larger than the style limit"}
	}
	var request struct {
		Body    json.RawMessage `json:"body"`
		Replace bool            `json:"replace"`
	}
	if err := json.Unmarshal([]byte(argument), &request); err != nil {
		return desktopStyleReply{Status: http.StatusBadRequest, Error: "bad request: " + err.Error()}
	}
	if len(request.Body) > limit {
		return desktopStyleReply{Status: http.StatusRequestEntityTooLarge, Error: "the request is larger than the style limit"}
	}
	var body styleSaveRequest
	if err := json.Unmarshal(request.Body, &body); err != nil {
		return desktopStyleReply{Status: http.StatusBadRequest, Error: "bad request: " + err.Error()}
	}
	if err := s.saveStyle(body, request.Replace); err != nil {
		var operation styleOperationError
		if errors.As(err, &operation) {
			return desktopStyleReply{Status: operation.status, Error: operation.message}
		}
		return desktopStyleReply{Status: http.StatusInternalServerError, Error: "could not save style: " + err.Error()}
	}
	return desktopStyleReply{Status: http.StatusOK, Value: s.styleSnapshot()}
}

type desktopStreamWriter struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	line      []byte
	event     string
	eventData []string
	emit      func([]byte)
}

func (w *desktopStreamWriter) Header() http.Header { return w.header }

func (w *desktopStreamWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *desktopStreamWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	contentType := w.header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/x-ndjson") {
		w.writeNDJSON(p)
		return len(p), nil
	}
	if strings.HasPrefix(contentType, "text/event-stream") {
		w.writeSSE(p)
		return len(p), nil
	}
	return w.body.Write(p)
}

func (w *desktopStreamWriter) writeNDJSON(p []byte) {
	w.line = append(w.line, p...)
	for {
		i := bytes.IndexByte(w.line, '\n')
		if i < 0 {
			return
		}
		line := bytes.TrimSpace(w.line[:i])
		if len(line) > 0 && w.emit != nil {
			w.emit(bytes.Clone(line))
		}
		w.line = w.line[i+1:]
	}
}

func (w *desktopStreamWriter) writeSSE(p []byte) {
	w.line = append(w.line, p...)
	for {
		i := bytes.IndexByte(w.line, '\n')
		if i < 0 {
			return
		}
		line := bytes.TrimSuffix(w.line[:i], []byte{'\r'})
		w.line = w.line[i+1:]
		if len(line) == 0 {
			w.emitSSE()
			continue
		}
		if line[0] == ':' { // SSE comment/keepalive
			continue
		}
		field, value, _ := bytes.Cut(line, []byte{':'})
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch string(field) {
		case "event":
			w.event = string(value)
		case "data":
			w.eventData = append(w.eventData, string(value))
		}
	}
}

func (w *desktopStreamWriter) emitSSE() {
	if len(w.eventData) == 0 {
		w.event = ""
		return
	}
	data := []byte(strings.Join(w.eventData, "\n"))
	if !json.Valid(data) {
		data, _ = json.Marshal(string(data))
	}
	if w.event == "" {
		w.event = "message"
	}
	message, err := json.Marshal(struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}{Event: w.event, Data: data})
	if err == nil && w.emit != nil {
		w.emit(message)
	}
	w.event = ""
	w.eventData = nil
}

func (w *desktopStreamWriter) Flush() {}

func (w *desktopStreamWriter) finish() error {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	switch {
	case strings.HasPrefix(w.header.Get("Content-Type"), "application/x-ndjson"):
		if len(bytes.TrimSpace(w.line)) > 0 && w.emit != nil {
			w.emit(bytes.Clone(bytes.TrimSpace(w.line)))
		}
	case strings.HasPrefix(w.header.Get("Content-Type"), "text/event-stream"):
		if len(w.line) > 0 {
			w.writeSSE([]byte{'\n'})
		}
		w.emitSSE()
	}
	if w.status < 400 {
		return nil
	}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(w.body.Bytes(), &payload) == nil && payload.Error != "" {
		return errors.New(payload.Error)
	}
	return fmt.Errorf("request failed (HTTP %d)", w.status)
}
