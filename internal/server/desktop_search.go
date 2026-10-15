// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/base64"
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

func desktopOrigin(raw string) (*url.URL, bool) {
	base, err := url.Parse(raw)
	if err != nil || base.Host == "" || base.User != nil {
		return nil, false
	}
	if base.Scheme == "http" || base.Scheme == "https" && strings.EqualFold(base.Host, "wudict2.local") {
		return base, true
	}
	return nil, false
}

// DesktopStream dispatches desktop streams. Search, dictionary listing, ingest
// and rescan index work call their shared Go operations directly. An ordinary
// folder rescan likewise refreshes through the registry and then streams rows.
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
	if method == "rescan" {
		return s.desktopRescan(ctx, emit)
	}
	if method == "rescanIndexes" {
		if _, ok := desktopOrigin(baseURL); !ok {
			return errors.New("invalid desktop server address")
		}
		return s.desktopRescanIndexes(ctx, []byte(query), emit)
	}
	if method == "ingest" {
		if _, ok := desktopOrigin(baseURL); !ok {
			return errors.New("invalid desktop server address")
		}
		return s.desktopIngest(ctx, baseURL, query, emit)
	}
	if method == "browseFind" {
		return s.desktopBrowseFindStream(ctx, query, emit)
	}
	if method == "browse" {
		return s.desktopBrowseStream(ctx, query, emit)
	}
	path := ""
	verb := http.MethodGet
	var body io.Reader
	switch method {
	case "rescan":
		path = "/api/rescan"
	default:
		return fmt.Errorf("unsupported desktop stream %q", method)
	}
	base, ok := desktopOrigin(baseURL)
	if !ok {
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
		if method == "browse" && w.status >= 400 {
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
	base, ok := desktopOrigin(baseURL)
	if !ok {
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

// desktopRescan is the local WebView equivalent of GET /api/rescan: refresh
// sources and prepared indexes, then stream a fresh dictionary list. The
// browser route retains its network-origin guard; a native bridge call comes
// only from the user's local Windows application.
func (s *Server) desktopRescan(ctx context.Context, emit func([]byte)) error {
	s.startRescanIndexes(rescanIndexesRequest{})
	for {
		status, changed, ok := s.jobs.watch("rescan-indexes")
		if !ok || !status.Running {
			if len(status.Failed) > 0 {
				return fmt.Errorf("rescan: %v", status.Failed)
			}
			break
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.reg.Warm()
	s.dictRows.flush()
	return s.desktopDictionaryList(ctx, emit)
}

// DesktopJSON dispatches explicitly supported JSON operations for the desktop
// bridge. It calls shared server methods directly; browser HTTP routes remain
// available and delegate to the same operations where they overlap.
func (s *Server) DesktopJSON(baseURL, method, argument string) (any, error) {
	return s.DesktopJSONContext(context.Background(), baseURL, method, argument)
}

// DesktopJSONContext is the cancellable form used by the Windows bridge for
// JSON requests that may scan dictionary headwords.
func (s *Server) DesktopJSONContext(ctx context.Context, baseURL, method, argument string) (any, error) {
	switch method {
	case "browseDiagnostic":
		if len(argument) > 2048 {
			return nil, errors.New("invalid diagnostic")
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/browse-diagnostic", strings.NewReader(argument))
		s.handleBrowseDiagnostic(recorder, request)
		if recorder.Code != http.StatusNoContent {
			return nil, errors.New("invalid diagnostic")
		}
		return map[string]bool{"recorded": true}, nil
	case "browseFindPage":
		values, err := url.ParseQuery(argument)
		if err != nil {
			return map[string]any{"status": http.StatusBadRequest, "error": "invalid browse search request"}, nil
		}
		result, err := s.browseFindPage(ctx, values)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var failure *browseFindSetupError
		if errors.As(err, &failure) {
			payload := map[string]any{"status": failure.status, "error": failure.message}
			if failure.status == http.StatusConflict {
				payload["prepared"] = false
			}
			return payload, nil
		}
		return map[string]any{"status": http.StatusInternalServerError, "error": err.Error()}, nil
	case "intakeStatus":
		return s.desktopIntakeStatus(), nil
	case "intakeURL":
		if len(argument) > 4096 {
			return nil, errors.New("import link is too long")
		}
		return s.desktopIntakeURL(argument)
	case "intakePath":
		if len(argument) > 32768 {
			return nil, errors.New("archive path is too long")
		}
		return s.desktopIntakePath(argument)
	case "intakeFileStart":
		var request struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		}
		if len(argument) > 4096 || json.Unmarshal([]byte(argument), &request) != nil {
			return nil, errors.New("invalid archive file")
		}
		return s.desktopIntakeUploadStart(request.Name, request.Size)
	case "intakeFileChunk":
		return nil, s.desktopIntakeUploadChunk(argument)
	case "intakeFileFinish":
		if len(argument) > 64 {
			return nil, errors.New("invalid file transfer")
		}
		return s.desktopIntakeUploadFinish(argument)
	case "intakeFileCancel":
		if len(argument) > 64 {
			return nil, errors.New("invalid file transfer")
		}
		return map[string]bool{"canceled": true}, s.desktopIntakeUploadCancel(argument)
	case "intakeConfirm":
		if len(argument) > 1<<20 {
			return nil, errors.New("import selection is too large")
		}
		var request intakeConfirmReq
		if err := json.Unmarshal([]byte(argument), &request); err != nil {
			return nil, fmt.Errorf("invalid import selection: %w", err)
		}
		return s.desktopIntakeConfirm(request)
	case "intakeCancel":
		return s.desktopIntakeCancel(argument), nil
	case "exitApp":
		if s.DesktopExit == nil {
			return nil, errors.New("desktop exit is unavailable")
		}
		go s.DesktopExit()
		return map[string]bool{"stopping": true}, nil
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
			On bool   `json:"on"`
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
	case "setGroupMember":
		var request groupMemberRequest
		if len(argument) > 4096 || json.Unmarshal([]byte(argument), &request) != nil || request.Member == nil {
			return nil, errors.New("Invalid membership")
		}
		if err := s.setGroupMember(request); err != nil {
			return nil, err
		}
		return map[string]bool{"saved": true}, nil
	case "replaceGroupMembership":
		var request replaceGroupMembershipRequest
		if len(argument) > 1<<20 || json.Unmarshal([]byte(argument), &request) != nil || request.Group == allDictionariesGroup || request.Members == nil {
			return nil, errors.New("Invalid group membership")
		}
		if err := s.replaceGroupMembership(request); err != nil {
			return nil, err
		}
		return map[string]bool{"saved": true}, nil
	case "addGroupMembers":
		var request addGroupMembersRequest
		if len(argument) > 1<<20 || json.Unmarshal([]byte(argument), &request) != nil || request.Group == "" || request.Group == allDictionariesGroup || request.Members == nil {
			return nil, errors.New("Invalid membership")
		}
		if err := s.addGroupMembers(request); err != nil {
			return nil, err
		}
		return map[string]bool{"saved": true}, nil
	case "linkGroup":
		var request linkGroupRequest
		if len(argument) > 4096 || json.Unmarshal([]byte(argument), &request) != nil ||
			request.Group == allDictionariesGroup || request.Filter != nil && !s.filterExists(*request.Filter) {
			return nil, errors.New("Filter not found")
		}
		if err := s.linkGroup(request); err != nil {
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
	case "saveLanguage":
		if len(argument) > 1024 {
			return nil, errors.New("bad request")
		}
		var request struct {
			Language string `json:"language"`
		}
		if err := json.Unmarshal([]byte(argument), &request); err != nil {
			return nil, errors.New("bad request")
		}
		if request.Language != "en" && request.Language != "ru" {
			return nil, errors.New("unsupported language")
		}
		if err := s.reg.prefs.setLanguage(request.Language); err != nil {
			return nil, errors.New("could not save language")
		}
		return map[string]string{"language": s.reg.prefs.Language()}, nil
	case "systemLog":
		content, filename, err := s.systemLogSnapshot()
		if err != nil {
			return nil, fmt.Errorf("could not read system log: %w", err)
		}
		return map[string]string{"content": string(content), "filename": filename}, nil
	case "userFiles":
		return s.userFilesPayload(), nil
	case "saveUserFile":
		const maxArgument = 4*((maxUserFileBytes+2)/3) + 1024
		if len(argument) > maxArgument {
			return nil, errors.New("file is too large")
		}
		var request struct {
			Name    string `json:"name"`
			Replace bool   `json:"replace"`
			Data    string `json:"data"`
		}
		if err := json.Unmarshal([]byte(argument), &request); err != nil {
			return nil, errors.New("bad request: invalid file upload")
		}
		body, err := base64.StdEncoding.DecodeString(request.Data)
		if err != nil {
			return nil, errors.New("bad request: invalid file data")
		}
		return s.saveUserFile(request.Name, request.Replace, body)
	case "deleteUserFile":
		return s.deleteUserFile(argument)
	case "setup":
		return s.DesktopSetup(argument)
	case "howto":
		switch argument {
		case "copy":
			return s.howtoOperation(false)
		case "restore":
			return s.howtoOperation(true)
		default:
			return nil, errors.New("unsupported howto operation")
		}
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
		base, ok := desktopOrigin(baseURL)
		if !ok {
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
		page, err := s.browsePage(values)
		if err != nil {
			var failure *browsePageFailure
			if errors.As(err, &failure) {
				body := map[string]any{"error": failure.message}
				if failure.unprepared {
					body["prepared"] = false
				}
				return map[string]any{"status": failure.status, "body": body}, nil
			}
			return map[string]any{"status": http.StatusInternalServerError,
				"body": map[string]any{"error": err.Error()}}, nil
		}
		return map[string]any{"status": http.StatusOK, "body": page}, nil
	}
	return nil, fmt.Errorf("unsupported desktop request %q", method)
}

// desktopLooks dispatches saved-appearance operations directly to the shared
// Go methods used by the browser and Android HTTP adapters.
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
	return s.lookOperation(call.Method, call.Path, []byte(call.Body))
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
