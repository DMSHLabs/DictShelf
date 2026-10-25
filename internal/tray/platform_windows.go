// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tray

import (
	"debug/pe"
	"os"
	"syscall"
	"unsafe"
)

// The desktop artifact is linked with the GUI PE subsystem, so Windows does
// not allocate a console before Go starts. A console build of the same
// executable remains available for commands that need normal terminal I/O.
//
// GetConsoleProcessList reports how many processes are attached to this
// process's console. A terminal-launched CLI has its shell attached too (>=2);
// a desktop process has no console (0), and a console process started by a
// desktop shell owns its console alone (1).

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	user32                    = syscall.NewLazyDLL("user32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procProcessIdToSessionId  = kernel32.NewProc("ProcessIdToSessionId")
	procGetCurrentProcessId   = kernel32.NewProc("GetCurrentProcessId")
	procMessageBoxW           = user32.NewProc("MessageBoxW")
	procShowWindow            = user32.NewProc("ShowWindow")
	consoleWindowHidden       bool
)

func init() {
	// Explorer starts this console-subsystem executable with a console of its
	// own. Hide it before CLI startup does configuration and server work; the
	// main WebView2 window is created and shown independently.
	if GUILaunched() {
		if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
			const swHide = 0
			procShowWindow.Call(hwnd, swHide)
			consoleWindowHidden = true
		}
	}
}

// preflight refuses only where there is no desktop to put an icon on.
// Shell_NotifyIconW is part of the OS on every supported Windows, so there is
// nothing to probe for - the notification area exists even when the user has
// collapsed it behind the chevron. Session 0 is the exception: it has been
// isolated from every interactive desktop since Vista, so a service or a "run
// whether the user is logged on or not" scheduled task would otherwise pump a
// message loop forever for an icon that cannot be drawn.
func preflight(Config) string {
	if sessionZero() {
		return "session 0 has no interactive desktop"
	}
	return ""
}

func defaultRussian() bool {
	lang, _, _ := kernel32.NewProc("GetUserDefaultUILanguage").Call()
	return uint16(lang)&0x03ff == 0x0019
}

func sessionZero() bool {
	pid, _, _ := procGetCurrentProcessId.Call()
	var session uint32
	ok, _, _ := procProcessIdToSessionId.Call(pid, uintptr(unsafe.Pointer(&session)))
	return ok != 0 && session == 0
}

// GUILaunched reports a desktop-style launch: no console, or one owned by
// this process. A console shared with a shell belongs to the user.
func GUILaunched() bool {
	var pids [8]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1 || n == 0 && !sessionZero() && guiExecutable()
}

func guiExecutable() bool {
	path, err := os.Executable()
	if err != nil {
		return false
	}
	f, err := pe.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	const windowsGUI = 2
	switch header := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return header.Subsystem == windowsGUI
	case *pe.OptionalHeader32:
		return header.Subsystem == windowsGUI
	default:
		return false
	}
}

// DetachConsole closes the console Windows created for this process alone, so
// a double-click does not leave a black window on the desktop for as long as
// the server runs. It is a no-op unless this process owns its console: a
// console shared with the shell that launched us is the user's, not ours.
//
// After this, writes to stderr fail silently - which is why the caller moves
// logx to a file FIRST and only detaches if that succeeded (D76).
func DetachConsole() {
	if !GUILaunched() {
		return
	}
	_, _, _ = procFreeConsole.Call()
}

// ShowConsole restores the hidden startup console if headless logging could
// not be opened. A terminal-owned console is never hidden or shown here.
func ShowConsole() {
	if consoleWindowHidden {
		if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
			const swShow = 5
			procShowWindow.Call(hwnd, swShow)
		}
		consoleWindowHidden = false
	}
}

// Alert shows a modal message box. It is the last channel a GUI launch has:
// the console was closed by DetachConsole, stderr goes nowhere, and the log
// file is not something anyone is watching. Modal, so callers who must keep
// running start it detached.
func Alert(title, body string) {
	// A modal box on the session 0 desktop is one nobody can see and nobody
	// can dismiss, and MessageBoxW does not return until it is - so a service
	// or a "run whether the user is logged on or not" task would hang here
	// forever instead of exiting with its error. Say nothing rather than that.
	if sessionZero() {
		return
	}
	text, err := syscall.UTF16PtrFromString(body)
	if err != nil {
		return
	}
	caption, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	const mbIconWarning, mbSetForeground, mbTopMost = 0x30, 0x10000, 0x40000
	_, _, _ = procMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)),
		uintptr(mbIconWarning|mbSetForeground|mbTopMost))
}

// stopHint completes "To stop it, ..." for this platform.
const stopHint = "end DictShelf.exe in Task Manager"

// killCmd is the command that stops process %d from a terminal here. /F is not
// optional: without it taskkill only posts WM_CLOSE, and a wudict whose tray
// failed has no window to receive it ("can only be terminated forcefully").
const killCmd = "taskkill /F /PID %d"
