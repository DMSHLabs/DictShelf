// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/config"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

const portableJobKey = "create-portable"

func (s *Server) createPortable(argument string) (any, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("portable creation requires Windows")
	}
	if s.User == "" {
		return nil, errors.New("user data folder is unavailable")
	}
	var req struct {
		Action       string `json:"action"`
		Path         string `json:"path"`
		Dictionaries bool   `json:"dictionaries"`
		InPlace      bool   `json:"inPlace"`
	}
	if err := json.Unmarshal([]byte(argument), &req); err != nil {
		return nil, err
	}
	switch req.Action {
	case "status":
		st, _ := s.jobs.status(portableJobKey)
		return st, nil
	case "stop":
		s.jobs.cancel(portableJobKey)
		st, _ := s.jobs.status(portableJobKey)
		return st, nil
	case "source":
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		dir, err := filepath.EvalSymlinks(filepath.Dir(exe))
		if err != nil {
			return nil, err
		}
		return map[string]string{"Path": dir}, nil
	case "checkWrite":
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		dir, err := filepath.EvalSymlinks(filepath.Dir(exe))
		if err != nil {
			return nil, err
		}
		return map[string]bool{"Writable": portableCanWrite(dir) == nil}, nil
	case "start":
	default:
		return nil, errors.New("invalid portable action")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var source string
	if req.InPlace {
		source, err = filepath.EvalSymlinks(filepath.Dir(exe))
		if err != nil {
			return nil, err
		}
		if err = portableCanWrite(source); err != nil {
			return nil, fmt.Errorf("cannot write to application folder: %w", err)
		}
		req.Path = filepath.Join(source, "Portable")
	} else if strings.TrimSpace(req.Path) == "" {
		return nil, errors.New("choose an output folder")
	}
	target, err := filepath.Abs(req.Path)
	if err != nil {
		return nil, err
	}
	// Resolve existing ancestors, including junctions, before containment checks.
	target, err = portableResolveTarget(target)
	if err != nil {
		return nil, err
	}
	profile, err := filepath.EvalSymlinks(string(s.User))
	if err != nil {
		return nil, err
	}
	var roots []string
	if req.Dictionaries {
		for _, root := range s.reg.Roots() {
			p, err := filepath.EvalSymlinks(root.Path)
			if err != nil {
				return nil, err
			}
			roots = append(roots, p)
		}
	}
	for _, p := range append([]string{profile, store.DefaultDBDir(), s.LemmaDir}, roots...) {
		if p == "" {
			continue
		}
		p, err = filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		if resolved, e := filepath.EvalSymlinks(p); e == nil {
			p = resolved
		}
		if portableWithin(p, target) || portableWithin(target, p) {
			return nil, fmt.Errorf("output folder overlaps source: %s", p)
		}
	}
	if !req.InPlace {
		if rows, e := os.ReadDir(target); e == nil {
			if len(rows) != 0 {
				return nil, errors.New("choose an empty output folder")
			}
		} else if !os.IsNotExist(e) {
			return nil, e
		}
	} else if portableWithin(target, filepath.Dir(exe)) || portableWithin(filepath.Dir(exe), target) {
		return nil, errors.New("portable output folder overlaps the application folder")
	}
	st, started := s.jobs.start(portableJobKey, 0, jobStatus{}, func(j *job) {
		var err error
		if req.InPlace {
			err = s.buildPortableInPlace(j, profile, exe, target, roots)
		} else {
			err = s.buildPortable(j, profile, exe, target, roots)
		}
		if err != nil {
			j.update(func(st *jobStatus) {
				st.Canceled = errors.Is(err, context.Canceled)
				if !st.Canceled {
					st.Err = err.Error()
				}
			})
		}
	})
	if !started {
		return nil, errors.New("portable creation is already running")
	}
	return st, nil
}

