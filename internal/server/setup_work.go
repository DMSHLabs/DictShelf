// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

const setupWorkKey = "folder-setup"

type setupResponse struct {
	bytes.Buffer
	header http.Header
	code   int
}

func (w *setupResponse) Header() http.Header  { return w.header }
func (w *setupResponse) WriteHeader(code int) { w.code = code }

// Saving folders owns its discovery and registry update independently of the
// page. Read-only validation still belongs to the request that asked for it.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Query().Get("migration") == "" {
		httpErr(w, http.StatusBadRequest, "missing migration choice")
		return
	}
	if action := r.URL.Query().Get("migration"); action != "" {
		if !isLoopback(r) {
			httpErr(w, http.StatusForbidden, "first-run migration is available only on this computer")
			return
		}
		if action == "status" {
			status := FirstRunMigration{}
			if s.FirstRunMigrationStatus != nil {
				status = s.FirstRunMigrationStatus()
			}
			writeJSON(w, status)
			return
		}
		if s.ApplyFirstRunMigration == nil {
			httpErr(w, http.StatusNotFound, "first-run migration is unavailable")
			return
		}
		if err := s.ApplyFirstRunMigration(action); err != nil {
			httpErr(w, http.StatusConflict, "%v", err)
			return
		}
		writeJSON(w, map[string]bool{"saved": true})
		return
	}
	save, _ := queryFlag(r.URL.Query(), "save")
	if !save {
		s.handleSetupSync(w, r)
		return
	}
	request := r.Clone(context.Background())
	_, started := s.jobs.start(setupWorkKey, 0, jobStatus{Total: 1, Stage: "folders"}, func(j *job) {
		response := &setupResponse{header: make(http.Header), code: http.StatusOK}
		s.handleSetupSync(response, request)
		var result map[string]any
		if err := json.Unmarshal(response.Bytes(), &result); err != nil {
			result = map[string]any{"error": err.Error()}
		}
		j.update(func(st *jobStatus) {
			st.Result = response
			st.Done = 1
			if message, ok := result["error"].(string); ok {
				st.Err = message
			}
		})
	})
	if !started {
		httpErr(w, http.StatusConflict, "folder setup already running")
		return
	}
	if r.URL.Query().Get("async") == "1" {
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, s.indexWorkStatus(setupWorkKey))
		return
	}
	for {
		st, changed, _ := s.jobs.watch(setupWorkKey)
		if !st.Running {
			response := st.Result.(*setupResponse)
			for key, values := range response.header {
				w.Header()[key] = values
			}
			w.WriteHeader(response.code)
			_, _ = w.Write(response.Bytes())
			return
		}
		select {
		case <-changed:
		case <-r.Context().Done():
			return
		}
	}
}

// DesktopSetup exposes the setup page's migration actions to the Windows
// WebView bridge, with the same host callbacks as the HTTP adapter.
func (s *Server) DesktopSetup(argument string) (any, error) {
	values, err := url.ParseQuery(argument)
	if err != nil {
		return nil, err
	}
	action := values.Get("migration")
	if action == "status" {
		if s.FirstRunMigrationStatus == nil {
			return FirstRunMigration{}, nil
		}
		return s.FirstRunMigrationStatus(), nil
	}
	if action == "" || s.ApplyFirstRunMigration == nil {
		return nil, errors.New("first-run migration is unavailable")
	}
	if err := s.ApplyFirstRunMigration(action); err != nil {
		return nil, err
	}
	return map[string]bool{"saved": true}, nil
}

func (s *Server) DesktopFolderSetup(argument string) (any, error) {
	if len(argument) > 1<<20 {
		return nil, errors.New("folder request is too large")
	}
	values, err := url.ParseQuery(argument)
	if err != nil {
		return nil, err
	}
	if values.Get("save") != "1" {
		values.Del("save")
		result, _ := s.setupResult(values, true)
		return result, nil
	}
	_, started := s.jobs.start(setupWorkKey, 0, jobStatus{Total: 1, Stage: "folders"}, func(j *job) {
		result, _ := s.setupResult(values, true)
		j.update(func(st *jobStatus) {
			st.Result = result
			st.Done = 1
			if message, ok := result["error"].(string); ok {
				st.Err = message
			}
		})
	})
	if !started {
		return nil, errors.New("folder setup already running")
	}
	return s.indexWorkStatus(setupWorkKey), nil
}
