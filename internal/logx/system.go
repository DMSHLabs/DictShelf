// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const SystemLogLimit = 2 * 1024 * 1024
const SystemLogFiles = 5
const maxSystemRecord = 16 * 1024

var systemMu sync.Mutex
var systemRoot func() string
var systemSessionPath string
var diagnosticURL = regexp.MustCompile(`https?://[^\s"<>]+`)
var canceledSystemErrorField = regexp.MustCompile(`(?i)\berror=(?:"(?:\\.|[^"\\])*context canceled"|context canceled)(?:$|[\s,;])`)
var emptySystemErrorField = regexp.MustCompile(`(?i)\berror=""`)
var systemLogEnabled = true
var systemLogBaseEvents = true
var systemLogWebEvents = true
var systemHasErrors bool
var systemErrorStatusKnown bool

// SetSystemLogRoot installs a path resolver. It must not call logging functions.
// One mutex owns configuration, complete records, rotation and snapshots.
func SetSystemLogRoot(root func() string) {
	systemMu.Lock()
	defer systemMu.Unlock()
	systemRoot = root
	systemSessionPath = ""
	systemHasErrors = false
	systemErrorStatusKnown = false
}

// StartSystemLogSession creates a new timestamped log for this application
// run. It is safe to call more than once; a process keeps its first session.
func StartSystemLogSession() error {
	systemMu.Lock()
	defer systemMu.Unlock()
	root := systemDir()
	if root == "" {
		return fmt.Errorf("system log location is not configured")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	unlock, err := systemFileGuard(root)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ensureSystemLogSession(root); err != nil {
		return err
	}
	pruneSystemLogSessions(root)
	return nil
}

// ConfigureSystemLog controls the retained diagnostic file. It does not
// change terminal output or error handling in callers.
func ConfigureSystemLog(enabled, baseEvents, webEvents bool) {
	systemMu.Lock()
	systemLogEnabled = enabled
	systemLogBaseEvents = baseEvents
	systemLogWebEvents = webEvents
	systemMu.Unlock()
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
	writeSystem(false, false, format, args...)
}

// SystemDiagnostic writes an explicitly enabled diagnostic without requiring
// the basic-events or WebView-events categories to be enabled.
func SystemDiagnostic(format string, args ...any) {
	writeSystem(false, true, format, args...)
}

// SystemDiagnosticIf records an explicitly enabled diagnostic. When it is
// disabled, error records still pass through the normal error-retention path.
func SystemDiagnosticIf(enabled bool, format string, args ...any) {
	if enabled {
		SystemDiagnostic(format, args...)
		return
	}
	message := fmt.Sprintf(format, args...)
	if isSystemError(message) {
		System("%s", message)
	}
}

// SystemError records an error and makes it visible in the panel's Info section.
func SystemError(format string, args ...any) {
	writeSystem(true, false, "error: "+format, args...)
}

func writeSystem(isError, diagnostic bool, format string, args ...any) {
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
	errorEvent := isError || isSystemError(message)
	if !systemLogEnabled || !diagnostic && !errorEvent && (isWebSystemEvent(message) && !systemLogWebEvents ||
		!isWebSystemEvent(message) && !systemLogBaseEvents) {
		return
	}
	root := systemDir()
	if root == "" || os.MkdirAll(root, 0o700) != nil {
		return
	}
	unlock, err := systemFileGuard(root)
	if err != nil {
		return
	}
	defer unlock()
	if ensureSystemLogSession(root) != nil {
		return
	}
	record := []byte(time.Now().UTC().Format(time.RFC3339Nano) + " " + message + "\n")
	path := systemSessionPath
	if st, err := os.Stat(path); err == nil && st.Size()+int64(len(record)) > SystemLogLimit {
		if err := os.Remove(fmt.Sprintf("%s.%d", path, SystemLogFiles-1)); err != nil && !os.IsNotExist(err) {
			return
		}
		for i := SystemLogFiles - 2; i >= 1; i-- {
			if err := os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1)); err != nil && !os.IsNotExist(err) {
				return
			}
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
	if _, err := f.Write(record); err == nil {
		if isError || isSystemError(message) {
			systemHasErrors = true
			systemErrorStatusKnown = true
		}
		_ = f.Sync()
	}
}

// SystemHasErrors reports whether any retained log for this library contains
// an error or crash record. The first check scans the bounded retained logs;
// later writes update the cached result.
func SystemHasErrors() bool {
	systemMu.Lock()
	defer systemMu.Unlock()
	if systemErrorStatusKnown {
		return systemHasErrors
	}
	root := systemDir()
	if root == "" {
		return false
	}
	unlock, err := systemFileGuard(root)
	if err != nil {
		return false
	}
	defer unlock()
	logs, err := retainedSystemLogFiles(root)
	if err != nil {
		return false
	}
	for _, path := range logs {
		contents, err := os.ReadFile(path)
		if err == nil && hasSystemErrorRecord(contents) {
			systemHasErrors = true
			break
		}
	}
	if !systemHasErrors {
		for _, name := range retainedCrashLogNames(root, logs) {
			if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Size() > 0 {
				systemHasErrors = true
				break
			}
		}
	}
	systemErrorStatusKnown = true
	return systemHasErrors
}

func hasSystemErrorRecord(contents []byte) bool {
	for _, line := range strings.Split(strings.ToLower(string(contents)), "\n") {
		if isSystemError(line) {
			return true
		}
	}
	return false
}

func isSystemError(message string) bool {
	message = strings.ToLower(message)
	// Successful jobs include error="<nil>" in their diagnostic records.
	// Remove that value before looking for error markers so it does not make
	// the Info panel report a false positive.
	message = strings.ReplaceAll(message, `error="<nil>"`, "")
	message = strings.ReplaceAll(message, "error=<nil>", "")
	// Successful diagnostic records may include an empty error field.
	message = emptySystemErrorField.ReplaceAllString(message, "")
	// Safe Stop cancels in-flight indexing jobs. Their context-canceled error
	// fields are expected and should not make the Info panel report a failure.
	message = canceledSystemErrorField.ReplaceAllString(message, "")
	return strings.Contains(message, " error:") || strings.Contains(message, " error=") ||
		strings.Contains(message, "event=error") || strings.Contains(message, "panic:") ||
		strings.Contains(message, "fatal error:") || strings.Contains(message, "unhandled exception") ||
		strings.Contains(message, "recovered")
}

func isWebSystemEvent(message string) bool {
	message = strings.ToLower(message)
	return strings.HasPrefix(message, "web ") || strings.HasPrefix(message, "webview ")
}

// SystemSnapshot copies retained rotations under the writer's mutex. The caller
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
	logs, err := retainedSystemLogFiles(root)
	if err != nil {
		return nil, err
	}
	for _, path := range logs {
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		result = append(result, b...)
	}
	for _, name := range retainedCrashLogNames(root, logs) {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(b) > 0 {
			result = append(result, []byte("\n--- Crash details ("+name+") ---\n")...)
			result = append(result, b...)
		}
	}
	return result, nil
}

