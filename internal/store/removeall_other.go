// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package store

import "os"

// removeFolder deletes a prepared folder. Off Windows an open file can be
// unlinked (and moved), so a holder is never in the way and nothing is retried:
// the two names exist so the removal reads the same on every platform.
func removeFolder(path string) error { return os.RemoveAll(path) }

func moveFolder(from, to string) error { return os.Rename(from, to) }
