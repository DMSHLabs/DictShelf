// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	webview "github.com/jchv/go-webview2"
	"github.com/wuweidict/wudict/internal/tray"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const windowsUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\{D73A049D-229D-4AA7-B711-1D50432EBF4F}_is1`

type windowsUpdateAsset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type windowsUpdateRequest struct {
	Action          string              `json:"action"`
	Version         string              `json:"version"`
	Installer       *windowsUpdateAsset `json:"installer"`
	Portable        *windowsUpdateAsset `json:"portable"`
	PortableFailure string              `json:"portableFailure"`
}

func bindWindowsUpdater(w webview.WebView) {
	_ = w.Bind("wudictUpdateWindows", func(raw string) error {
		var request windowsUpdateRequest
		if err := json.Unmarshal([]byte(raw), &request); err != nil {
			return fmt.Errorf("invalid update request")
		}
		go func() {
			var js string
			if request.Action == "status" {
				ready := windowsUpdateAlreadyDownloaded(request)
				versionJSON, _ := json.Marshal(request.Version)
				js = fmt.Sprintf("window.wudictWindowsUpdateStatus&&window.wudictWindowsUpdateStatus(%s,%t)", versionJSON, ready)
			} else if request.Action == "download" {
				err := downloadWindowsUpdateForRunningExecutable(request, func(percent int) {
					w.Dispatch(func() {
						w.Eval(fmt.Sprintf("window.wudictUpdateProgress&&window.wudictUpdateProgress(%d)", percent))
					})
				})
				message := ""
				if err != nil {
					message = err.Error()
				}
				messageJSON, _ := json.Marshal(message)
				versionJSON, _ := json.Marshal(request.Version)
				js = fmt.Sprintf("window.wudictUpdateDownloaded(%t,%s,%s)", err == nil, versionJSON, messageJSON)
			} else if request.Action == "install" {
				message, closeApp, err := installWindowsUpdate(request)
				if err != nil {
					message = err.Error()
				}
				messageJSON, _ := json.Marshal(message)
				js = fmt.Sprintf("window.wudictUpdateFinished(%t,%s,%t)", err == nil, messageJSON, closeApp)
			} else {
				messageJSON, _ := json.Marshal("invalid update action")
				js = fmt.Sprintf("window.wudictUpdateDownloaded(false,%s,%s)", `""`, messageJSON)
			}
			w.Dispatch(func() {
				w.Eval(js)
			})
		}()
		return nil
	})
	_ = w.Bind("wudictWindowsExit", func() error {
		w.Dispatch(func() { w.Terminate() })
		return nil
	})
}

func windowsUpdateAlreadyDownloaded(request windowsUpdateRequest) bool {
	if !windowsVersionPattern.MatchString(request.Version) {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if _, installed := installedWindowsExecutable(exe); installed {
		if request.Installer == nil {
			return false
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		path := filepath.Join(home, "Downloads", "DictShelf-setup-"+request.Version+".exe")
		return verifyWindowsUpdate(path, *request.Installer) == nil
	}
	if request.Portable == nil {
		return false
	}
	path := filepath.Join(filepath.Dir(exe), "_DictShelf.exe")
	return verifyWindowsUpdate(path, *request.Portable) == nil
}

var windowsVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)

func downloadWindowsUpdateForRunningExecutable(request windowsUpdateRequest, progress func(int)) error {
	if !windowsVersionPattern.MatchString(request.Version) {
		return fmt.Errorf("the update version is invalid")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate the running executable")
	}
	if _, installed := installedWindowsExecutable(exe); installed {
		if request.Installer == nil {
			return fmt.Errorf("the installer is not available for this release")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not locate the Downloads folder")
		}
		dir := filepath.Join(home, "Downloads")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("could not create the Downloads folder: %w", err)
		}
		return downloadWindowsUpdate(*request.Installer, filepath.Join(dir, "DictShelf-setup-"+request.Version+".exe"), progress)
	}
	if request.Portable == nil {
		return fmt.Errorf("the portable executable is not available for this release")
	}
	target, err := filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("cannot locate the running executable")
	}
	probe, err := os.CreateTemp(filepath.Dir(target), ".dictshelf-update-check-*")
	if err != nil {
		return fmt.Errorf("this folder does not allow updating the executable; download the new file and replace it manually")
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return downloadWindowsUpdate(*request.Portable, filepath.Join(filepath.Dir(target), "_DictShelf.exe"), progress)
}

func installWindowsUpdate(request windowsUpdateRequest) (string, bool, error) {
	if !windowsVersionPattern.MatchString(request.Version) {
		return "", false, fmt.Errorf("the update version is invalid")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf("cannot locate the running executable")
	}
	if installMode, installed := installedWindowsExecutable(exe); installed {
		if request.Installer == nil {
			return "", false, fmt.Errorf("the installer is not available for this release")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false, fmt.Errorf("could not locate the Downloads folder")
		}
		setup := filepath.Join(home, "Downloads", "DictShelf-setup-"+request.Version+".exe")
		if err := verifyWindowsUpdate(setup, *request.Installer); err != nil {
			return "", false, err
		}
		if err := exec.Command(setup, installMode).Start(); err != nil {
			return "", false, fmt.Errorf("could not start the installer: %w", err)
		}
		return "", false, nil
	}
	if request.Portable == nil {
		return "", false, fmt.Errorf("the portable executable is not available for this release")
	}
	target, err := filepath.Abs(exe)
	if err != nil {
		return "", false, fmt.Errorf("cannot locate the running executable")
	}
	newExe := filepath.Join(filepath.Dir(target), "_DictShelf.exe")
	if err := verifyWindowsUpdate(newExe, *request.Portable); err != nil {
		return "", false, err
	}
	if err := launchPortableReplacement(newExe, target, os.Getpid(), request.Portable.SHA256, request.PortableFailure); err != nil {
		return "", false, err
	}
	return "", true, nil
}

func installedWindowsExecutable(exe string) (string, bool) {
	absExe, err := filepath.Abs(exe)
	if err != nil {
		return "", false
	}
	for _, item := range []struct {
		root registry.Key
		mode string
	}{{registry.CURRENT_USER, "/CURRENTUSER"}, {registry.LOCAL_MACHINE, "/ALLUSERS"}} {
		key, err := registry.OpenKey(item.root, windowsUninstallKey, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		location, _, err := key.GetStringValue("InstallLocation")
		_ = key.Close()
		if err != nil || location == "" {
			continue
		}
		registered := filepath.Join(location, "DictShelf.exe")
		absRegistered, err := filepath.Abs(registered)
		if err == nil && strings.EqualFold(filepath.Clean(absExe), filepath.Clean(absRegistered)) {
			return item.mode, true
		}
	}
	return "", false
}

func downloadWindowsUpdate(asset windowsUpdateAsset, destination string, progress func(int)) error {
	u, err := url.Parse(asset.URL)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") ||
		!strings.HasPrefix(u.Path, "/DMSHLabs/DictShelf/releases/download/") {
		return fmt.Errorf("the update download link is invalid")
	}
	digest, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("the update file has no valid SHA-256 checksum")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("could not prepare the update download")
	}
	response, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("could not download the update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not download the update: HTTP %d", response.StatusCode)
	}
	const maxUpdateSize = 512 << 20
	if response.ContentLength > maxUpdateSize {
		return fmt.Errorf("the update file is unexpectedly large")
	}
	if progress != nil {
		progress(-1)
		if response.ContentLength > 0 {
			progress(0)
		}
	}
	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("could not prepare the update folder: %w", err)
	}
	f, err := os.CreateTemp(dir, ".dictshelf-update-download-*")
	if err != nil {
		return fmt.Errorf("could not create a temporary update file: %w", err)
	}
	name := f.Name()
	hash := sha256.New()
	buf := make([]byte, 64<<10)
	var written int64
	lastPercent := -1
	var copyErr error
	for {
		count, readErr := response.Body.Read(buf)
		if count > 0 {
			if written+int64(count) > maxUpdateSize {
				copyErr = fmt.Errorf("the update file is unexpectedly large")
				break
			}
			if _, copyErr = f.Write(buf[:count]); copyErr != nil {
				break
			}
			_, _ = hash.Write(buf[:count])
			written += int64(count)
			if progress != nil && response.ContentLength > 0 {
				percent := int(written * 100 / response.ContentLength)
				if percent > 100 {
					percent = 100
				}
				if percent != lastPercent {
					progress(percent)
					lastPercent = percent
				}
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				copyErr = readErr
			}
			break
		}
	}
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || written > maxUpdateSize {
		_ = os.Remove(name)
		return fmt.Errorf("could not save the update file")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.SHA256) {
		_ = os.Remove(name)
		return fmt.Errorf("the downloaded update failed its SHA-256 check")
	}
	if progress != nil {
		progress(100)
	}
	if err := moveWindowsFile(name, destination); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("could not move the update into place: %w", err)
	}
	return nil
}

func verifyWindowsUpdate(path string, asset windowsUpdateAsset) error {
	expected, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(expected) != sha256.Size {
		return fmt.Errorf("the update file has no valid SHA-256 checksum")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("the downloaded update is missing; download it again")
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, io.LimitReader(f, (512<<20)+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		return fmt.Errorf("could not verify the downloaded update")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.SHA256) {
		return fmt.Errorf("the downloaded update failed its SHA-256 check; download it again")
	}
	return nil
}

func launchPortableReplacement(download, target string, pid int, sha256, failureText string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not locate the portable update helper: %w", err)
	}
	helperExe, err := copyWindowsUpdaterHelper(exe)
	if err != nil {
		return fmt.Errorf("could not prepare the portable update helper: %w", err)
	}
	cmd := exec.Command(helperExe, windowsUpdateHelperArg, strconv.Itoa(pid), target, download, sha256, failureText)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(helperExe)
		return fmt.Errorf("could not start the portable update helper: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

const windowsUpdateHelperArg = "--dictshelf-update-helper"
const windowsUpdaterHelperEnv = "DICTShelf_UPDATER_HELPER_COPY"

func copyWindowsUpdaterHelper(exe string) (string, error) {
	source, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer source.Close()
	target, err := os.CreateTemp(os.TempDir(), "DictShelf-updater-*.exe")
	if err != nil {
		return "", err
	}
	path := target.Name()
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// A fresh app process removes a helper copy after its source process exits.
// The helper cannot delete its own executable while Windows has it mapped.
func cleanupWindowsUpdaterHelperState() {
	if path, ok := os.LookupEnv(windowsUpdaterHelperEnv); ok {
		_ = os.Unsetenv(windowsUpdaterHelperEnv)
		if path != "" {
			go func() {
				for range 120 {
					if err := os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
						return
					}
					time.Sleep(500 * time.Millisecond)
				}
			}()
		}
	}
	current, _ := os.Executable()
	if entries, err := filepath.Glob(filepath.Join(os.TempDir(), "DictShelf-updater-*.exe")); err == nil {
		for _, path := range entries {
			if !strings.EqualFold(filepath.Clean(path), filepath.Clean(current)) {
				_ = os.Remove(path)
			}
		}
	}
}

// runWindowsUpdateHelper is dispatched before normal startup so the helper
// does not acquire the desktop instance lock or initialize the main window.
func runWindowsUpdateHelper(args []string) bool {
	if len(args) == 0 || args[0] != windowsUpdateHelperArg {
		return false
	}
	if len(args) != 6 {
		tray.Alert("DictShelf", "The portable update helper received invalid arguments.")
		return true
	}

	pid, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil || pid == 0 {
		tray.Alert("DictShelf", args[5])
		return true
	}
	exe, err := os.Executable()
	if err != nil || !strings.HasPrefix(strings.ToLower(filepath.Base(exe)), "dictshelf-updater-") ||
		strings.EqualFold(filepath.Clean(exe), filepath.Clean(args[2])) ||
		!strings.EqualFold(filepath.Base(args[2]), "DictShelf.exe") ||
		!strings.EqualFold(filepath.Clean(args[3]), filepath.Join(filepath.Dir(args[2]), "_DictShelf.exe")) {
		tray.Alert("DictShelf", args[5])
		return true
	}
	if err := waitForWindowsProcess(uint32(pid)); err != nil {
		tray.Alert("DictShelf", args[5]+"\n\n"+err.Error())
		return true
	}

	target, download := args[2], args[3]
	if err := verifyWindowsUpdate(download, windowsUpdateAsset{SHA256: args[4]}); err != nil {
		tray.Alert("DictShelf", args[5]+"\n\n"+err.Error())
		return true
	}
	backup := target + ".bak"
	_ = os.Remove(backup)
	if err := moveWindowsFile(target, backup); err != nil {
		tray.Alert("DictShelf", args[5]+"\n\n"+err.Error())
		return true
	}
	if err := moveWindowsFile(download, target); err != nil {
		restoreErr := moveWindowsFile(backup, target)
		if restoreErr != nil {
			tray.Alert("DictShelf", args[5]+"\n\n"+err.Error()+"\n\nCould not restore the previous executable: "+restoreErr.Error())
			return true
		}
		_ = startWindowsExecutable(target, exe)
		tray.Alert("DictShelf", args[5]+"\n\n"+err.Error())
		return true
	}
	if err := startWindowsExecutable(target, exe); err != nil {
		_ = moveWindowsFile(target, download)
		restoreErr := moveWindowsFile(backup, target)
		if restoreErr == nil {
			_ = startWindowsExecutable(target, exe)
		} else {
			err = fmt.Errorf("%w; could not restore the previous executable: %v", err, restoreErr)
		}
		tray.Alert("DictShelf", args[5]+"\n\n"+err.Error())
	}
	return true
}

func waitForWindowsProcess(pid uint32) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		// The app may already have exited before Windows opened its handle.
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return fmt.Errorf("could not wait for DictShelf to close: %w", err)
	}
	defer windows.CloseHandle(h)
	result, err := windows.WaitForSingleObject(h, windows.INFINITE)
	if err != nil {
		return fmt.Errorf("could not wait for DictShelf to close: %w", err)
	}
	if result != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("could not wait for DictShelf to close (Windows result %d)", result)
	}
	return nil
}

func moveWindowsFile(from, to string) error {
	fromPtr, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(fromPtr, toPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func startWindowsExecutable(path, helperPath string) error {
	cmd := exec.Command(path)
	cmd.Dir = filepath.Dir(path)
	// DictShelf.exe is built as a GUI executable and has no console to hide.
	// Do not pass SW_HIDE as its startup show state: the WebView window must be
	// shown when the updater starts the replacement process.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), strings.ToUpper(windowsUpdaterHelperEnv)+"=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, windowsUpdaterHelperEnv+"="+helperPath)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
