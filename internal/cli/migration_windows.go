// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wuweidict/wudict/internal/config"
	"github.com/wuweidict/wudict/internal/server"
	"github.com/wuweidict/wudict/internal/tray"
)

var windowsFreshDataRoot bool

const windowsPortableHomeEnv = "WUDICT_PORTABLE_HOME"

var legacyForkItems = []string{"db", "db.logs", "state.json", "style", "groups.ini"}

// prepareWindowsDataRoot selects DictShelf's separate Windows data folder.
// Legacy wuDict2 data is left untouched for explicit import/export instead.
func prepareWindowsDataRoot() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot identify the Windows user folder: %w", err)
	}
	if home == "" {
		return fmt.Errorf("cannot identify the Windows user folder")
	}
	newRoot := filepath.Join(home, ".dmshlabs", "dictshelf")
	if existing := os.Getenv("WUDICT_HOME"); existing != "" {
		windowsFreshDataRoot = false
		return ensureWindowsConfigName(existing)
	}
	// This variable is internal to the portable launcher. Do not let a stale
	// shell value change path resolution after a normal-mode fallback.
	_ = os.Unsetenv(windowsPortableHomeEnv)
	if portableRoot, found, portableErr := windowsPortableDataRoot(); portableErr != nil {
		warnPortableFallback(portableErr)
	} else if found {
		if err := activateWindowsPortableData(portableRoot); err != nil {
			warnPortableFallback(err)
		} else {
			windowsFreshDataRoot = false
			return nil
		}
	}
	if !windowsServerLaunch() {
		windowsFreshDataRoot = false
		if err := os.Setenv("WUDICT_HOME", newRoot); err != nil {
			return err
		}
		return ensureWindowsConfigName(newRoot)
	}
	if info, err := os.Stat(newRoot); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("DictShelf data path exists but is not a folder: %s", newRoot)
		}
		windowsFreshDataRoot = false
	} else if os.IsNotExist(err) {
		windowsFreshDataRoot = true
	} else {
		return fmt.Errorf("cannot inspect DictShelf data folder: %w", err)
	}
	if err := os.Setenv("WUDICT_HOME", newRoot); err != nil {
		return err
	}
	if err := ensureWindowsConfigName(newRoot); err != nil {
		return err
	}
	if windowsFreshDataRoot {
		if err := os.MkdirAll(newRoot, 0o755); err != nil {
			return fmt.Errorf("cannot create DictShelf data folder: %w", err)
		}
		cfg := filepath.Join(newRoot, config.Name)
		if _, err := os.Stat(cfg); os.IsNotExist(err) {
			dir := filepath.Join(newRoot, "Dictionaries")
			if err := config.SaveKeyRaw(cfg, "DICT_DIR", config.FormatList([]string{dir})); err != nil {
				return fmt.Errorf("cannot prepare DictShelf's first-run folder: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("cannot inspect DictShelf settings: %w", err)
		}
	}
	return nil
}

func windowsPortableDataRoot() (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf("cannot find DictShelf.exe: %w", err)
	}
	root := filepath.Join(filepath.Dir(exe), "Data")
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return root, true, fmt.Errorf("cannot inspect %s: %w", root, err)
	}
	if !info.IsDir() {
		return root, true, fmt.Errorf("%s exists but is not a folder", root)
	}
	configPath := filepath.Join(root, config.Name)
	dictRoot := filepath.Join(root, "Dictionaries")
	dictInfo, err := os.Stat(dictRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return root, true, fmt.Errorf("%s is missing", dictRoot)
		}
		return root, true, fmt.Errorf("cannot inspect %s: %w", dictRoot, err)
	}
	if !dictInfo.IsDir() {
		return root, true, fmt.Errorf("%s exists but is not a folder", dictRoot)
	}
	if configured := os.Getenv("CONFIG_PATH"); configured != "" && !sameWindowsPath(configured, configPath) {
		return root, true, fmt.Errorf("CONFIG_PATH points outside the portable data folder: %s", configured)
	}
	if configured := windowsCommandConfigPath(); configured != "" && !sameWindowsPath(configured, configPath) {
		return root, true, fmt.Errorf("the --config option points outside the portable data folder: %s", configured)
	}
	if err := ensureWindowsConfigName(root); err != nil {
		return root, true, err
	}
	if !fileExists(configPath) {
		return root, true, fmt.Errorf("%s is missing", configPath)
	}
	if err := normalizeWindowsPortableConfig(configPath); err != nil {
		return root, true, err
	}
	if err := validateWindowsPortableConfig(root, configPath); err != nil {
		return root, true, err
	}
	return root, true, nil
}

