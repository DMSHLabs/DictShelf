//go:build windows

// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package logx

import (
	"golang.org/x/sys/windows"
	"os"
)

func lockSystemFile(f *os.File) (func(), error) {
	var overlap windows.Overlapped
	handle := windows.Handle(f.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlap); err != nil {
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(handle, 0, 1, 0, &overlap) }, nil
}
