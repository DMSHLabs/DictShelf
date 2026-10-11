// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !(darwin || linux || windows)

package fsx

func freeSpace(string) (int64, bool) { return 0, false }
