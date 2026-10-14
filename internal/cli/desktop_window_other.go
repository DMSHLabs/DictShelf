// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package cli

import "context"

func openDesktopWindow(string, func(), func(string, string) (any, error), func(context.Context, string, string, func([]byte)) error) bool {
	return false
}
