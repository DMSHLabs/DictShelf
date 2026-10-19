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
)

// setCrashOutputFile directs unrecovered Go panics and runtime-fatal errors to
// an additional persistent file. The runtime includes all goroutine stacks.
func setCrashOutputFile(f *os.File) error {
	debug.SetTraceback("all")
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}

// ConfigureCrashLog installs a separate crash file beside the retained system
// log. A non-empty file from the preceding run is kept as crash.log.1 so a
// normal restart does not erase the dump before the user can export it.
func ConfigureCrashLog() error {
	systemMu.Lock()
	root := systemDir()
	systemMu.Unlock()
	if root == "" {
		return fmt.Errorf("system log location is not configured")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	path := filepath.Join(root, "crash.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		previous := path + ".1"
		if err := os.Remove(previous); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(path, previous); err != nil {
			return err
		}
	}
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
	return nil
}
