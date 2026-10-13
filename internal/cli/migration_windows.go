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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wuweidict/wudict/internal/config"
)

var (
	migrationMessageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	migrationUILanguage = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")
)

const (
	migrationYesNoCancel   = 0x00000003
	migrationIconInfo      = 0x00000040
	migrationSetForeground = 0x00010000
	migrationTopmost       = 0x00040000
	migrationYes           = 6
	migrationNo            = 7
	migrationCancel        = 2
)

var legacyForkItems = []string{"state.json", "style", "groups.ini"}

// prepareWindowsDataRoot separates wuDict2's Windows files from upstream
// wuDict. The environment override is read by config and store defaults.
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
		return nil
	}
	if !migrationRequestedForLaunch() {
		return os.Setenv("WUDICT_HOME", newRoot)
	}
	newInfo, newErr := os.Stat(newRoot)
	if newErr != nil && !os.IsNotExist(newErr) {
		return fmt.Errorf("cannot check %s: %w", newRoot, newErr)
	}
	if newInfo != nil && !newInfo.IsDir() {
		return fmt.Errorf("the wuDict2 data path exists but is not a folder: %s", newRoot)
	}
	oldRoot := filepath.Join(home, ".wudict")
	oldInfo, oldErr := os.Stat(oldRoot)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return fmt.Errorf("cannot check %s: %w", oldRoot, oldErr)
	}
	if newInfo != nil {
		return os.Setenv("WUDICT_HOME", newRoot)
	}
	if oldInfo == nil || !oldInfo.IsDir() {
		return os.Setenv("WUDICT_HOME", newRoot)
	}
	forked, err := hasForkState(filepath.Join(oldRoot, "state.json"))
	if err != nil {
		return fmt.Errorf("cannot inspect the existing wuDict settings: %w", err)
	}
	choice, err := migrationChoice(forked)
	if err != nil {
		return err
	}
	if choice != migrationYes && choice != migrationNo {
		return fmt.Errorf("could not read the wuDict2 data choice (Windows returned %d)", choice)
	}
	if !forked && choice == migrationNo {
		if err := os.Setenv("WUDICT_HOME", newRoot); err != nil {
			return fmt.Errorf("cannot select the wuDict2 data folder: %w", err)
		}
		return startFreshSetup(newRoot)
	}
	if err := stopLegacyWindowsServer(oldRoot); err != nil {
		return err
	}
	if err := os.Setenv("WUDICT_HOME", newRoot); err != nil {
		return fmt.Errorf("cannot select the wuDict2 data folder: %w", err)
	}
	if err := importLegacyData(oldRoot, newRoot, forked); err != nil {
		return err
	}
	if forked && choice == migrationNo {
		for _, name := range append(append([]string(nil), legacyForkItems...), "lemmas") {
			if err := os.RemoveAll(filepath.Join(oldRoot, name)); err != nil {
				migrationNotice("wuDict2", "Данные перенесены в ~/.wuDict2, но часть старых файлов не удалось удалить. Проверьте папку ~/.wudict.")
				break
			}
		}
	}
	return os.Setenv("WUDICT_HOME", newRoot)
}

