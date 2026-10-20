// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package logx

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	miniDumpWithDataSegs   = 0x00000001
	miniDumpWithHandleData = 0x00000004
	miniDumpWithThreadInfo = 0x00001000
)

type windowsExceptionRecord struct {
	Code       uint32
	Flags      uint32
	Nested     uintptr
	Address    uintptr
	ParamCount uint32
	Info       [15]uintptr
}

type windowsExceptionPointers struct {
	Record  *windowsExceptionRecord
	Context uintptr
}

type miniDumpExceptionInformation struct {
	ThreadID          uint32
	ExceptionPointers *windowsExceptionPointers
	ClientPointers    int32
}

var (
	nativeCrashLogPath  string
	nativeCrashDumpPath string
	nativeCrashFilter   uintptr
	previousCrashFilter uintptr

	kernel32Crash               = syscall.NewLazyDLL("kernel32.dll")
	setUnhandledExceptionFilter = kernel32Crash.NewProc("SetUnhandledExceptionFilter")
	getCurrentProcess           = kernel32Crash.NewProc("GetCurrentProcess")
	getCurrentProcessID         = kernel32Crash.NewProc("GetCurrentProcessId")
	getCurrentThreadID          = kernel32Crash.NewProc("GetCurrentThreadId")
	createFileWCrash            = kernel32Crash.NewProc("CreateFileW")
	writeFileCrash              = kernel32Crash.NewProc("WriteFile")
	closeHandleCrash            = kernel32Crash.NewProc("CloseHandle")
	dbghelpCrash                = windows.NewLazySystemDLL("dbghelp.dll")
	miniDumpWriteDump           = dbghelpCrash.NewProc("MiniDumpWriteDump")
)

func configureNativeCrashCapture(root, sessionPath string) error {
	name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(sessionPath), "system_"), ".log")
	nativeCrashLogPath = filepath.Join(root, "crash_"+name+"_windows.log")
	nativeCrashDumpPath = filepath.Join(root, "crash_"+name+"_windows.dmp")
	if err := dbghelpCrash.Load(); err != nil {
		return fmt.Errorf("load dbghelp.dll: %w", err)
	}
	nativeCrashFilter = syscall.NewCallback(windowsUnhandledException)
	previous, _, _ := setUnhandledExceptionFilter.Call(nativeCrashFilter)
	previousCrashFilter = previous
	System("Windows unhandled-exception capture enabled log=%q dump=%q previous_filter=%#x",
		nativeCrashLogPath, nativeCrashDumpPath, previous)
	return nil
}

func windowsUnhandledException(exceptionInfo uintptr) uintptr {
	var code uint32
	var address uintptr
	if exceptionInfo != 0 {
		pointers := (*windowsExceptionPointers)(unsafe.Pointer(exceptionInfo))
		if pointers.Record != nil {
			code = pointers.Record.Code
			address = pointers.Record.Address
		}
	}
	threadID, _, _ := getCurrentThreadID.Call()
	metadata := fmt.Sprintf("Windows unhandled exception\r\nprocess_id=%d\r\nthread_id=%d\r\nexception_code=0x%08X\r\nexception_address=0x%X\r\ndump_file=%s\r\n",
		currentProcessID(), threadID, code, address, nativeCrashDumpPath)
	writeWindowsCrashText(nativeCrashLogPath, []byte(metadata))
	dumpErr := writeWindowsMiniDump(exceptionInfo, uint32(threadID))
	if dumpErr != nil {
		writeWindowsCrashText(nativeCrashLogPath, []byte("minidump_error="+dumpErr.Error()+"\r\n"))
	} else {
		writeWindowsCrashText(nativeCrashLogPath, []byte("minidump_written=true\r\n"))
	}
	if previousCrashFilter != 0 {
		result, _, _ := syscall.SyscallN(previousCrashFilter, exceptionInfo)
		return result
	}
	return 0 // EXCEPTION_CONTINUE_SEARCH leaves Windows Error Reporting enabled.
}

func currentProcessID() uintptr {
	id, _, _ := getCurrentProcessID.Call()
	return id
}

func writeWindowsCrashText(path string, content []byte) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	handle, _, _ := createFileWCrash.Call(uintptr(unsafe.Pointer(name)), 0x00000004, 1, 0, 4, 0x80, 0)
	if handle == ^uintptr(0) {
		return
	}
	defer closeHandleCrash.Call(handle)
	var written uint32
	if len(content) == 0 {
		return
	}
	writeFileCrash.Call(handle, uintptr(unsafe.Pointer(&content[0])), uintptr(len(content)), uintptr(unsafe.Pointer(&written)), 0)
}

func writeWindowsMiniDump(exceptionInfo uintptr, threadID uint32) error {
	name, err := syscall.UTF16PtrFromString(nativeCrashDumpPath)
	if err != nil {
		return err
	}
	handle, _, callErr := createFileWCrash.Call(uintptr(unsafe.Pointer(name)), 0x40000000, 1, 0, 2, 0x80, 0)
	if handle == ^uintptr(0) {
		if callErr == syscall.Errno(0) {
			return fmt.Errorf("CreateFileW failed")
		}
		return fmt.Errorf("CreateFileW: %w", callErr)
	}
	defer closeHandleCrash.Call(handle)
	process, _, _ := getCurrentProcess.Call()
	var exceptionInfoArg uintptr
	var exception miniDumpExceptionInformation
	if exceptionInfo != 0 {
		exception = miniDumpExceptionInformation{
			ThreadID:          threadID,
			ExceptionPointers: (*windowsExceptionPointers)(unsafe.Pointer(exceptionInfo)),
		}
		exceptionInfoArg = uintptr(unsafe.Pointer(&exception))
	}
	flags := uintptr(miniDumpWithDataSegs | miniDumpWithHandleData | miniDumpWithThreadInfo)
	ok, _, callErr := miniDumpWriteDump.Call(process, currentProcessID(), handle, flags,
		exceptionInfoArg, 0, 0)
	if ok == 0 {
		if callErr == syscall.Errno(0) {
			return fmt.Errorf("MiniDumpWriteDump failed")
		}
		return fmt.Errorf("MiniDumpWriteDump: %w", callErr)
	}
	return nil
}