// ClearSystemLog removes retained diagnostic history and truncates the active
// session so logging can continue in the same file. The active crash outputs
// remain open and are kept for this process.
func ClearSystemLog() error {
	systemMu.Lock()
	defer systemMu.Unlock()
	root := systemDir()
	if root == "" {
		return fmt.Errorf("system log location is not configured")
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		systemHasErrors = false
		systemErrorStatusKnown = true
		return nil
	} else if err != nil {
		return err
	}
	unlock, err := systemFileGuard(root)
	if err != nil {
		return err
	}
	defer unlock()
	systemErrorStatusKnown = false

	activeLog := ""
	activeCrashPrefix := ""
	if systemSessionPath != "" && filepath.Dir(systemSessionPath) == root {
		activeLog = filepath.Base(systemSessionPath)
		stamp := strings.TrimSuffix(strings.TrimPrefix(activeLog, "system_"), ".log")
		activeCrashPrefix = "crash_" + stamp
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == activeLog && activeLog != "" {
			continue
		}
		if strings.HasPrefix(name, activeCrashPrefix) && activeCrashPrefix != "" &&
			(name == activeCrashPrefix+".log" || name == activeCrashPrefix+"_windows.log" || name == activeCrashPrefix+"_windows.dmp") {
			continue
		}
		if !clearableSystemLogName(name) && !clearableCrashLogName(name) {
			continue
		}
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(root, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if activeLog != "" {
		f, err := os.OpenFile(filepath.Join(root, activeLog), os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if f != nil {
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	systemHasErrors = false
	systemErrorStatusKnown = true
	return nil
}

func clearableSystemLogName(name string) bool {
	if name == "system.log" {
		return true
	}
	if strings.HasPrefix(name, "system.log.") {
		return numericRotation(name[len("system.log."):])
	}
	if !strings.HasPrefix(name, "system_") {
		return false
	}
	marker := strings.Index(name, ".log")
	if marker < 0 {
		return false
	}
	suffix := name[marker+4:]
	if suffix == "" {
		return true
	}
	if !strings.HasPrefix(suffix, ".") || len(suffix) == 1 {
		return false
	}
	return numericRotation(suffix[1:])
}

func numericRotation(suffix string) bool {
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func clearableCrashLogName(name string) bool {
	return strings.HasPrefix(name, "crash_") &&
		(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, "_windows.log") || strings.HasSuffix(name, "_windows.dmp")) ||
		name == "crash.log" || name == "crash.log.1"
}

func ensureSystemLogSession(root string) error {
	if systemSessionPath != "" && filepath.Dir(systemSessionPath) == root {
		return nil
	}
	stamp := time.Now().Format("2006_01_02__15_04_05")
	// A pruned session may leave a hole at the beginning of this second's
	// suffix sequence. Reusing that lexically oldest name would make the next
	// prune delete the session we just opened. Continue after the highest
	// existing suffix so lexical order remains chronological within a second.
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	maxSuffix := -1
	base := "system_" + stamp
	for _, entry := range entries {
		name := entry.Name()
		if name == base+".log" {
			maxSuffix = max(maxSuffix, 0)
			continue
		}
		if !strings.HasPrefix(name, base+"_") || !strings.HasSuffix(name, ".log") {
			continue
		}
		suffixText := strings.TrimSuffix(strings.TrimPrefix(name, base+"_"), ".log")
		var suffix int
		if _, err := fmt.Sscanf(suffixText, "%d", &suffix); err == nil && suffix > maxSuffix {
			maxSuffix = suffix
		}
	}
	start := maxSuffix + 1
	if maxSuffix < 0 {
		start = 0
	}
	for suffix := start; suffix < 1000; suffix++ {
		name := base
		if suffix > 0 {
			name += fmt.Sprintf("_%02d", suffix)
		}
		path := filepath.Join(root, name+".log")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if closeErr := f.Close(); closeErr != nil {
				_ = os.Remove(path)
				return closeErr
			}
			systemSessionPath = path
			return nil
		}
		if !os.IsExist(err) {
			return err
		}
	}
	return fmt.Errorf("could not allocate a unique system log filename")
}

func retainedSystemLogFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var sessions []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "system_") && strings.HasSuffix(name, ".log") && !strings.Contains(name, ".log.") {
			sessions = append(sessions, name)
		}
	}
	sort.Strings(sessions)
	if len(sessions) > SystemLogFiles {
		sessions = sessions[len(sessions)-SystemLogFiles:]
	}
	var result []string
	// Include logs made by older builds in the first export after upgrade.
	for i := SystemLogFiles - 1; i >= 0; i-- {
		name := "system.log"
		if i > 0 {
			name = fmt.Sprintf("system.log.%d", i)
		}
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			result = append(result, path)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	for _, name := range sessions {
		for i := SystemLogFiles - 1; i >= 0; i-- {
			path := filepath.Join(root, name)
			if i > 0 {
				path = fmt.Sprintf("%s.%d", path, i)
			}
			if _, err := os.Stat(path); err == nil {
				result = append(result, path)
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
	}
	return result, nil
}

func retainedCrashLogNames(root string, logs []string) []string {
	var names []string
	seen := make(map[string]bool)
	for _, logPath := range logs {
		name := filepath.Base(logPath)
		if !strings.HasPrefix(name, "system_") {
			continue
		}
		stamp := strings.TrimPrefix(name, "system_")
		stamp = strings.SplitN(stamp, ".log", 2)[0]
		for _, crashName := range []string{"crash_" + stamp + ".log", "crash_" + stamp + "_windows.log"} {
			if seen[crashName] {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, crashName)); err == nil {
				names = append(names, crashName)
				seen[crashName] = true
			}
		}
	}
	for _, name := range []string{"crash.log.1", "crash.log"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			names = append(names, name)
		}
	}
	return names
}

func pruneSystemLogSessions(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var sessions []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "system_") && strings.HasSuffix(name, ".log") && !strings.Contains(name, ".log.") {
			sessions = append(sessions, name)
		}
	}
	sort.Strings(sessions)
	if len(sessions) <= SystemLogFiles {
		return
	}
	for _, name := range sessions[:len(sessions)-SystemLogFiles] {
		path := filepath.Join(root, name)
		_ = os.Remove(path)
		for i := 1; i < SystemLogFiles; i++ {
			_ = os.Remove(fmt.Sprintf("%s.%d", path, i))
		}
		crashName := "crash_" + strings.TrimSuffix(strings.TrimPrefix(name, "system_"), ".log") + ".log"
		_ = os.Remove(filepath.Join(root, crashName))
		_ = os.Remove(filepath.Join(root, "crash_"+strings.TrimSuffix(strings.TrimPrefix(name, "system_"), ".log")+"_windows.log"))
		_ = os.Remove(filepath.Join(root, "crash_"+strings.TrimSuffix(strings.TrimPrefix(name, "system_"), ".log")+"_windows.dmp"))
	}
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
