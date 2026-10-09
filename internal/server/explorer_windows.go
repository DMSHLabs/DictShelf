// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package server

import (
	"os/exec"
	"syscall"
)

// explorerSelect opens Explorer with path selected in its folder.
//
// The command line is written by hand. explorer.exe parses its own arguments,
// and exec.Command would quote "/select,C:\My Dicts\x" whole - Explorer does
// not recognise a quoted switch and opens its default folder instead, which a
// space in the path (most user names, most library folders) triggers. The path
// is quoted on its own, which also keeps a comma in it from reading as the
// next option. A Windows path cannot contain '"', so the quoting is complete.
func explorerSelect(path string) *exec.Cmd {
	c := exec.Command("explorer")
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: explorerCmdLine(path)}
	return c
}

func explorerCmdLine(path string) string {
	return `explorer /select,"` + path + `"`
}
