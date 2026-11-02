// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package store

import (
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// On Windows a file anything holds open cannot be unlinked at all: the deletion
// fails with ERROR_SHARING_VIOLATION ("The process cannot access the file
// because it is being used by another process") and, because os.RemoveAll walks
// the folder, leaves it half removed with an error for the user.
//
// Two kinds of holder are transient and mean nothing: a process outside this one
// (a scanner or the system indexer touching a file that was just written), and
// one of this process's own readers taking a moment with the stats cache. The
// long holder - a measurement or stats pass with the database open - is what the
// fence closes (entry.walk in internal/server): it lets go before a removal
// claims the files. Neither is a reason to fail a deletion the user asked for,
// so the rest is retried: briefly, because a holder still there after a second
// is a real one, and its error is the one to report.
const (
	errSharingViolation = syscall.Errno(32) // ERROR_SHARING_VIOLATION
	errLockViolation    = syscall.Errno(33) // ERROR_LOCK_VIOLATION
)

func removeFolder(path string) error {
	var err error
	for attempt, wait := 0, 10*time.Millisecond; ; attempt, wait = attempt+1, wait+wait {
		if err = os.RemoveAll(path); err == nil {
			return nil
		}
		if attempt >= 7 || !errors.Is(err, errSharingViolation) && !errors.Is(err, errLockViolation) {
			return err
		}
		logx.V("remove: %s is in use, retrying in %v (%v)", path, wait, err)
		time.Sleep(wait)
	}
}
