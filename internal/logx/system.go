// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const SystemLogLimit = 2 * 1024 * 1024
const maxSystemRecord = 16 * 1024

var systemMu sync.Mutex
var systemRoot func() string
var diagnosticURL = regexp.MustCompile(`https?://[^\s"<>]+`)

// SetSystemLogRoot installs a path resolver. It must not call logging functions.
// One mutex owns configuration, complete records, rotation and snapshots.
func SetSystemLogRoot(root func() string) {
	systemMu.Lock()
	defer systemMu.Unlock()
	systemRoot = root
}

func systemDir() string {
	if systemRoot == nil {
		return ""
	}
	return filepath.Clean(systemRoot()) + ".logs"
}

// System writes diagnostics independently of verbose console output. Logging
// failures never replace the original failure and never recurse into logging.
func System(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	// URL queries can carry access keys. Keep the host/path for diagnosis.
	message = diagnosticURL.ReplaceAllStringFunc(message, func(value string) string {
		if pos := strings.IndexAny(value, "?#"); pos >= 0 {
			value = value[:pos] + "[redacted]"
		}
		if pos := strings.Index(value, "://"); pos >= 0 {
			start := pos + 3
			end := strings.Index(value[start:], "/")
			if end < 0 {
				end = len(value) - start
			}
			if at := strings.LastIndex(value[start:start+end], "@"); at >= 0 {
				value = value[:start] + value[start+at+1:]
			}
		}
		return value
	})
	message = strings.ReplaceAll(strings.ReplaceAll(message, "\r", `\r`), "\n", `\n`)
	if len(message) > maxSystemRecord {
		message = message[:maxSystemRecord] + " [truncated]"
	}
	systemMu.Lock()
	defer systemMu.Unlock()
	root := systemDir()
	if root == "" || os.MkdirAll(root, 0o700) != nil {
		return
	}
	unlock, err := systemFileGuard(root)
	if err != nil {
		return
	}
	defer unlock()
	record := []byte(time.Now().UTC().Format(time.RFC3339Nano) + " " + message + "\n")
	path := filepath.Join(root, "system.log")
	if st, err := os.Stat(path); err == nil && st.Size()+int64(len(record)) > SystemLogLimit {
		if err := os.Remove(path + ".1"); err != nil && !os.IsNotExist(err) {
			return
		}
		if os.Rename(path, path+".1") != nil {
			return
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(record)
	_ = f.Sync()
}

// SystemSnapshot copies both rotations under the writer's mutex. The caller
// sends the independent bytes after releasing it; slow exports cannot hold it.
func SystemSnapshot() ([]byte, error) {
	systemMu.Lock()
	defer systemMu.Unlock()
	root := systemDir()
	if root == "" {
		return nil, nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	unlock, err := systemFileGuard(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	var result []byte
	for _, name := range []string{"system.log.1", "system.log"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		result = append(result, b...)
	}
	return result, nil
}

// The stable lock file also coordinates a CLI process using the same library.
// It is never rotated or removed while a writer can be alive.
func systemFileGuard(root string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(root, "system.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	unlock, err := lockSystemFile(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { unlock(); _ = f.Close() }, nil
}
