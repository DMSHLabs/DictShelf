// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/store"
)

// Old comparison receipts remain readable when the source is unavailable.
// When it is available, only the native source is listed and rebuilt with our
// reader. Migration changes preferences, never source files or prepared data.
func (r *Registry) singleDSLPaths(paths []string) ([]string, error) {
	r.mu.RLock()
	roots := slices.Clone(r.dictDirs)
	r.mu.RUnlock()
	selected := map[string]string{}
	out := []string{}
	for _, path := range paths {
		if store.IsTextDB(path) {
			explicit := false
			for _, root := range roots {
				rel, err := filepath.Rel(root, path)
				if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					explicit = true
					break
				}
			}
			if explicit {
				out = append(out, path)
				continue
			}
		}
		source, variant := dslIdentity(path)
		if source == "" {
			out = append(out, path)
			continue
		}
		if fsx.FileExists(source) {
			path = source
			variant = "original"
		}
		if old, ok := selected[source]; ok {
			_, oldVariant := dslIdentity(old)
			if variant == "gd" && oldVariant != "gd" {
				selected[source] = path
			}
		} else {
			selected[source] = path
		}
	}
	sources := make([]string, 0, len(selected))
	for source := range selected {
		sources = append(sources, source)
	}
	slices.Sort(sources)
	for _, source := range sources {
		out = append(out, selected[source])
	}
	if err := r.migrateDSLPreferences(selected); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Registry) migrateDSLPreferences(selected map[string]string) error {
	p := r.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	f, _ := p.data()
	f = f.clone()
	changed := false
	enhanced := map[string]bool{}
	plans := map[string]store.Plan{}
	for _, pref := range f.Dicts {
		if source, variant := dslIdentity(pref.Path); variant == "gd" {
			enhanced[source] = true
		}
	}
	folders, err := store.Folders()
	if err != nil {
		return err
	}
	for _, folder := range folders {
		if source, variant := dslIdentity(folder.Source); source != "" {
			if variant == "gd" {
				enhanced[source] = true
			}
			plan := store.KeptPlan(store.TextDBPath(folder.Dir))
			old := plans[source]
			plans[source] = store.Plan{Contains: old.Contains || plan.Contains, FullText: old.FullText || plan.FullText}
		}
	}
	remap := map[string]string{}
	preferredOff := map[string]bool{}
	for _, pref := range f.Dicts {
		source, variant := dslIdentity(pref.Path)
		mode := f.DSLParser
		if mode == "" {
			mode = f.DSL[source]
		}
		if variant == mode && (mode == "gd" || mode == "original") {
			if path, ok := selected[source]; ok {
				preferredOff[pathID(path)] = pref.Off
			}
		}
	}
	merged := []DictPref{}
	positions := map[string]int{}
	for _, pref := range f.Dicts {
		source, variant := dslIdentity(pref.Path)
		if path, ok := selected[source]; ok {
			id := pathID(path)
			remap[pref.ID] = id
			if pref.ID != id || pref.Path != path {
				changed = true
			}
			pref.ID, pref.Path = id, path
			if variant == "gd" {
				name := strings.TrimSuffix(pref.Name, " GD")
				changed = changed || name != pref.Name
				pref.Name = name
			}
		} else {
			merged = append(merged, pref)
			continue
		}
		if at, ok := positions[pref.ID]; ok {
			changed = true
			for _, group := range pref.Groups {
				if !slices.Contains(merged[at].Groups, group) {
					merged[at].Groups = append(merged[at].Groups, group)
				}
			}
			merged[at].Off = merged[at].Off && pref.Off
		} else {
			positions[pref.ID] = len(merged)
			merged = append(merged, pref)
		}
	}
	for i := range merged {
		if off, ok := preferredOff[merged[i].ID]; ok {
			merged[i].Off = off
		}
	}
	f.Dicts = merged
	for i := range f.Groups {
		order := []string{}
		for _, id := range f.Groups[i].Order {
			if next, ok := remap[id]; ok {
				id = next
			}
			if !slices.Contains(order, id) {
				order = append(order, id)
			}
		}
		if !slices.Equal(order, f.Groups[i].Order) {
			changed = true
			f.Groups[i].Order = order
		}
	}
	for source, path := range selected {
		key := source + "\noriginal"
		firstMigration := !f.IndexMigrated[source]
		// Prefer the enhanced variant's explicit removal decision if it existed.
		if f.DSLRemoved[source+"\ngd"] || firstMigration && enhanced[source] {
			if f.DSLRemoved == nil {
				f.DSLRemoved = map[string]bool{}
			}
			removed := f.DSLRemoved[source+"\ngd"]
			if f.DSLRemoved[key] != removed {
				changed = true
			}
			if removed {
				f.DSLRemoved[key] = true
			} else {
				delete(f.DSLRemoved, key)
			}
			delete(f.DSLRemoved, source+"\ngd")
			changed = true
		}
		needsNative := false
		if enhanced[source] && path == source {
			db, ready := store.PreparedFor(source)
			needsNative = !ready || len(store.Inspect(db).TextStale(source)) != 0
		}
		if plan, ok := plans[source]; ok && enhanced[source] && (firstMigration || needsNative) && path == source && !f.DSLRemoved[key] {
			if f.DSLPending == nil {
				f.DSLPending = map[string]dslIndexOptions{}
			}
			if f.DSLRemoved == nil {
				f.DSLRemoved = map[string]bool{}
			}
			f.DSLPending[key] = dslIndexOptions{Index: true, Contains: plan.Contains, FullText: plan.FullText}
			f.DSLRemoved[key] = true
			changed = true
		}
		if plan, ok := f.DSLPending[source+"\ngd"]; ok && key != source+"\ngd" {
			f.DSLPending[key] = plan
			delete(f.DSLPending, source+"\ngd")
			changed = true
		}
		if firstMigration && enhanced[source] {
			if f.IndexMigrated == nil {
				f.IndexMigrated = map[string]bool{}
			}
			f.IndexMigrated[source] = true
			changed = true
		}
	}
	if f.DSLParser != "" || f.DSLDefaults != nil || len(f.DSL) > 0 || f.DSLInitialSetup {
		if f.IndexDefaults == nil {
			v := p.newIndexDefaults()
			f.IndexDefaults = &v
		}
		f.DSLParser = ""
		f.DSLDefaults = nil
		f.DSL = nil
		f.DSLInitialSetup = false
		changed = true
	}
	if !changed {
		return nil
	}
	return p.store(f)
}
