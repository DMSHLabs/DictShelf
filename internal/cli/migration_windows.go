// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
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
	migrationYesNo         = 0x00000004
	migrationYesNoCancel   = 0x00000003
	migrationIconInfo      = 0x00000040
	migrationSetForeground = 0x00010000
	migrationTopmost       = 0x00040000
	migrationYes           = 6
	migrationNo            = 7
	migrationCancel        = 2
	migrationFirstRun      = 0
)

var migrationItems = []string{"db", "db.logs", "lemmas", "style", "state.json", "wudict.toml"}

const migrationDecisionFile = ".wuDict2-migration-choice"

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
	if err := os.Setenv("WUDICT_HOME", newRoot); err != nil {
		return fmt.Errorf("cannot select the wuDict2 data folder: %w", err)
	}
	if !migrationRequestedForLaunch() {
		return nil
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
		return nil
	}
	if oldInfo == nil || !oldInfo.IsDir() {
		return nil
	}
	choice, err := migrationChoice(oldRoot)
	if err != nil {
		return err
	}
	if choice == migrationFirstRun {
		if err := copyMigrationItems(oldRoot, newRoot, []string{"db", "db.logs", "lemmas", "style"}); err != nil {
			return err
		}
		// This branch is only available when wuDict has no saved config, so no
		// wuDict.toml is copied and the first wuDict2 launch gets fresh defaults.
		return nil
	}
	if choice == migrationCancel { // Cancel / close: use a clean wuDict2 folder.
		return writeMigrationDecision(newRoot, "fresh")
	}
	if choice != migrationYes && choice != migrationNo {
		return fmt.Errorf("could not read the wuDict2 data choice (Windows returned %d)", choice)
	}
	if err := stopLegacyWindowsServer(oldRoot); err != nil {
		return err
	}
	if err := copyMigrationItems(oldRoot, newRoot, migrationItems); err != nil {
		return err
	}
	if choice == migrationNo { // Move: destination is complete before deleting sources.
		for _, name := range migrationItems {
			p := filepath.Join(oldRoot, name)
			if err := os.RemoveAll(p); err != nil {
				migrationNotice("wuDict2", "Данные скопированы в ~/.wuDict2, но не все исходные файлы удалось удалить. Старые файлы сохранены там, где Windows не разрешила удаление.")
				break
			}
		}
	}
	decision := "copy"
	if choice == migrationNo {
		decision = "move"
	}
	return writeMigrationDecision(newRoot, decision)
}

func writeMigrationDecision(root, choice string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("cannot create ~/.wuDict2: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, migrationDecisionFile), []byte(choice+"\n"), 0o600); err != nil {
		return fmt.Errorf("cannot save the wuDict2 data choice: %w", err)
	}
	return nil
}

func migrationChoice(oldRoot string) (int, error) {
	caption, _ := syscall.UTF16PtrFromString("wuDict2 — папка данных")
	oldConfig := filepath.Join(oldRoot, config.Name)
	oldHasConfig := fileExists(oldConfig)
	oldSafeToCopy := !oldHasConfig
	body := "Найдена папка wuDict: ~/.wudict, но в ней нет настроек wuDict.\n\nДа — настроить wuDict2 как новую программу. Старые файлы останутся без изменений.\nНет — скопировать db, db.logs, lemmas и style в ~/.wuDict2. Возможно, базы потребуется перестроить.\nОтмена — закрыть программу."
	if oldHasConfig {
		cfg, err := config.Load(oldConfig, nil)
		if err != nil {
			return 0, fmt.Errorf("cannot read the old wuDict config: %w", err)
		}
		oldSafeToCopy = cfg.DBDir != "" && filepath.Clean(cfg.DBDir) == filepath.Clean(filepath.Join(oldRoot, "db"))
		if oldSafeToCopy {
			body = "Найдена папка wuDict: ~/.wudict с настройками wuDict. Выберите, что сделать с настройками и данными.\n\nДа — скопировать в ~/.wuDict2 и оставить wuDict без изменений.\nНет — перенести в ~/.wuDict2 для использования только wuDict2.\nОтмена — начать работу с пустой папкой ~/.wuDict2.\n\nwuDict будет штатно остановлен перед копированием или переносом, если он запущен."
		} else {
			body = "Найдена папка wuDict: ~/.wudict с настройками. База индексов расположена вне старого каталога, поэтому безопасно перенести её не получится.\n\nДа — продолжить как новую установку wuDict2. Старые файлы останутся без изменений.\nОтмена — закрыть программу."
		}
	}
	language, _, _ := migrationUILanguage.Call()
	if language&0x3ff != 0x19 {
		caption, _ = syscall.UTF16PtrFromString("wuDict2 data folder")
		body = "The wuDict folder was found: ~/.wudict, but it has no wuDict settings.\n\nYes — set up wuDict2 as a new program. The old files will stay unchanged.\nNo — copy db, db.logs, lemmas and style to ~/.wuDict2. The indexes may need to be rebuilt.\nCancel — exit the program."
		if oldHasConfig && oldSafeToCopy {
			body = "The wuDict folder was found: ~/.wudict with wuDict settings. Choose what to do with its settings and data.\n\nYes — copy to ~/.wuDict2 and keep wuDict's files.\nNo — move to ~/.wuDict2 for wuDict2 only.\nCancel — start with an empty ~/.wuDict2 folder.\n\nIf wuDict is running, it will be stopped cleanly before copying or moving."
		} else if oldHasConfig {
			body = "The wuDict folder was found with settings, but its index library is outside the old folder and cannot be safely moved.\n\nYes — continue as a new wuDict2 installation. The old files will stay unchanged.\nCancel — exit the program."
		}
	}
	if !oldSafeToCopy {
		message, _ := syscall.UTF16PtrFromString(body)
		owner, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow").Call()
		r, _, callErr := migrationMessageBox.Call(owner, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(caption)), migrationYesNo|migrationIconInfo|migrationSetForeground|migrationTopmost)
		runtime.KeepAlive(caption)
		runtime.KeepAlive(message)
		if r == 0 {
			if callErr == nil {
				callErr = syscall.EINVAL
			}
			return 0, fmt.Errorf("could not show the wuDict2 data choice: %w", callErr)
		}
		if int(r) == migrationYes {
			return migrationFirstRun, nil
		}
		return migrationCancel, nil
	}
	message, _ := syscall.UTF16PtrFromString(body)
	owner, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow").Call()
	r, _, callErr := migrationMessageBox.Call(owner, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(caption)), migrationYesNoCancel|migrationIconInfo|migrationSetForeground|migrationTopmost)
	runtime.KeepAlive(caption)
	runtime.KeepAlive(message)
	if r == 0 {
		if callErr == nil {
			callErr = syscall.EINVAL
		}
		return 0, fmt.Errorf("could not show the wuDict2 data choice: %w", callErr)
	}
	return int(r), nil
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
