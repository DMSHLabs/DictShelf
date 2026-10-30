// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package cli

func cleanupWindowsUpdaterHelperState() {}

func runWindowsUpdateHelper([]string) bool { return false }

func runWindowsTaskbarRefreshHelper([]string) bool { return false }
