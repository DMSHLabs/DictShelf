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
)

var windowsFreshDataRoot bool

var legacyForkItems = []string{"state.json", "style", "groups.ini"}

// prepareWindowsDataRoot selects wuDict2's Windows data folder. Migration
// discovery and choices are handled after the first interface is shown.
func prepareWindowsDataRoot() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot identify the Windows user folder: %w", err)
	}
	if home == "" {
		return fmt.Errorf("cannot identify the Windows user folder")
	}
	newRoot := filepath.Join(home, ".wuDict2")
	if existing := os.Getenv("WUDICT_HOME"); existing != "" {
		windowsFreshDataRoot = false
		return nil
	}
	if !windowsServerLaunch() {
		windowsFreshDataRoot = false
		return os.Setenv("WUDICT_HOME", newRoot)
	}
	if info, err := os.Stat(newRoot); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("wuDict2 data path exists but is not a folder: %s", newRoot)
		}
		windowsFreshDataRoot = false
	} else if os.IsNotExist(err) {
		windowsFreshDataRoot = true
	} else {
		return fmt.Errorf("cannot inspect wuDict2 data folder: %w", err)
	}
	if err := os.Setenv("WUDICT_HOME", newRoot); err != nil {
		return err
	}
	if windowsFreshDataRoot {
		if err := os.MkdirAll(newRoot, 0o755); err != nil {
			return fmt.Errorf("cannot create wuDict2 data folder: %w", err)
		}
		cfg := filepath.Join(newRoot, config.Name)
		if _, err := os.Stat(cfg); os.IsNotExist(err) {
			dir := filepath.Join(newRoot, "Dictionaries")
			if err := config.SaveKeyRaw(cfg, "DICT_DIR", config.FormatList([]string{dir})); err != nil {
				return fmt.Errorf("cannot prepare wuDict2's first-run folder: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("cannot inspect wuDict2 settings: %w", err)
		}
	}
	return nil
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

func applyLegacyMigration(action string, reg *server.Registry, refresh func()) error {
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
	if err := stopLegacyWindowsServer(oldRoot); err != nil {
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
	if err := reg.SetDirs(cfg.DictDirs); err != nil {
		return fmt.Errorf("could not open imported dictionary folders: %w", err)
	}
	if action == "move" {
		for _, name := range []string{"db", "db.logs", "lemmas", "style", "state.json", "groups.ini", config.Name} {
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
	oldConfig := filepath.Join(oldRoot, config.Name)
	var dirs []string
	lemmaSource := filepath.Join(oldRoot, "lemmas")
	if fileExists(oldConfig) {
		cfg, err := loadLegacyConfig(oldConfig)
		if err != nil {
			return fmt.Errorf("cannot read dictionary folders from wuDict settings: %w", err)
		}
		dirs = cfg.DictDirs
		if cfg.Origin("LEMMA_DIR") == config.OriginFile {
			lemmaSource = cfg.LemmaDir
		}
	}
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		return fmt.Errorf("cannot create ~/.wuDict2: %w", err)
	}
	var rollbackNames []string
	rollback := func() {
		for _, name := range rollbackNames {
			_ = os.RemoveAll(filepath.Join(newRoot, name))
		}
	}
	if forked {
		if err := copyMigrationItems(oldRoot, newRoot, legacyForkItems); err != nil {
			return err
		}
		rollbackNames = append(rollbackNames, legacyForkItems...)
	}
	lemmaTarget := filepath.Join(newRoot, "lemmas")
	if info, err := os.Stat(lemmaSource); err == nil && info.IsDir() {
		if err := copyMigrationPath(lemmaSource, lemmaTarget); err != nil {
			rollback()
			return fmt.Errorf("could not copy lemma files: %w", err)
		}
		rollbackNames = append(rollbackNames, "lemmas")
	} else if err != nil && !os.IsNotExist(err) {
		rollback()
		return fmt.Errorf("cannot inspect lemma folder %s: %w", lemmaSource, err)
	}
	if len(dirs) > 0 {
		if err := config.SaveKeyRaw(config.HomeConfig(), "DICT_DIR", config.FormatList(dirs)); err != nil {
			rollback()
			return fmt.Errorf("cannot save dictionary folder settings: %w", err)
		}
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

func stopLegacyWindowsServer(oldRoot string) error {
	oldConfig := filepath.Join(oldRoot, config.Name)
	configExists := true
	if _, err := os.Stat(oldConfig); os.IsNotExist(err) {
		configExists = false
	} else if err != nil {
		return fmt.Errorf("cannot inspect old wuDict settings: %w", err)
	}
	// Resolve the listener from the old file itself, not environment values
	// that may belong to this wuDict2 launch.
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
	if _, ok := probeRunning(addr); !ok {
		return nil
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

func copyMigrationItems(from, to string, names []string) error {
	if err := os.MkdirAll(to, 0o755); err != nil {
		return fmt.Errorf("cannot create ~/.wuDict2: %w", err)
	}
	var completed []string
	rollback := func() {
		for _, name := range completed {
			_ = os.RemoveAll(filepath.Join(to, name))
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
		if err := copyMigrationPath(src, dst); err != nil {
			_ = os.RemoveAll(dst)
			rollback()
			_ = os.Remove(to)
			return fmt.Errorf("could not copy %s: %w. The old data is unchanged", name, err)
		}
		completed = append(completed, name)
	}
	return nil
}

func copyMigrationPath(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(link, to)
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return errors.New("unsupported file type")
		}
		in, err := os.Open(from)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			_ = os.Remove(to)
			return copyErr
		}
		if closeErr != nil {
			_ = os.Remove(to)
			return closeErr
		}
		return os.Chtimes(to, info.ModTime(), info.ModTime())
	}
	if err := os.Mkdir(to, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyMigrationPath(filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())); err != nil {
			return err
		}
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}
