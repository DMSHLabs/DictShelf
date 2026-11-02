// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package store

import "os"

// removeFolder deletes a prepared folder. Off Windows an open file can be
// unlinked, so a holder is never in the way and there is nothing to retry.
func removeFolder(path string) error { return os.RemoveAll(path) }
