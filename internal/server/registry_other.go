// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package server

// Everywhere else a rename over an open file is legal, so there is nothing to
// hand back: the entry keeps serving its prepared view for the length of the
// rebuild, exactly as before. See registry_windows.go for why it does not.
func releasePrepared(e *entry, textDB string) {}

// releaseSuperseded is a no-op where a rename over, or a delete of, an open
// file is legal: a retired backend sits out its closeGrace for in-flight
// readers, as before.
// See registry_windows.go.
func releaseSuperseded(e *entry) {}
