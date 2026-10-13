// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package server

func browseProcessMemory() browseMemorySample {
	return browseMemorySample{err: "process sampling is available on Windows"}
}
