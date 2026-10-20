// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

// setCrashOutputFile directs unrecovered Go panics and runtime-fatal errors to
// an additional persistent file. The runtime includes all goroutine stacks.
func setCrashOutputFile(f *os.File) error {
	debug.SetTraceback("all")
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}

// ConfigureCrashLog installs a crash file paired with the current run's system
// log, preserving the traceback beside the diagnostics from the same run.
func ConfigureCrashLog() error {
	if err := StartSystemLogSession(); err != nil {
		return err
	}
	systemMu.Lock()
	root := systemDir()
	sessionPath := systemSessionPath
	systemMu.Unlock()
	if root == "" {
		return fmt.Errorf("system log location is not configured")
	}
	if sessionPath == "" {
		return fmt.Errorf("system log session is not configured")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(sessionPath), "system_"), ".log")
	path := filepath.Join(root, "crash_"+stamp+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := setCrashOutputFile(f); err != nil {
		return err
	}
	System("Go crash output armed file=%q runtime=%s platform=%s/%s pid=%d", path,
		runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Getpid())
	if err := configureNativeCrashCapture(root, sessionPath); err != nil {
		System("native crash capture unavailable error=%q", err)
	}
	return nil
}