func startFreshSetup(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("cannot create ~/.wuDict2: %w", err)
	}
	// Point the first registry scan at an empty folder so the first page is
	// Edit Folders even when the upstream default folder already has dictionaries.
	dir := filepath.Join(root, "Dictionaries")
	if err := config.SaveKeyRaw(filepath.Join(root, config.Name), "DICT_DIR", config.FormatList([]string{dir})); err != nil {
		return fmt.Errorf("cannot prepare the new wuDict2 setup: %w", err)
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

func migrationChoice(forked bool) (int, error) {
	caption, _ := syscall.UTF16PtrFromString("wuDict2 — папка данных")
	body := "Найдена папка wuDict: ~/.wudict. Импортировать настройки путей к словарям и файлы лемм? Индексы будут созданы заново в формате wuDict2.\n\nДа — импортировать пути и леммы.\nНет — настроить wuDict2 как новую программу и открыть Edit Folders.\nОтмена — закрыть программу."
	if forked {
		body = "В ~/.wudict найдены настройки wuDict2. Их необходимо перенести в отдельную папку, чтобы wuDict и wuDict2 не использовали общие настройки.\n\nДа — скопировать настройки wuDict2 и леммы в ~/.wuDict2, оставив исходные файлы wuDict на месте.\nНет — перенести настройки wuDict2 и леммы в ~/.wuDict2. Индексы не переносятся: они будут созданы заново.\nОтмена — закрыть программу."
	}
	language, _, _ := migrationUILanguage.Call()
	if language&0x3ff != 0x19 {
		caption, _ = syscall.UTF16PtrFromString("wuDict2 data folder")
		body = "The wuDict folder was found: ~/.wudict. Import dictionary folder settings and lemma files? Indexes will be rebuilt in the wuDict2 format.\n\nYes — import dictionary paths and lemmas.\nNo — set up wuDict2 as a new program and open Edit Folders.\nCancel — exit."
		if forked {
			body = "wuDict2 settings were found in ~/.wudict. They need to be moved to a separate folder so wuDict and wuDict2 do not share settings.\n\nYes — copy wuDict2 settings and lemmas to ~/.wuDict2, leaving the original files in place.\nNo — move wuDict2 settings and lemmas to ~/.wuDict2. Indexes are rebuilt in the wuDict2 format.\nCancel — exit."
		}
	}
	message, _ := syscall.UTF16PtrFromString(body)
	owner, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow").Call()
	r, _, _ := migrationMessageBox.Call(owner, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(caption)), migrationYesNoCancel|migrationIconInfo|migrationSetForeground|migrationTopmost)
	runtime.KeepAlive(caption)
	runtime.KeepAlive(message)
	if r == 0 || r == migrationCancel {
		return 0, errMigrationCancelled
	}
	return int(r), nil
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
	if forked {
		if err := copyMigrationItems(oldRoot, newRoot, legacyForkItems); err != nil {
			return err
		}
	}
	lemmaTarget := filepath.Join(newRoot, "lemmas")
	if info, err := os.Stat(lemmaSource); err == nil && info.IsDir() {
		if err := copyMigrationPath(lemmaSource, lemmaTarget); err != nil {
			return fmt.Errorf("could not copy lemma files: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot inspect lemma folder %s: %w", lemmaSource, err)
	}
	if len(dirs) > 0 {
		if err := config.SaveKeyRaw(config.HomeConfig(), "DICT_DIR", config.FormatList(dirs)); err != nil {
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

func migrationRequestedForLaunch() bool {
	if guiLaunch || len(os.Args) < 2 {
		return true
	}
	cmd := os.Args[1]
	if cmd == "serve" || strings.HasPrefix(cmd, "-") {
		return true
	}
	return openFileArgs(cmd) != nil
}

func migrationNotice(title, message string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(message)
	migrationMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), migrationIconInfo)
	runtime.KeepAlive(t)
	runtime.KeepAlive(m)
}

func stopLegacyWindowsServer(oldRoot string) error {
	oldConfig := filepath.Join(oldRoot, config.Name)
	if _, err := os.Stat(oldConfig); os.IsNotExist(err) {
		return nil
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
	for _, key := range []string{"SERVER_IP", "SERVER_PORT", "AUTH", "AUTH_TOKEN"} {
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
	cfg, err := config.Load(oldConfig, nil)
	if err != nil {
		return fmt.Errorf("cannot read the old wuDict server port: %w", err)
	}
	addr := cfg.Addr()
	if _, ok := probeRunning(addr); !ok {
		return nil
	}
	host := probeHost(addr)
	base := "http://" + host
	u, _ := url.Parse(base + "/api/exit")
	if token := cfg.AuthToken; token != "" {
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
		return fmt.Errorf("wuDict is running on port %s but could not be stopped. Close wuDict and try again", cfg.Port)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("wuDict is running on port %s but did not accept the stop request (HTTP %d). Close wuDict and try again", cfg.Port, resp.StatusCode)
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
