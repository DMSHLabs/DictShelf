// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

// FirstRunMigration describes an optional host-provided migration offered on
// the setup page before the user chooses dictionary folders.
type FirstRunMigration struct {
	Available bool `json:"available"`
	Forked    bool `json:"forked"`
}
