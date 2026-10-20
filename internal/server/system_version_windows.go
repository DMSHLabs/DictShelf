// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func systemVersion() string {
	version := windows.RtlGetVersion()
	name := "Windows"
	if version.ProductType == 1 {
		if version.BuildNumber >= 22000 {
			name = "Windows 11"
		} else {
			name = "Windows 10"
		}
	} else {
		name = "Windows Server"
	}
	return fmt.Sprintf("%s (%d.%d.%d)", name, version.MajorVersion, version.MinorVersion, version.BuildNumber)
}
