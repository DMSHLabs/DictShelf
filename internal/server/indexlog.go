// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/wuweidict/wudict/internal/store"
)

func (s *Server) handleIndexLog(w http.ResponseWriter, r *http.Request) {
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
	header := fmt.Sprintf("wuDict2 System Log\nBuild: %s\nCommit: %s\nVCS modified: %s\nSystem: %s\nRuntime: %s %s/%s\nTimestamps: UTC\n\n--- Server ---\n",
		version, revision, modified, systemVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	filename := fmt.Sprintf("wuDict2_SystemLog_%s.log", time.Now().Format("2006_01_02-15-04"))
	return append([]byte(header), b...), filename, nil
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
	return version, revision, modified
}
