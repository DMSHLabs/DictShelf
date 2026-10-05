// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"maps"
	"slices"
)

// clone detaches every mutable field from the record currently being served.
// Writers can edit this copy without exposing partial changes or failed saves.
func (f prefsFile) clone() prefsFile {
	if f.IndexDefaults != nil {
		v := *f.IndexDefaults
		f.IndexDefaults = &v
	}
	f.IndexMigrated = maps.Clone(f.IndexMigrated)
	f.DSLKnown = maps.Clone(f.DSLKnown)
	f.DSLPending = maps.Clone(f.DSLPending)
	f.DSLRemoved = maps.Clone(f.DSLRemoved)
	f.DSL = maps.Clone(f.DSL)
	if f.DSLDefaults != nil {
		v := *f.DSLDefaults
		f.DSLDefaults = &v
	}
	if f.UI != nil {
		v := *f.UI
		v.GroupsOff = slices.Clone(v.GroupsOff)
		f.UI = &v
	}
	f.Dicts = slices.Clone(f.Dicts)
	for i := range f.Dicts {
		f.Dicts[i].Groups = slices.Clone(f.Dicts[i].Groups)
	}
	f.Groups = slices.Clone(f.Groups)
	for i := range f.Groups {
		f.Groups[i].Order = slices.Clone(f.Groups[i].Order)
	}
	return f
}
