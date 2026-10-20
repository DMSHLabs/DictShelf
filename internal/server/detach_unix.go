// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package server

import (
	"os/exec"
	"syscall"
)

// detach puts cmd in a process group of its own, so the terminal's job-control
// signals (SIGINT on Ctrl-C, SIGHUP when it closes) reach wudict and not what
// it opened. Added to an existing SysProcAttr, never replacing one.
func detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}
