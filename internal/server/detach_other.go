// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package server

import "os/exec"

// detach has nothing to do on Windows: rundll32 and explorer are GUI programs
// and attach to no console, so a console Ctrl-C never reaches them.
func detach(*exec.Cmd) {}
