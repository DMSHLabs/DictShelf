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
	// 5 is ERROR_ACCESS_DENIED, and Windows uses it for "in use" as well: an
	// unlink of a held file reports SHARING_VIOLATION, while moving a folder that
	// has a held file INSIDE it reports this one (measured, 2026-11-03). A real
	// permission problem pays the same second of retries and then reports
	// unchanged.
	errAccessDenied = syscall.Errno(5)
)

func removeFolder(path string) error {
	return retryWhileHeld(path, func() error { return os.RemoveAll(path) }, "remove")
}

// moveFolder renames a directory. Windows refuses to move a folder that has an
// open file inside it exactly as it refuses to unlink one, so the removal's
// rename needs the same bounded retry the delete gets: without it a scanner
// holding one file of the folder for a moment would fail a removal that used to
// succeed.
func moveFolder(from, to string) error {
	return retryWhileHeld(from, func() error { return os.Rename(from, to) }, "move")
}

// retryWhileHeld runs op, and while Windows says the path is held it waits and
// tries again - briefly, because a holder still there after about a second is a
// real one and its error is the one to report.
func retryWhileHeld(path string, op func() error, what string) error {
	var err error
	for attempt, wait := 0, 10*time.Millisecond; ; attempt, wait = attempt+1, wait+wait {
		if err = op(); err == nil {
			return nil
		}
		if attempt >= 7 || !errors.Is(err, errSharingViolation) && !errors.Is(err, errLockViolation) && !errors.Is(err, errAccessDenied) {
			return err
		}
		logx.V("%s: %s is in use, retrying in %v (%v)", what, path, wait, err)
		time.Sleep(wait)
	}
}
