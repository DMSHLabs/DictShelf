// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package server

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var processMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
var appWebviewPIDs struct {
	sync.Mutex
	refreshed time.Time
	pids      map[uint32]bool
}

// WebView2's browser process can be broker-launched rather than parented by
// wuDict2.exe. Its command line carries the owning executable name.
func appWebviewProcessIDs() map[uint32]bool {
	appWebviewPIDs.Lock()
	defer appWebviewPIDs.Unlock()
	if time.Since(appWebviewPIDs.refreshed) < 15*time.Second && appWebviewPIDs.pids != nil {
		return appWebviewPIDs.pids
	}
	appWebviewPIDs.refreshed = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" | Where-Object { $_.CommandLine -like '*--webview-exe-name=wuDict2.exe*' } | ForEach-Object { $_.ProcessId }`)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return appWebviewPIDs.pids
	}
	pids := make(map[uint32]bool)
	for _, line := range strings.Fields(string(out)) {
		pid, err := strconv.ParseUint(line, 10, 32)
		if err == nil {
			pids[uint32(pid)] = true
		}
	}
	appWebviewPIDs.pids = pids
	return pids
}

type processMemoryCounters struct {
	size, pageFaults                                                                                uint32
	peakWorking, working, peakPaged, paged, peakNonPaged, nonPaged, pagefile, peakPagefile, private uintptr
}

func memoryForProcess(pid uint32) (uint64, uint64, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, 0, err
	}
	defer windows.CloseHandle(h)
	var counters processMemoryCounters
	counters.size = uint32(unsafe.Sizeof(counters))
	ok, _, callErr := processMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&counters)), uintptr(counters.size))
	if ok == 0 {
		return 0, 0, callErr
	}
	return uint64(counters.working), uint64(counters.private), nil
}

func browseProcessMemory() browseMemorySample {
	var sample browseMemorySample
	root := uint32(windows.GetCurrentProcessId())
	ownerPIDs := appWebviewProcessIDs()
	var err error
	sample.serverWorking, sample.serverPrivate, err = memoryForProcess(root)
	if err != nil {
		sample.err = fmt.Sprintf("server: %v", err)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		sample.err = fmt.Sprintf("process snapshot: %v", err)
		return sample
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	parents := make(map[uint32]uint32)
	var webviews []uint32
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "msedgewebview2.exe") {
			webviews = append(webviews, entry.ProcessID)
		}
	}
	for _, pid := range webviews {
		working, private, err := memoryForProcess(pid)
		if err != nil {
			continue // A WebView2 child may exit between the snapshot and the read.
		}
		sample.webviewCount++
		sample.webviewWorking += working
		sample.webviewPrivate += private
		ancestor := pid
		for steps := 0; steps < 16 && ancestor != 0 && ancestor != root; steps++ {
			ancestor = parents[ancestor]
		}
		if ancestor == root || ownerPIDs[pid] {
			sample.appWebviewCount++
			sample.appWebviewWorking += working
			sample.appWebviewPrivate += private
		}
	}
	return sample
}
