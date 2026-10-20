// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const headwordExportKey = "headword-export"

type headwordExportRequest struct {
	Action       string   `json:"action"`
	Dictionaries []string `json:"dictionaries"`
	Path         string   `json:"path"`
	Separate     bool     `json:"separate"`
}

func (s *Server) headwordExport(argument string) (any, error) {
	var req headwordExportRequest
	if err := json.Unmarshal([]byte(argument), &req); err != nil {
		return nil, err
	}
	switch req.Action {
	case "status":
		st, _ := s.jobs.status(headwordExportKey)
		return st, nil
	case "stop":
		s.jobs.cancel(headwordExportKey)
		st, _ := s.jobs.status(headwordExportKey)
		return st, nil
	case "start":
	default:
		return nil, errors.New("invalid export action")
	}
	if len(req.Dictionaries) == 0 {
		return nil, errors.New("select dictionaries")
	}
	path, err := filepath.Abs(strings.TrimSpace(req.Path))
	if err != nil || strings.TrimSpace(req.Path) == "" {
		return nil, errors.New("choose an output folder")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("output path is not a folder")
	}
	var entries []*entry
	seen := map[string]bool{}
	for _, id := range req.Dictionaries {
		if seen[id] {
			continue
		}
		seen[id] = true
		e, err := s.reg.get(id)
		if err != nil {
			return nil, err
		}
		if !hasBrowseIndex(e.Path) {
			return nil, fmt.Errorf("%s: create a headword index in Dictionary Settings", e.probeName())
		}
		entries = append(entries, e)
	}
	st, started := s.jobs.start(headwordExportKey, 0, jobStatus{Total: int64(len(entries))}, func(j *job) {
		err := s.runHeadwordExport(j, entries, path, req.Separate)
		if err != nil {
			j.update(func(st *jobStatus) {
				if errors.Is(err, context.Canceled) {
					st.Canceled = true
				} else {
					st.Err = err.Error()
				}
			})
		}
	})
	if !started {
		return nil, errors.New("headword export is already running")
	}
	return st, nil
}

// createExportFile reserves a new name exclusively; existing files are never replaced.
func createExportFile(dir, name string) (*os.File, error) {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "all"
	}
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	name = "headwords-" + name
	for n := 1; n <= 10000; n++ {
		suffix := ""
		if n > 1 {
			suffix = fmt.Sprintf(" (%d)", n)
		}
		f, err := os.OpenFile(filepath.Join(dir, name+suffix+".txt"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, errors.New("no available export filename")
}

func (s *Server) runHeadwordExport(j *job, entries []*entry, dir string, separate bool) error {
	var files []string
	write := func(name string, group []*entry) (err error) {
		f, err := createExportFile(dir, name)
		if err != nil {
			return err
		}
		complete := false
		closed := false
		defer func() {
			if !closed {
				closeErr := f.Close()
				if err == nil {
					err = closeErr
				}
			}
			if !complete || err != nil {
				if removeErr := os.Remove(f.Name()); removeErr != nil {
					err = errors.Join(err, removeErr)
				}
			}
		}()
		out := bufio.NewWriterSize(f, 128*1024)
		for _, e := range group {
			if err = j.ctx.Err(); err != nil {
				return err
			}
			j.update(func(st *jobStatus) { st.Current = e.probeName(); st.CurrentDone = 0; st.CurrentTotal = 0 })
			d, openErr := e.open()
			if openErr != nil {
				return openErr
			}
			exporter, ok := d.(interface {
				ExportHeadwords(context.Context, io.Writer, func(int64)) error
			})
			if !ok {
				return fmt.Errorf("%s: headword export is unavailable", e.probeName())
			}
			j.update(func(st *jobStatus) { st.CurrentTotal = int64(d.Meta().EntryCount) })
			if err = exporter.ExportHeadwords(j.ctx, out, func(n int64) { j.update(func(st *jobStatus) { st.CurrentDone = n }) }); err != nil {
				return err
			}
			j.update(func(st *jobStatus) { st.Done++ })
		}
		if err = out.Flush(); err != nil {
			return err
		}
		if err = f.Sync(); err != nil {
			return err
		}
		if err = j.ctx.Err(); err != nil {
			return err
		}
		err = f.Close()
		closed = true
		if err != nil {
			return err
		}
		complete = true
		files = append(files, f.Name())
		j.update(func(st *jobStatus) { st.Result = append([]string(nil), files...) })
		return nil
	}
	if separate {
		for _, e := range entries {
			if err := write(e.probeName(), []*entry{e}); err != nil {
				return err
			}
		}
		return nil
	}
	return write("all", entries)
}

func (s *Server) handleHeadwordExport(w http.ResponseWriter, r *http.Request) {
	var req headwordExportRequest
	if !decodeJSON(w, r, &req, 1<<20) {
		return
	}
	data, err := json.Marshal(req)
	if err != nil {
		httpErr(w, 400, "%s", err.Error())
		return
	}
	result, err := s.headwordExport(string(data))
	if err != nil {
		httpErr(w, 400, "%s", err.Error())
		return
	}
	writeJSON(w, result)
}
