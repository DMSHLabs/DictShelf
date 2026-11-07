// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const favouriteExportKey = "favourites-mdx-export"

type favouriteExportRequest struct {
	Action string `json:"action"`
	ID     string `json:"id"`
	Path   string `json:"path"`
}

func (s *Server) favouriteExport(argument string) (any, error) {
	var req favouriteExportRequest
	if len(argument) > 1<<20 || json.Unmarshal([]byte(argument), &req) != nil {
		return nil, errors.New("invalid favourites export request")
	}
	switch req.Action {
	case "status":
		st, _ := s.jobs.status(favouriteExportKey)
		return st, nil
	case "stop":
		s.jobs.cancel(favouriteExportKey)
		st, _ := s.jobs.status(favouriteExportKey)
		return st, nil
	case "start":
	default:
		return nil, errors.New("invalid favourites export action")
	}
	set, ok := s.favouriteStore(req.ID)
	if !ok {
		return nil, errors.New("unknown favourites collection")
	}
	if strings.TrimSpace(req.Path) == "" {
		return nil, errors.New("choose an output folder")
	}
	folder, err := filepath.Abs(strings.TrimSpace(req.Path))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(folder)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("output path is not a folder")
	}
	st, started := s.jobs.start(favouriteExportKey, 0, jobStatus{Total: 1, Current: req.ID}, func(j *job) {
		mdx, mdd, err := set.ExportMDX(j.ctx, folder)
		j.update(func(st *jobStatus) {
			if err != nil {
				if errors.Is(err, context.Canceled) {
					st.Canceled = true
				} else {
					st.Err = err.Error()
				}
				return
			}
			st.Done = 1
			st.Result = []string{mdx}
			if mdd != "" {
				st.Result = []string{mdx, mdd}
			}
		})
	})
	if !started {
		return nil, errors.New("favourites export is already running")
	}
	return st, nil
}

func (s *Server) handleFavouriteExport(w http.ResponseWriter, r *http.Request) {
	var req favouriteExportRequest
	if !decodeJSON(w, r, &req, 1<<20) {
		return
	}
	data, _ := json.Marshal(req)
	result, err := s.favouriteExport(string(data))
	if err != nil {
		httpErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	writeJSON(w, result)
}
