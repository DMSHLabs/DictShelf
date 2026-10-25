// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !(darwin || linux || windows)

package sysmem

func total() int64 { return 0 }

func available() (int64, bool) { return 0, false }
