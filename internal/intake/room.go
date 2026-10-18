// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/fsx"
)

// diskFree is fsx.FreeSpace; a variable for the tests.
var diskFree = fsx.FreeSpace

// roomFor refuses work that needs more free space in dir than there is,
// before any of it is written: a download or an extraction that fills the
// disk part way fails after minutes instead of at once, and leaves a .part or
// a stage behind. A folder whose free space cannot be read is not refused -
// the write then reports a full disk itself.
func roomFor(dir, what string, need int64) error {
	if need <= 0 {
		return nil
	}
	if free, ok := diskFree(dir); ok && need > free {
		return &dict.NoRoomError{What: what, Dir: dir, Need: need, Free: free}
	}
	return nil
}