func normalizeWindowsPortableConfig(configPath string) error {
	if _, err := config.Load(configPath, nil); err != nil {
		return fmt.Errorf("cannot read portable settings: %w", err)
	}
	if err := config.RelativizePortableDictionaries(configPath); err != nil {
		return fmt.Errorf("cannot update portable dictionary paths: %w", err)
	}
	paths := []struct{ key, value string }{
		{"DB_DIR", config.QuoteTOML("db")},
		{"LEMMA_DIR", config.QuoteTOML("lemmas")},
	}
	for _, item := range paths {
		if err := config.SaveKeyRaw(configPath, item.key, item.value); err != nil {
			return fmt.Errorf("cannot update portable %s in %s: %w", item.key, configPath, err)
		}
	}
	return nil
}

func ensureWindowsConfigName(root string) error {
	if os.Getenv("CONFIG_PATH") != "" || windowsCommandConfigPath() != "" {
		return nil
	}
	current := filepath.Join(root, config.Name)
	if fileExists(current) {
		return nil
	}
	legacy := filepath.Join(root, config.LegacyName)
	if !fileExists(legacy) {
		return nil
	}
	var created []string
	if err := copyMigrationPathTracked(legacy, current, &created); err != nil {
		return fmt.Errorf("could not copy the existing settings to %s: %w", current, err)
	}
	return nil
}

func validateWindowsPortableConfig(root, configPath string) error {
	oldHome, hadHome := os.LookupEnv("WUDICT_HOME")
	oldPortable, hadPortable := os.LookupEnv(windowsPortableHomeEnv)
	defer func() {
		if hadHome {
			_ = os.Setenv("WUDICT_HOME", oldHome)
		} else {
			_ = os.Unsetenv("WUDICT_HOME")
		}
		if hadPortable {
			_ = os.Setenv(windowsPortableHomeEnv, oldPortable)
		} else {
			_ = os.Unsetenv(windowsPortableHomeEnv)
		}
	}()
	if err := os.Setenv("WUDICT_HOME", root); err != nil {
		return fmt.Errorf("cannot select portable data folder: %w", err)
	}
	if err := os.Setenv(windowsPortableHomeEnv, root); err != nil {
		return fmt.Errorf("cannot select portable path base: %w", err)
	}
	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return fmt.Errorf("cannot read portable settings: %w", err)
	}
	if len(cfg.Problems) > 0 {
		return fmt.Errorf("portable settings contain errors: %s", strings.Join(cfg.Problems, "; "))
	}
	if cfg.DBDir != "" && !windowsPathWithin(root, cfg.DBDir) {
		return fmt.Errorf("DB_DIR points outside the portable data folder: %s", cfg.DBDir)
	}
	if dbOverride := os.Getenv("WUDICT_DB_DIR"); dbOverride != "" {
		resolved := config.ExpandHome(dbOverride)
		if !windowsPathWithin(root, resolved) {
			return fmt.Errorf("WUDICT_DB_DIR points outside the portable data folder: %s", resolved)
		}
	}
	if cfg.LemmaDir != "" && !windowsPathWithin(root, cfg.LemmaDir) {
		return fmt.Errorf("LEMMA_DIR points outside the portable data folder: %s", cfg.LemmaDir)
	}
	return nil
}