func portableCanWrite(dir string) error {
	f, err := os.CreateTemp(dir, ".dictshelf-write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if err = f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func (s *Server) buildPortableInPlace(j *job, source, exe, target string, roots []string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	data := filepath.Dir(target)
	type copyTask struct {
		src, dst string
		excluded map[string]bool
		replace  bool
	}
	var tasks []copyTask
	excluded := map[string]bool{strings.ToLower(filepath.Clean(exe)): true}
	if s.reg != nil {
		for _, root := range s.reg.Roots() {
			if path, e := filepath.EvalSymlinks(root.Path); e == nil && portableWithin(source, path) && !strings.EqualFold(source, path) {
				excluded[strings.ToLower(path)] = true
			}
		}
	}
	tasks = append(tasks, copyTask{src: source, dst: data, excluded: excluded})
	tasks = append(tasks, copyTask{src: exe, dst: filepath.Join(target, filepath.Base(exe))})
	for _, item := range []struct{ src, name string }{{store.DefaultDBDir(), "db"}, {s.LemmaDir, "lemmas"}} {
		if item.src == "" {
			continue
		}
		if _, e := os.Stat(item.src); os.IsNotExist(e) {
			continue
		}
		dst := filepath.Join(data, item.name)
		if portableWithin(source, item.src) && strings.EqualFold(filepath.Clean(item.src), filepath.Join(source, item.name)) {
			continue
		}
		tasks = append(tasks, copyTask{src: item.src, dst: dst, replace: true})
	}
	dictionaries := filepath.Join(data, "Dictionaries")
	if err := os.MkdirAll(dictionaries, 0755); err != nil {
		return err
	}
	layout := portableLayout(roots)
	for i, root := range roots {
		tasks = append(tasks, copyTask{src: root, dst: filepath.Join(dictionaries, layout[i])})
	}
	cfg := filepath.Join(data, config.Name)
	if s.ConfigPath != "" && !strings.EqualFold(s.ConfigPath, filepath.Join(source, config.Name)) {
		tasks = append(tasks, copyTask{src: s.ConfigPath, dst: cfg})
	}
	j.update(func(st *jobStatus) { st.Stage = "counting"; st.Done = 0; st.Total = 0 })
	for _, task := range tasks {
		if err := portableCopyExcept(j, task.src, task.dst, map[string]bool{}, task.excluded, true); err != nil {
			return err
		}
	}
	j.update(func(st *jobStatus) { st.Stage = "copying"; st.Current = "" })
	for _, task := range tasks {
		if task.replace {
			if err := os.RemoveAll(task.dst); err != nil {
				return err
			}
		}
		if err := portableCopyExcept(j, task.src, task.dst, map[string]bool{}, task.excluded, false); err != nil {
			return err
		}
	}
	if len(roots) > 0 {
		if err := config.SaveKeyRaw(cfg, "DICT_DIR", config.FormatList([]string{"Dictionaries"})); err != nil {
			return err
		}
	}
	for _, item := range []struct{ key, value string }{{"DB_DIR", config.QuoteTOML("db")}, {"LEMMA_DIR", config.QuoteTOML("lemmas")}} {
		if err := config.SaveKeyRaw(cfg, item.key, item.value); err != nil {
			return err
		}
	}
	if err := j.ctx.Err(); err != nil {
		return err
	}
	j.update(func(st *jobStatus) { st.Done = st.Total; st.Result = target })
	return nil
}

func portableResolveTarget(p string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(p)
	if parent == p {
		return "", fmt.Errorf("cannot resolve %s", p)
	}
	resolved, err := portableResolveTarget(parent)
	return filepath.Join(resolved, filepath.Base(p)), err
}

func portableWithin(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// portableLayout removes the common ancestor on one volume. Across volumes,
// each volume name becomes a directory (C, D, or the UNC server/share).
func portableLayout(roots []string) []string {
	result := make([]string, len(roots))
	if len(roots) == 0 {
		return result
	}
	common := roots[0]
	for _, p := range roots[1:] {
		for !portableWithin(common, p) {
			parent := filepath.Dir(common)
			if parent == common {
				common = ""
				break
			}
			common = parent
		}
		if common == "" {
			break
		}
	}
	if len(roots) == 1 {
		common = roots[0]
	}
	for i, p := range roots {
		if common != "" {
			result[i], _ = filepath.Rel(common, p)
		} else {
			volume := filepath.VolumeName(p)
			label := strings.Trim(strings.ReplaceAll(strings.TrimSuffix(volume, ":"), `\`, string(filepath.Separator)), string(filepath.Separator))
			result[i] = filepath.Join(label, strings.TrimLeft(strings.TrimPrefix(p, volume), `\/`))
		}
	}
	return result
}

func (s *Server) buildPortable(j *job, source, exe, target string, roots []string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".wudict-portable-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage) // stage is the unique directory created above.
	data := filepath.Join(stage, "Data")
	type copyTask struct {
		src, dst string
		excluded map[string]bool
		replace  bool
	}
	var tasks []copyTask
	planCopy := func(src, dst string) {
		tasks = append(tasks, copyTask{src: src, dst: dst})
	}
	// Dictionary roots inside the profile follow the checkbox too. Their
	// destination is established by the same layout as external roots.
	excluded := map[string]bool{}
	if s.reg != nil {
		for _, root := range s.reg.Roots() {
			if path, e := filepath.EvalSymlinks(root.Path); e == nil && portableWithin(source, path) && !strings.EqualFold(source, path) {
				excluded[strings.ToLower(path)] = true
			}
		}
	}
	tasks = append(tasks, copyTask{src: source, dst: data, excluded: excluded})
	planCopy(exe, filepath.Join(stage, filepath.Base(exe)))
	// These paths can be configured outside the normal user-data directory.
	for _, item := range []struct{ src, name string }{{store.DefaultDBDir(), "db"}, {s.LemmaDir, "lemmas"}} {
		if item.src == "" {
			continue
		}
		if _, e := os.Stat(item.src); os.IsNotExist(e) {
			continue
		}
		dst := filepath.Join(data, item.name)
		if portableWithin(source, item.src) && strings.EqualFold(filepath.Clean(item.src), filepath.Join(source, item.name)) {
			continue
		}
		if !portableWithin(stage, dst) {
			return errors.New("invalid staging path")
		}
		tasks = append(tasks, copyTask{src: item.src, dst: dst, replace: true})
	}
	dictionaries := filepath.Join(data, "Dictionaries")
	if err = os.MkdirAll(dictionaries, 0755); err != nil {
		return err
	}
	layout := portableLayout(roots)
	for i, root := range roots {
		planCopy(root, filepath.Join(dictionaries, layout[i]))
	}
	cfg := filepath.Join(data, config.Name)
	// The active configuration may have a custom file name.
	if s.ConfigPath != "" && !strings.EqualFold(s.ConfigPath, filepath.Join(source, config.Name)) {
		planCopy(s.ConfigPath, cfg)
	}
	j.update(func(st *jobStatus) { st.Stage = "counting"; st.Done = 0; st.Total = 0 })
	for _, task := range tasks {
		if err = portableCopyExcept(j, task.src, task.dst, map[string]bool{}, task.excluded, true); err != nil {
			return err
		}
	}
	j.update(func(st *jobStatus) { st.Stage = "copying"; st.Current = "" })
	for _, task := range tasks {
		if task.replace {
			if err = os.RemoveAll(task.dst); err != nil {
				return err
			}
		}
		if err = portableCopyExcept(j, task.src, task.dst, map[string]bool{}, task.excluded, false); err != nil {
			return err
		}
	}
	if len(roots) > 0 {
		if err = config.SaveKeyRaw(cfg, "DICT_DIR", config.FormatList([]string{"Dictionaries"})); err != nil {
			return err
		}
	}
	for _, item := range []struct{ key, value string }{{"DB_DIR", config.QuoteTOML("db")}, {"LEMMA_DIR", config.QuoteTOML("lemmas")}} {
		if err = config.SaveKeyRaw(cfg, item.key, item.value); err != nil {
			return err
		}
	}
	if err = j.ctx.Err(); err != nil {
		return err
	}
	// Remove only an empty destination. A folder populated during copying is preserved.
	if _, err = os.Stat(target); err == nil {
		if err = os.Remove(target); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(stage, target); err != nil {
		return err
	}
	j.update(func(st *jobStatus) { st.Done = st.Total; st.Result = target })
	return nil
}

func portableCopy(j *job, src, dst string, ancestors map[string]bool) error {
	return portableCopyExcept(j, src, dst, ancestors, nil, false)
}

func portableCopyExcept(j *job, src, dst string, ancestors, excluded map[string]bool, measure bool) error {
	if err := j.ctx.Err(); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	if excluded[strings.ToLower(real)] {
		return nil
	}
	info, err := os.Stat(real)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if portableWithin(real, dst) {
			return fmt.Errorf("copy destination is inside source: %s", src)
		}
		key := strings.ToLower(real)
		if ancestors[key] {
			return fmt.Errorf("recursive directory link: %s", src)
		}
		ancestors[key] = true
		defer delete(ancestors, key)
		if !measure {
			if err = os.MkdirAll(dst, 0755); err != nil {
				return err
			}
		}
		rows, err := os.ReadDir(real)
		if err != nil {
			return err
		}
		for _, row := range rows {
			// WebView's live profile is a regenerable cache. Durable reader
			// settings are copied from settings.json and history.json.
			if home := os.Getenv("WUDICT_PORTABLE_HOME"); home != "" && strings.EqualFold(real, home) && strings.EqualFold(row.Name(), "WebView2") {
				continue
			}
			if err = portableCopyExcept(j, filepath.Join(real, row.Name()), filepath.Join(dst, row.Name()), ancestors, excluded, measure); err != nil {
				return fmt.Errorf("%s: %w", filepath.Join(src, row.Name()), err)
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file: %s", src)
	}
	// SQLite sidecars belong to the snapshot, never to a second copied WAL.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if strings.HasSuffix(src, suffix) && portableSQLite(strings.TrimSuffix(src, suffix)) {
			return nil
		}
	}
	if measure {
		j.update(func(st *jobStatus) { st.Total += info.Size(); st.Current = src })
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	j.update(func(st *jobStatus) { st.Current = src })
	if portableSQLite(real) {
		if _, err = os.Stat(dst); err == nil {
			j.update(func(st *jobStatus) { st.Done += info.Size() })
			return nil
		}
		err = store.SnapshotFile(j.ctx, real, dst)
		if err == nil {
			j.update(func(st *jobStatus) { st.Done += info.Size() })
		}
		return err
	}
	in, err := os.Open(real)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	buf := make([]byte, 1024*1024)
	for {
		if err = j.ctx.Err(); err != nil {
			break
		}
		var n int
		n, err = in.Read(buf)
		if n > 0 {
			var e error
			_, e = out.Write(buf[:n])
			if e != nil {
				err = e
				break
			}
			j.update(func(st *jobStatus) { st.Done += int64(n) })
		}
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

func portableSQLite(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var header [16]byte
	_, err = io.ReadFull(f, header[:])
	return err == nil && string(header[:]) == "SQLite format 3\x00"
}
