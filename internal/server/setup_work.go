// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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