func windowsPathWithin(root, path string) bool {
	absRoot, errRoot := filepath.Abs(root)
	absPath, errPath := filepath.Abs(path)
	if errRoot != nil || errPath != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func activateWindowsPortableData(root string) error {
	values := []struct{ key, value string }{
		{"WUDICT_HOME", root},
		{windowsPortableHomeEnv, root},
		{"CONFIG_PATH", filepath.Join(root, config.Name)},
	}
	var changed []struct {
		key, value string
		present    bool
	}
	for _, item := range values {
		if item.key == "CONFIG_PATH" && os.Getenv(item.key) != "" {
			continue
		}
		old, present := os.LookupEnv(item.key)
		changed = append(changed, struct {
			key, value string
			present    bool
		}{item.key, old, present})
		if err := os.Setenv(item.key, item.value); err != nil {
			for _, previous := range changed {
				if previous.present {
					_ = os.Setenv(previous.key, previous.value)
				} else {
					_ = os.Unsetenv(previous.key)
				}
			}
			return fmt.Errorf("cannot select the portable data folder: %w", err)
		}
	}
	return nil
}

func windowsCommandConfigPath() string {
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		for _, prefix := range []string{"--config=", "-config="} {
			if strings.HasPrefix(arg, prefix) {
				return strings.TrimPrefix(arg, prefix)
			}
		}
		if arg == "--config" || arg == "-config" {
			if i+1 < len(os.Args) {
				return os.Args[i+1]
			}
			return ""
		}
	}
	return ""
}

func warnPortableFallback(err error) {
	message := fmt.Sprintf("Portable mode is unavailable: %v\nStarting with the normal user data folder.", err)
	if guiLaunch {
		tray.Alert("DictShelf", message)
		return
	}
	fmt.Fprintln(os.Stderr, message)
}

func windowsServerLaunch() bool {
	if guiLaunch || len(os.Args) < 2 {
		return true
	}
	command := os.Args[1]
	return command == "serve" || strings.HasPrefix(command, "-") || openFileArgs(command) != nil
}

func legacyMigrationStatus() server.FirstRunMigration {
	if !windowsFreshDataRoot {
		return server.FirstRunMigration{}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return server.FirstRunMigration{}
	}
	oldRoot := filepath.Join(home, ".wudict")
	info, err := os.Stat(oldRoot)
	if err != nil || !info.IsDir() {
		return server.FirstRunMigration{}
	}
	forked, err := hasForkState(filepath.Join(oldRoot, "state.json"))
	if err != nil {
		return server.FirstRunMigration{}
	}
	return server.FirstRunMigration{Available: true, Forked: forked}
}

