// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os/exec"
	"runtime"

	"github.com/wuweidict/wudict/internal/logx"
)

// OpenURL hands u to the platform's default browser. Best effort: a machine
// with no opener (headless Linux, Android) logs it under -v, and the startup
// banner carries the link either way.
func OpenURL(u string) {
	// Refused, not passed on: FileProtocolHandler hands an empty target to
	// ShellExecute, which opens an Explorer window instead of failing.
	if u == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	// The browser is usually already running, and it is that process, not
	// any child of ours, that raises the window; see allowForeground.
	allowForeground()
	if err := startDetached(cmd); err != nil {
		logx.V("open browser: %v", err)
	}
}

// startDetached starts a desktop opener and reaps it in the background.
//
// Its own process group (detach): xdg-open hands the target to whatever
// launches it, and an application started fresh that way stays in OUR group -
// so a Ctrl-C to a terminal wudict, or the terminal closing, would take the
// user's browser or file manager down with the server.
//
// Reaped: open(1), xdg-open and explorer exit at once, and an unwaited child
// stays a zombie for the life of the server. Their exit status is ignored -
// explorer reports failure even when it succeeds.
func startDetached(cmd *exec.Cmd) error {
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
