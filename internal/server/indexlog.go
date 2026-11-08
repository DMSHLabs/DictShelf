// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/store"
	"github.com/DMSHLabs/DictShelf/internal/sysmem"
)

func (s *Server) handleIndexLog(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		if err := store.ClearIndexLog(); err != nil {
			httpErr(w, 500, "could not clear system log: %v", err)
			return
		}
		writeJSON(w, map[string]bool{"cleared": true})
		return
	}
	b, filename, err := s.systemLogSnapshot()
	if err != nil {
		httpErr(w, 500, "could not read system log: %v", err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

func (s *Server) systemLogSnapshot() ([]byte, string, error) {
	b, err := store.IndexLog()
	if err != nil {
		return nil, "", err
	}
	version, revision, modified := buildIdentity(s.Version)
	totalMemory := sysmem.Total()
	availableMemory, availableOK := sysmem.Available()
	availableText := "unavailable"
	if availableOK {
		availableText = fmt.Sprintf("%s (%d bytes)", formatLogBytes(availableMemory), availableMemory)
	}
	memoryLimit := debug.SetMemoryLimit(-1)
	memoryLimitText := "unlimited"
	if memoryLimit < int64(^uint64(0)>>1) {
		memoryLimitText = fmt.Sprintf("%s (%d bytes)", formatLogBytes(memoryLimit), memoryLimit)
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	totalMemoryText := "unavailable"
	if totalMemory > 0 {
		totalMemoryText = fmt.Sprintf("%s (%d bytes)", formatLogBytes(totalMemory), totalMemory)
	}
	header := fmt.Sprintf("DictShelf System Log\nBuild: %s\nCommit: %s\nVCS modified: %s\nSystem: %s\nRuntime: %s %s/%s\nProcess bitness: %d-bit\nLogical processors (Go runtime startup snapshot): %d\nCurrent indexing CPU allowance: %d\nGo scheduler processors (GOMAXPROCS): %d\nPhysical memory: %s\nCurrently available memory: %s\nGo memory limit: %s\nGo heap: allocated=%s (%d bytes) system=%s (%d bytes)\nTimestamps: UTC\n\n--- Server ---\n",
		version, revision, modified, systemVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH, strconv.IntSize,
		runtime.NumCPU(), currentIndexCPUCount(), runtime.GOMAXPROCS(0), totalMemoryText, availableText,
		memoryLimitText, formatLogBytes(int64(memory.Alloc)), memory.Alloc, formatLogBytes(int64(memory.Sys)), memory.Sys)
	filename := fmt.Sprintf("DictShelf_SystemLog_%s.log", time.Now().Format("2006_01_02-15-04"))
	var out strings.Builder
	out.WriteString(header)
	s.writeOutdatedIndexSummary(&out)
	out.Write(b)
	return []byte(out.String()), filename, nil
}

// writeOutdatedIndexSummary records the same per-dictionary selection that the
// Rebuild action uses, with the stored source name, user-facing name and stale
// stamps that explain why each prepared index needs work.
func (s *Server) writeOutdatedIndexSummary(out *strings.Builder) {
	type row struct{ name, source, path, reason string }
	rows := make([]row, 0)
	for _, e := range s.reg.all() {
		if !rebuildable(e.Path) {
			continue
		}
		dir, ok := store.LookupDir(e.Path)
		if !ok {
			continue
		}
		prepared := store.Inspect(store.TextDBPath(dir))
		nativeIndex := !isZIMPath(e.Path) && formatHasSelfIndex(prepared.Meta["format"])
		if !isZIMPath(e.Path) && prepared.Meta != nil && !preparedIndexesNeedRebuild(prepared.Meta["format"], prepared.Plan) {
			nativeIndex = true
		}
		if nativeIndex {
			continue
		}
		reasons := prepared.Stale(e.Path)
		if isZIMPath(e.Path) && !zimPreparedHeadwords(prepared) {
			reasons = append(reasons, store.Reason("zim-headwords"))
		}
		if len(reasons) == 0 {
			continue
		}
		name := prepared.Meta["name"]
		if name == "" {
			name = e.probeName()
		}
		label := s.reg.prefs.DisplayTitle(e.ID)
		if label == "" {
			label = name
		}
		rows = append(rows, row{name: label, source: name, path: e.Path, reason: strings.Join(reasonNames(reasons), ", ")})
	}
	fmt.Fprintf(out, "--- Dictionaries requiring rebuild: %d ---\n", len(rows))
	if len(rows) == 0 {
		out.WriteString("None\n\n")
		return
	}
	for _, r := range rows {
		fmt.Fprintf(out, "- %s (Name: %s; reason: %s)\n  Source: %s\n", r.name, r.source, r.reason, filepath.Clean(r.path))
	}
	out.WriteByte('\n')
}

func reasonNames(reasons []store.Reason) []string {
	names := make([]string, len(reasons))
	for i, reason := range reasons {
		names[i] = string(reason)
	}
	return names
}

func formatLogBytes(n int64) string {
	if n < 0 {
		return "unavailable"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for value := n / unit; value >= unit; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func buildIdentity(version string) (string, string, string) {
	revision := "unavailable"
	modified := "unavailable"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}
	}
	if version == "" {
		version = "unknown"
	}
	version = strings.TrimPrefix(version, "wudict2-")
	if version != "unknown" && !strings.HasPrefix(version, "DictShelf-") {
		version = "DictShelf-" + version
	}
	return version, revision, modified
}