func applyLegacyMigration(action string, reg *server.Registry, refresh func(), currentConfig string) error {
	status := legacyMigrationStatus()
	if !status.Available {
		return fmt.Errorf("the old wuDict data folder is no longer available")
	}
	if action != "import" && action != "copy" && action != "move" {
		return fmt.Errorf("unknown migration choice")
	}
	if status.Forked && action == "import" || !status.Forked && (action == "copy" || action == "move") {
		return fmt.Errorf("migration choice does not match the data found")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	oldRoot, newRoot := filepath.Join(home, ".wudict"), filepath.Join(home, ".wuDict2")
	if err := stopLegacyWindowsServer(oldRoot, currentConfig); err != nil {
		return err
	}
	if err := importLegacyData(oldRoot, newRoot, status.Forked); err != nil {
		return err
	}
	windowsFreshDataRoot = false
	if refresh != nil {
		refresh()
	}
	// Re-read the effective paths from the config layers so the live registry
	// agrees with the next launch, including any explicit environment override.
	cfg, err := config.Load(config.HomeConfig(), nil)
	if err != nil {
		return fmt.Errorf("could not load imported dictionary folders: %w", err)
	}
	if err := reg.SetDirsDeferred(cfg.DictDirs); err != nil {
		return fmt.Errorf("could not open imported dictionary folders: %w", err)
	}
	if action == "move" {
		for _, name := range []string{"db", "db.logs", "lemmas", "style", "state.json", "groups.ini", config.LegacyName} {
			if err := os.RemoveAll(filepath.Join(oldRoot, name)); err != nil {
				return fmt.Errorf("data were copied, but %s could not be removed from ~/.wudict: %w", name, err)
			}
		}
	}
	return nil
}

func hasForkState(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return false, nil // an unreadable state file is not proof of a fork install
	}
	for _, key := range []string{"indexDefaults", "indexMigrated", "dslKnown", "dslPending", "dslRemoved", "damagedSources", "groups"} {
		if _, ok := state[key]; ok {
			return true, nil
		}
	}
	if ui, ok := state["ui"]; ok {
		var fields map[string]json.RawMessage
		if json.Unmarshal(ui, &fields) == nil {
			if _, ok := fields["fontWeight"]; ok {
				return true, nil
			}
		}
	}
	if dicts, ok := state["dicts"]; ok {
		var rows []map[string]json.RawMessage
		if json.Unmarshal(dicts, &rows) == nil {
			for _, row := range rows {
				if _, ok := row["groups"]; ok {
					return true, nil
				}
				if _, ok := row["pinned"]; ok {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func importLegacyData(oldRoot, newRoot string, forked bool) error {
	oldConfig := filepath.Join(oldRoot, config.LegacyName)
	var dirs []string
	configData := []byte(nil)
	lemmaSource := filepath.Join(oldRoot, "lemmas")
	if fileExists(oldConfig) {
		cfg, err := loadLegacyConfig(oldConfig)
		if err != nil {
			return fmt.Errorf("cannot read dictionary folders from wuDict settings: %w", err)
		}
		dirs = cfg.DictDirs
		for i := range dirs {
			dirs[i] = rewriteMigrationPath(dirs[i], oldRoot, newRoot)
		}
		if cfg.Origin("LEMMA_DIR") == config.OriginFile {
			lemmaSource = cfg.LemmaDir
		}
		configData, err = os.ReadFile(oldConfig)
		if err != nil {
			return fmt.Errorf("cannot read wuDict settings: %w", err)
		}
	}
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		return fmt.Errorf("cannot create ~/.wuDict2: %w", err)
	}
	var createdPaths []string
	rollback := func() {
		for i := len(createdPaths) - 1; i >= 0; i-- {
			_ = os.Remove(createdPaths[i])
		}
	}
	if forked {
		if err := copyMigrationItems(oldRoot, newRoot, legacyForkItems, &createdPaths); err != nil {
			return err
		}
	}
	lemmaTarget := filepath.Join(newRoot, "lemmas")
	if info, err := os.Stat(lemmaSource); err == nil && info.IsDir() {
		if err := copyMigrationPathTracked(lemmaSource, lemmaTarget, &createdPaths); err != nil {
			rollback()
			return fmt.Errorf("could not copy lemma files: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		rollback()
		return fmt.Errorf("cannot inspect lemma folder %s: %w", lemmaSource, err)
	}
	if len(configData) > 0 {
		// Keep every setting from the old config while moving DictShelf's data
		// paths into its new home. Fork indexes use the same format and are
		// copied above; upstream indexes are intentionally left behind.
		configData = rewriteMigrationConfigPaths(configData, oldRoot, newRoot)
		if err := installMigratedConfig(config.HomeConfig(), newRoot, configData, dirs); err != nil {
			rollback()
			return fmt.Errorf("cannot save DictShelf settings: %w", err)
		}
	}
	return nil
}

func rewriteMigrationConfigPaths(data []byte, oldRoot, newRoot string) []byte {
	return []byte(rewriteMigrationPath(string(data), oldRoot, newRoot))
}

func rewriteMigrationPath(text, oldRoot, newRoot string) string {
	oldClean, newClean := filepath.Clean(oldRoot), filepath.Clean(newRoot)
	text = replacePathFold(text, oldClean, newClean)
	text = replacePathFold(text, strings.ReplaceAll(oldClean, `\`, `\\`), strings.ReplaceAll(newClean, `\`, `\\`))
	text = replacePathFold(text, filepath.ToSlash(oldClean), filepath.ToSlash(newClean))
	text = replacePathFold(text, "~/.wudict", "~/.wuDict2")
	text = renameMigrationComment(text)
	return text
}

func renameMigrationComment(text string) string {
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		quote := byte(0)
		escaped := false
		for j := 0; j < len(line); j++ {
			c := line[j]
			if quote != 0 {
				if quote == '"' && c == '\\' && !escaped {
					escaped = true
					continue
				}
				if c == quote && !escaped {
					quote = 0
				}
				escaped = false
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				continue
			}
			if c == '#' {
				lines[i] = line[:j] + renameMigrationCommentWords(line[j:])
				break
			}
		}
	}
	return strings.Join(lines, "")
}

func renameMigrationCommentWords(comment string) string {
	var out strings.Builder
	for i := 0; i < len(comment); {
		if i+6 <= len(comment) && strings.EqualFold(comment[i:i+6], "wudict") &&
			(i == 0 || !isConfigWordByte(comment[i-1])) &&
			(i+6 == len(comment) || !isConfigWordByte(comment[i+6])) {
			if strings.HasPrefix(strings.ToLower(comment[i+6:]), ".toml") {
				out.WriteString("wudict2.toml")
				i += len("wudict.toml")
				continue
			} else {
				out.WriteString("DictShelf")
				i += 6
				continue
			}
		}
		out.WriteByte(comment[i])
		i++
	}
	return out.String()
}

func isConfigWordByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func replacePathFold(text, oldPath, newPath string) string {
	if oldPath == "" {
		return text
	}
	lowerText, lowerOld := strings.ToLower(text), strings.ToLower(oldPath)
	var out strings.Builder
	for start := 0; ; {
		i := strings.Index(lowerText[start:], lowerOld)
		if i < 0 {
			out.WriteString(text[start:])
			break
		}
		i += start
		end := i + len(oldPath)
		if end < len(text) && text[end] != '\\' && text[end] != '/' && text[end] != '"' && text[end] != '\'' && text[end] != ' ' && text[end] != '\t' && text[end] != '#' {
			out.WriteString(text[start : i+1])
			start = i + 1
			continue
		}
		out.WriteString(text[start:i])
		out.WriteString(newPath)
		start = end
	}
	return out.String()
}

func installMigratedConfig(path, root string, data []byte, dirs []string) error {
	tmp, err := os.CreateTemp(root, ".wuDict2-config-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if len(dirs) > 0 {
		if err := config.SaveKeyRaw(tmpPath, "DICT_DIR", config.FormatList(dirs)); err != nil {
			return err
		}
	}
	if err := config.SaveKeyRaw(tmpPath, "DB_DIR", config.QuoteTOML(filepath.Join(root, "db"))); err != nil {
		return err
	}
	if err := config.SaveKeyRaw(tmpPath, "LEMMA_DIR", config.QuoteTOML(filepath.Join(root, "lemmas"))); err != nil {
		return err
	}
	backup := ""
	if _, err := os.Lstat(path); err == nil {
		f, createErr := os.CreateTemp(root, ".wuDict2-config-backup-*")
		if createErr != nil {
			return createErr
		}
		backup = f.Name()
		if closeErr := f.Close(); closeErr != nil {
			os.Remove(backup)
			return closeErr
		}
		if removeErr := os.Remove(backup); removeErr != nil {
			return removeErr
		}
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if backup != "" {
			_ = os.Rename(backup, path)
		}
		return err
	}
	if backup != "" {
		_ = os.Remove(backup)
	}
	return nil
}

func loadLegacyConfig(path string) (config.Config, error) {
	var saved []struct {
		key, value string
		present    bool
	}
	for _, key := range []string{"WUDICT_HOME", "DICT_DIR", "LEMMA_DIR", "DB_DIR", "SERVER_PORT", "SERVER_IP", "AUTH", "AUTH_TOKEN"} {
		value, present := os.LookupEnv(key)
		saved = append(saved, struct {
			key, value string
			present    bool
		}{key, value, present})
		_ = os.Unsetenv(key)
	}
	defer func() {
		for _, item := range saved {
			if item.present {
				_ = os.Setenv(item.key, item.value)
			} else {
				_ = os.Unsetenv(item.key)
			}
		}
	}()
	return config.Load(path, nil)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func stopLegacyWindowsServer(oldRoot, currentConfig string) error {
	oldConfig := filepath.Join(oldRoot, config.LegacyName)
	configExists := true
	if _, err := os.Stat(oldConfig); os.IsNotExist(err) {
		configExists = false
	} else if err != nil {
		return fmt.Errorf("cannot inspect old wuDict settings: %w", err)
	}
	// Resolve the listener from the old file itself, not environment values
	// that may belong to this DictShelf launch.
	type savedEnv struct {
		key, value string
		present    bool
	}
	var saved []savedEnv
	for _, key := range []string{"WUDICT_HOME", "SERVER_IP", "SERVER_PORT", "AUTH", "AUTH_TOKEN"} {
		value, present := os.LookupEnv(key)
		saved = append(saved, savedEnv{key: key, value: value, present: present})
		_ = os.Unsetenv(key)
	}
	defer func() {
		for _, item := range saved {
			if item.present {
				_ = os.Setenv(item.key, item.value)
			} else {
				_ = os.Unsetenv(item.key)
			}
		}
	}()
	addr, port, authToken := net.JoinHostPort("127.0.0.1", "6888"), "6888", ""
	if configExists {
		cfg, err := config.Load(oldConfig, nil)
		if err != nil {
			return fmt.Errorf("cannot read the old wuDict server port: %w", err)
		}
		addr, port, authToken = cfg.Addr(), cfg.Port, cfg.AuthToken
	}
	inst, ok := probeRunning(addr)
	if !ok {
		return nil
	}
	// The just-started DictShelf may use the same default port as wuDict. Never
	// send Exit to the server handling the migration request itself.
	if sameWindowsPath(inst.ConfigPath, currentConfig) {
		return nil
	}
	if !sameWindowsPath(inst.ConfigPath, oldConfig) {
		return fmt.Errorf("a wuDict server is running on port %s with settings %s; close it before migrating the files", port, inst.ConfigPath)
	}
	host := probeHost(addr)
	base := "http://" + host
	u, _ := url.Parse(base + "/api/exit")
	if token := authToken; token != "" {
		q := u.Query()
		q.Set("k", token)
		u.RawQuery = q.Encode()
	} else if b, readErr := os.ReadFile(filepath.Join(oldRoot, "token")); readErr == nil {
		if token := strings.TrimSpace(string(b)); token != "" {
			q := u.Query()
			q.Set("k", token)
			u.RawQuery = q.Encode()
		}
	}
	// API shutdown requires a same-origin local request, and the identity probe
	// above ensures this port belongs to a wudict server.
	req, err := http.NewRequest(http.MethodPost, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Origin", base)
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("wuDict is running on port %s but could not be stopped. Close wuDict and try again", port)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("wuDict is running on port %s but did not accept the stop request (HTTP %d). Close wuDict and try again", port, resp.StatusCode)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if _, alive := probeRunning(addr); !alive {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("wuDict accepted the stop request but is still using its files. Close wuDict and try again")
}

func sameWindowsPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}

func copyMigrationItems(from, to string, names []string, created *[]string) error {
	if err := os.MkdirAll(to, 0o755); err != nil {
		return fmt.Errorf("cannot create ~/.wuDict2: %w", err)
	}
	rollback := func() {
		for i := len(*created) - 1; i >= 0; i-- {
			_ = os.Remove((*created)[i])
		}
	}
	for _, name := range names {
		src := filepath.Join(from, name)
		if _, err := os.Lstat(src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			rollback()
			return fmt.Errorf("cannot inspect %s: %w", src, err)
		}
		dst := filepath.Join(to, name)
		if err := copyMigrationPathTracked(src, dst, created); err != nil {
			rollback()
			return fmt.Errorf("could not copy %s: %w. The old data is unchanged", name, err)
		}
	}
	return nil
}

func copyMigrationPathTracked(from, to string, created *[]string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(from)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(to); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Symlink(link, to); err != nil {
			if os.IsExist(err) {
				return nil
			}
			return err
		}
		*created = append(*created, to)
		return nil
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return errors.New("unsupported file type")
		}
		if _, err := os.Lstat(to); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		in, err := os.Open(from)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			if os.IsExist(err) {
				return nil
			}
			return err
		}
		*created = append(*created, to)
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Chtimes(to, info.ModTime(), info.ModTime())
	}
	createdDir := false
	if err := os.Mkdir(to, info.Mode().Perm()); err != nil {
		if !os.IsExist(err) {
			return err
		}
		destInfo, statErr := os.Lstat(to)
		if statErr != nil {
			return statErr
		}
		if !destInfo.IsDir() {
			return fmt.Errorf("destination exists and is not a folder: %s", to)
		}
	} else {
		*created = append(*created, to)
		createdDir = true
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyMigrationPathTracked(filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name()), created); err != nil {
			return err
		}
	}
	if createdDir {
		return os.Chtimes(to, info.ModTime(), info.ModTime())
	}
	return nil
}
