// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "path/filepath"

// UserDir is the folder beside the wudict.toml in effect: everything the user
// owns besides that file lives in it, at names derived here and nowhere else.
// One value, so a portable install (D32) and --config move all of it at once.
// Empty is "no such folder": each path is then empty, and what lives there
// cannot be saved.
type UserDir string

// State is state.json: which dictionaries are searched, in what order.
func (d UserDir) State() string { return d.join(StateFile) }

// Style is the stylesheets' folder (style.go), and under it the user's files.
func (d UserDir) Style() string { return d.join(StyleDirName) }

// Groups is groups.ini (groups.go).
func (d UserDir) Groups() string { return d.join(GroupsFileName) }

// Builtin is where the wudict howto is written (internal/howto).
func (d UserDir) Builtin() string { return d.join("builtin") }

func (d UserDir) join(name string) string {
	if d == "" {
		return ""
	}
	return filepath.Join(string(d), name)
}
