// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package server

import "runtime"

func systemVersion() string { return runtime.GOOS }
