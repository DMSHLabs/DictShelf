// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"os"
	"sync"
)

type lockMode uint8

const (
	lockShared lockMode = iota
	lockExclusive
)

type fileLock struct {
	file  *os.File
	local *sync.Mutex
}

func openFileLock(path string, mode lockMode, wait bool) (*fileLock, bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	ok, err := lockFile(f, mode, wait)
	if err != nil || !ok {
		_ = f.Close()
		return nil, ok, err
	}
	return &fileLock{file: f}, true, nil
}

func (l *fileLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		if l.local != nil {
			l.local.Unlock()
		}
		return err
	}
	if l.local != nil {
		l.local.Unlock()
	}
	return closeErr
}
