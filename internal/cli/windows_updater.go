// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"
)

const windowsUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\{D73A049D-229D-4AA7-B711-1D50432EBF4F}_is1`

type windowsUpdateAsset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type windowsUpdateRequest struct {
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
			message, closeApp, err := startWindowsUpdate(request)
			ok := err == nil
			if err != nil {
				message = err.Error()
			}
			messageJSON, _ := json.Marshal(message)
			js := fmt.Sprintf("window.wudictUpdateFinished(%t,%s,%t)", ok, messageJSON, closeApp)
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

func startWindowsUpdate(request windowsUpdateRequest) (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf("cannot locate the running executable")
	}
	if installMode, installed := installedWindowsExecutable(exe); installed {
		if request.Installer == nil {
			return "", false, fmt.Errorf("the installer is not available for this release")
		}
		setup, err := downloadWindowsUpdate(*request.Installer, "dictshelf-setup-*.exe")
		if err != nil {
			return "", false, err
		}
		if err := exec.Command(setup, installMode).Start(); err != nil {
			_ = os.Remove(setup)
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
	probe, err := os.CreateTemp(filepath.Dir(target), ".dictshelf-update-check-*")
	if err != nil {
		return "", false, fmt.Errorf("this folder does not allow updating the executable; download the new file and replace it manually")
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	newExe, err := downloadWindowsUpdate(*request.Portable, "dictshelf-portable-*.exe")
	if err != nil {
		return "", false, err
	}
	if err := launchPortableReplacement(newExe, target, os.Getpid(), request.PortableFailure); err != nil {
		_ = os.Remove(newExe)
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

func downloadWindowsUpdate(asset windowsUpdateAsset, pattern string) (string, error) {
	u, err := url.Parse(asset.URL)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") ||
		!strings.HasPrefix(u.Path, "/DMSHLabs/DictShelf/releases/download/") {
		return "", fmt.Errorf("the update download link is invalid")
	}
	digest, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return "", fmt.Errorf("the update file has no valid SHA-256 checksum")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("could not prepare the update download")
	}
	response, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return "", fmt.Errorf("could not download the update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not download the update: HTTP %d", response.StatusCode)
	}
	const maxUpdateSize = 512 << 20
	if response.ContentLength > maxUpdateSize {
		return "", fmt.Errorf("the update file is unexpectedly large")
	}
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("could not create a temporary update file: %w", err)
	}
	name := f.Name()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(response.Body, maxUpdateSize+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || written > maxUpdateSize {
		_ = os.Remove(name)
		return "", fmt.Errorf("could not save the update file")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.SHA256) {
		_ = os.Remove(name)
		return "", fmt.Errorf("the downloaded update failed its SHA-256 check")
	}
	return name, nil
}

func launchPortableReplacement(download, target string, pid int, failureText string) error {
	psQuote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$target=%s
$download=%s
$pidToWait=%d
$failureText=%s
try { Wait-Process -Id $pidToWait -ErrorAction SilentlyContinue } catch {}
$staged=$target+'.dictshelf-new-'+$pidToWait
$backup=$target+'.dictshelf-old-'+$pidToWait
try {
  Copy-Item -LiteralPath $download -Destination $staged -Force
  Move-Item -LiteralPath $target -Destination $backup -Force
  try { Move-Item -LiteralPath $staged -Destination $target -Force }
  catch { Move-Item -LiteralPath $backup -Destination $target -Force; throw }
  Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
  Start-Process -FilePath $target -WorkingDirectory (Split-Path -Parent $target)
  Remove-Item -LiteralPath $download -Force -ErrorAction SilentlyContinue
} catch {
  if ((Test-Path -LiteralPath $backup) -and -not (Test-Path -LiteralPath $target)) {
    Move-Item -LiteralPath $backup -Destination $target -Force
  }
  if (Test-Path -LiteralPath $target) {
    Start-Process -FilePath $target -WorkingDirectory (Split-Path -Parent $target)
  }
  Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $download -Force -ErrorAction SilentlyContinue
  Add-Type -AssemblyName PresentationFramework
  [System.Windows.MessageBox]::Show($failureText, 'DictShelf') | Out-Null
}
`, psQuote(target), psQuote(download), pid, psQuote(failureText))
	units := utf16.Encode([]rune(script))
	bytes := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(bytes[i*2:], unit)
	}
	encoded := base64.StdEncoding.EncodeToString(bytes)
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encoded)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the portable update helper: %w", err)
	}
	return nil
}
