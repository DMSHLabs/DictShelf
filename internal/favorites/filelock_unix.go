// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package favorites

import (
	"errors"
	"os"
	"syscall"
)

func lockFile(f *os.File, mode lockMode, wait bool) (bool, error) {
	flags := syscall.LOCK_SH
	if mode == lockExclusive {
		flags = syscall.LOCK_EX
	}
	if !wait {
		flags |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), flags); err != nil {
		if !wait && (errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
