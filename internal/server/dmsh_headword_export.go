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
	"unicode/utf16"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
)

const headwordExportKey = "headword-export"

type headwordExportRequest struct {
	Action       string   `json:"action"`
	Dictionaries []string `json:"dictionaries"`
	Path         string   `json:"path"`
	Separate     bool     `json:"separate"`
}

type headwordExportCompanion struct {
	Path string
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
	companions := headwordExportCompanions(entries)
	st, started := s.jobs.start(headwordExportKey, 0, jobStatus{Total: int64(len(entries) + len(companions))}, func(j *job) {
		err := s.runHeadwordExport(j, entries, companions, path, req.Separate)
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
	if isWindowsDeviceName(name) {
		name = "_" + name
	}
	// Leave room for the collision suffix and .txt on Windows' 255-unit limit.
	name = truncateUTF16(name, 243)
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

func isWindowsDeviceName(name string) bool {
	first := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if first == "CON" || first == "PRN" || first == "AUX" || first == "NUL" {
		return true
	}
	if len(first) == 4 && (strings.HasPrefix(first, "COM") || strings.HasPrefix(first, "LPT")) {
		return first[3] >= '1' && first[3] <= '9'
	}
	return false
}

func truncateUTF16(value string, maxUnits int) string {
	var out strings.Builder
	used := 0
	for _, r := range value {
		units := utf16.RuneLen(r)
		if units < 0 || used+units > maxUnits {
			break
		}
		out.WriteRune(r)
		used += units
	}
	return out.String()
}

func headwordExportFilename(e *entry) string {
	return filepath.Base(headwordExportSourcePath(e))
}

func headwordExportSourcePath(e *entry) string {
	path := e.Path
	if d, err := e.open(); err == nil {
		if source := d.Meta().Path; source != "" && !store.IsTextDB(source) {
			path = source
		}
	}
	return path
}

func headwordExportCompanions(entries []*entry) []headwordExportCompanion {
	var companions []headwordExportCompanion
	seen := make(map[string]bool)
	for _, e := range entries {
		if source := headwordExportSourcePath(e); source != "" {
			if companion, ok := dict.AbbrevCompanion(source); ok {
				key := dict.CanonPath(companion)
				if !seen[key] {
					seen[key] = true
					companions = append(companions, headwordExportCompanion{Path: companion})
				}
			}
		}
	}
	return companions
}

func (s *Server) runHeadwordExport(j *job, entries []*entry, companions []headwordExportCompanion, dir string, separate bool) error {
	var files []string
	writeFile := func(name string, fill func(*bufio.Writer) error) (err error) {
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
		if err = fill(out); err != nil {
			return err
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
	write := func(name string, group []*entry) error {
		return writeFile(name, func(out *bufio.Writer) error {
			for _, e := range group {
				if err := j.ctx.Err(); err != nil {
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
				title := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(strings.TrimSpace(d.Meta().Name))
				if title == "" {
					title = e.probeName()
				}
				if _, err := fmt.Fprintf(out, "### #%s\n", title); err != nil {
					return err
				}
				j.update(func(st *jobStatus) { st.CurrentTotal = int64(d.Meta().EntryCount) })
				if err := exporter.ExportHeadwords(j.ctx, out, func(n int64) { j.update(func(st *jobStatus) { st.CurrentDone = n }) }); err != nil {
					return err
				}
				j.update(func(st *jobStatus) { st.Done++ })
			}
			return nil
		})
	}
	writeCompanion := func(companion headwordExportCompanion) error {
		return writeFile(filepath.Base(companion.Path), func(out *bufio.Writer) error {
			if err := j.ctx.Err(); err != nil {
				return err
			}
			j.update(func(st *jobStatus) {
				st.Current = filepath.Base(companion.Path)
				st.CurrentDone = 0
				st.CurrentTotal = 0
			})
			reader, err := dict.OpenReader(companion.Path)
			if err != nil {
				return err
			}
			defer reader.Close()
			meta := reader.Meta()
			j.update(func(st *jobStatus) { st.CurrentTotal = int64(meta.EntryCount) })
			title := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(strings.TrimSpace(meta.Name))
			if title == "" {
				title = filepath.Base(companion.Path)
			}
			if _, err := fmt.Fprintf(out, "### #%s\n", title); err != nil {
				return err
			}
			tempDir, err := os.MkdirTemp("", "wudict-abrv-export-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tempDir)
			dbPath := filepath.Join(tempDir, "text.db")
			if _, err = store.IngestPlanContext(j.ctx, reader, dbPath, store.Plan{}, func(done, _ int) {
				j.update(func(st *jobStatus) { st.CurrentDone = int64(done) })
			}); err != nil {
				return err
			}
			indexed, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer indexed.Close()
			return indexed.ExportHeadwords(j.ctx, out, func(n int64) {
				j.update(func(st *jobStatus) { st.CurrentDone = n })
			})
		})
	}

	if separate {
		for _, e := range entries {
			if err := write(headwordExportFilename(e), []*entry{e}); err != nil {
				return err
			}
		}
	} else if err := write("headwords", entries); err != nil {
		return err
	}
	for _, companion := range companions {
		if err := writeCompanion(companion); err != nil {
			return err
		}
		j.update(func(st *jobStatus) { st.Done++ })
	}
	return nil
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
