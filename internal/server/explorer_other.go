// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package server

import "os/exec"

// explorerSelect is only reached on Windows (reveal switches on GOOS); see
// explorer_windows.go.
func explorerSelect(path string) *exec.Cmd {
	return exec.Command("explorer", "/select,"+path)
}
