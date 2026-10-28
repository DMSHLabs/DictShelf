// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/wuweidict/wudict/internal/store"
	"github.com/wuweidict/wudict/internal/sysmem"
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
	header := fmt.Sprintf("DictShelf System Log\nBuild: %s\nCommit: %s\nVCS modified: %s\nSystem: %s\nRuntime: %s %s/%s\nProcess bitness: %d-bit\nLogical processors (Go runtime): %d\nGo scheduler processors (GOMAXPROCS): %d\nPhysical memory: %s\nCurrently available memory: %s\nGo memory limit: %s\nGo heap: allocated=%s (%d bytes) system=%s (%d bytes)\nTimestamps: UTC\n\n--- Server ---\n",
		version, revision, modified, systemVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH, strconv.IntSize,
		runtime.NumCPU(), runtime.GOMAXPROCS(0), totalMemoryText, availableText,
		memoryLimitText, formatLogBytes(int64(memory.Alloc)), memory.Alloc, formatLogBytes(int64(memory.Sys)), memory.Sys)
	filename := fmt.Sprintf("DictShelf_SystemLog_%s.log", time.Now().Format("2006_01_02-15-04"))
	return append([]byte(header), b...), filename, nil
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
