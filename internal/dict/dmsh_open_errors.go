// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import (
	"errors"
	"fmt"

	"github.com/wuweidict/wudict/internal/logx"
)

// ErrUnreadable marks an open failure decided by a file's bytes and the
// current reader version. ErrTooLarge also covers a failed space check.
var (
	ErrUnreadable = errors.New("not readable")
	ErrTooLarge   = errors.New("too large to open")
)

// NoRoomError reports that a dictionary cannot be unpacked in the available
// disk space or memory. It remains distinguishable from a malformed file.
type NoRoomError struct {
	What       string
	Dir        string
	Need, Free int64
}

func (e *NoRoomError) Error() string {
	where := "memory"
	if e.Dir != "" {
		where = "free disk space in " + e.Dir
	}
	return fmt.Sprintf("not enough %s %s: more than %s needed, %s available",
		where, e.What, logx.Size(e.Need), logx.Size(e.Free))
}

func (e *NoRoomError) Is(target error) bool { return target == ErrTooLarge }
