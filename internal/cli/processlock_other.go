// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package cli

func acquireProcessLock() (func(), error) { return nil, nil }
