// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wuweidict/wudict/internal/config"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/tray"
)

const forkProfileName = ".wudict2"

// prepareWindowsForkProfile runs before any config, library, or log file is
// opened. A rename keeps the library, lemmas, styles, state and configuration
// together. If the destination already exists, leave both trees untouched.
func prepareWindowsForkProfile() error {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return fmt.Errorf("cannot locate the Windows user profile")
	}
	if os.Getenv("CONFIG_PATH") != "" || hasPortableConfig() {
		return nil
	}
	oldRoot := filepath.Join(home, ".wudict")
	newRoot := filepath.Join(home, forkProfileName)
	if info, err := os.Lstat(newRoot); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", newRoot)
		}
		return activateWindowsForkProfile(newRoot)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", newRoot, err)
	}
	oldInfo, err := os.Lstat(oldRoot)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", oldRoot, err)
	}
	if err == nil {
		if !oldInfo.IsDir() {
			return fmt.Errorf("%s is not a directory", oldRoot)
		}
		if running, endpoint := probeSharedWindowsServer(oldRoot); running {
			return fmt.Errorf("server at %s is still using %s; close it before moving the profile", endpoint, oldRoot)
		}
		choice, err := chooseProfileTransfer(oldRoot, newRoot)
		if err != nil {
			return err
		}
		switch choice {
		case transferCopy:
			if err := copyProfile(oldRoot, newRoot); err != nil {
				return fmt.Errorf("copying %s to %s: %w", oldRoot, newRoot, err)
			}
			logx.Status("wuDict2 profile copied from %s to %s", oldRoot, newRoot)
		case transferRename:
			if err := os.Rename(oldRoot, newRoot); err != nil {
				return fmt.Errorf("moving %s to %s: %w", oldRoot, newRoot, err)
			}
			logx.Status("wuDict2 profile moved from %s to %s", oldRoot, newRoot)
		default:
			return fmt.Errorf("wuDict2 needs its data in %s before it can start", newRoot)
		}
	}
	return activateWindowsForkProfile(newRoot)
}

func activateWindowsForkProfile(newRoot string) error {
	if err := os.Setenv("WUDICT_PROFILE", "wudict2"); err != nil {
		return err
	}
	if err := os.Setenv("WUDICT_DB_DIR", filepath.Join(newRoot, "db")); err != nil {
		return err
	}
	config.SetHomeConfigDir(forkProfileName)
	return nil
}

type profileTransfer int

const (
	transferCancel profileTransfer = iota
	transferCopy
	transferRename
)

func chooseProfileTransfer(oldRoot, newRoot string) (profileTransfer, error) {
	if !tray.GUILaunched() {
		fmt.Fprintf(os.Stderr, "Move wuDict2 data from %s to %s\n1. Rename (the original wuDict will stop working)\n2. Copy (keep both apps working)\nChoose 1 or 2: ", oldRoot, newRoot)
		var choice string
		if _, err := fmt.Fscanln(os.Stdin, &choice); err == nil {
			switch choice {
			case "1":
				return transferRename, nil
			case "2":
				return transferCopy, nil
			}
		}
		return transferCancel, nil
	}
	message := "Move all wuDict data from:\n" + oldRoot + "\n\nto:\n" + newRoot +
		"\n\nYes — Rename. The original wuDict will stop working." +
		"\nNo — Copy. Keep both applications working."
	text, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return transferCancel, err
	}
	title, err := syscall.UTF16PtrFromString("Move wuDict2 data")
	if err != nil {
		return transferCancel, err
	}
	const yesNo, warning, setForeground = 0x4, 0x30, 0x10000
	choice, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), yesNo|warning|setForeground)
	switch choice {
	case 6: // Yes: Rename
		return transferRename, nil
	case 7: // No: Copy
		return transferCopy, nil
	default:
		return transferCancel, nil
	}
}

func copyProfile(oldRoot, newRoot string) error {
	home := filepath.Dir(newRoot)
	temporary, err := os.MkdirTemp(home, ".wudict2-copy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := filepath.WalkDir(oldRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(oldRoot, path)
		if err != nil {
			return err
		}
		target := filepath.Join(temporary, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cannot copy symbolic link %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("cannot copy non-regular file %s", path)
		}
		return copyProfileFile(path, target)
	}); err != nil {
		return err
	}
	return os.Rename(temporary, newRoot)
}

func copyProfileFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, input)
	if copyErr == nil && written != info.Size() {
		copyErr = fmt.Errorf("size changed while copying %s", src)
	}
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func hasPortableConfig() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(filepath.Dir(exe), config.Name))
	return err == nil && info.Mode().IsRegular()
}

// probeSharedWindowsServer stops only a local wudict that reports the old
// library. An unidentified wudict on the configured port blocks the move.
func probeSharedWindowsServer(oldRoot string) (bool, string) {
	ports := []string{"6888"}
	if b, err := os.ReadFile(filepath.Join(oldRoot, config.Name)); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "SERVER_PORT") {
				if _, value, ok := strings.Cut(line, "="); ok {
					value = strings.Trim(strings.TrimSpace(strings.SplitN(value, "#", 2)[0]), "\"'")
					if value != "" && value != "6888" {
						ports = append(ports, value)
					}
				}
			}
		}
	}
	client := &http.Client{Timeout: 700 * time.Millisecond}
	for _, port := range ports {
		base := "http://127.0.0.1:" + port
		resp, err := client.Get(base + "/api/config")
		if err != nil {
			continue
		}
		if !strings.HasPrefix(resp.Header.Get("Server"), "wudict") {
			resp.Body.Close()
			continue
		}
		var cfg struct {
			LibDir string `json:"libDir"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cfg)
		resp.Body.Close()
		if decodeErr != nil || resp.StatusCode != http.StatusOK {
			return true, base
		}
		if !sameWindowsPath(cfg.LibDir, filepath.Join(oldRoot, "db")) {
			continue
		}
		if !stopSharedWindowsServer(client, base) {
			return true, base
		}
	}
	return false, ""
}

func stopSharedWindowsServer(client *http.Client, base string) bool {
	request, err := http.NewRequest(http.MethodPost, base+"/api/exit", nil)
	if err != nil {
		return false
	}
	request.Header.Set("Origin", base)
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return false
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(base + "/api/config")
		if err != nil {
			return true
		}
		response.Body.Close()
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func sameWindowsPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, errA := filepath.Abs(a)
	b, errB := filepath.Abs(b)
	return errA == nil && errB == nil && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
