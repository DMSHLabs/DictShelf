// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

type browseMemorySample struct {
	serverWorking     uint64
	serverPrivate     uint64
	webviewCount      int
	webviewWorking    uint64
	webviewPrivate    uint64
	appWebviewCount   int
	appWebviewWorking uint64
	appWebviewPrivate uint64
	err               string
}
