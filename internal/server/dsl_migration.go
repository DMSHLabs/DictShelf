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
	f, exists := p.data()
	f = f.clone()
	// Rewriting a v1 record through the current schema drops ignored parser
	// selection fields without interpreting their old values.
	changed := exists && f.Version < prefsVersion
	if changed && f.IndexDefaults == nil {
		// The single index is mandatory for new dictionaries. Old split
		// Original/GD defaults do not choose contains or full-text for it.
		defaults := indexOptions{Index: true}
		f.IndexDefaults = &defaults
	}
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
	merged := []DictPref{}
	positions := map[string]int{}
	for _, pref := range f.Dicts {
		source, variant := dslIdentity(pref.Path)
		if path, ok := selected[source]; ok {
			id := pathID(path)
			remap[pref.ID] = id
			if pref.ID != id || pref.Path != path || pref.Off {
				changed = true
			}
			// The old parser selection and its Off flag cannot hide the only
			// remaining DSL entry; the current UI has no Off control either.
			pref.ID, pref.Path, pref.Off = id, path, false
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
			if pref.Pinned != nil && *pref.Pinned {
				pinned := true
				merged[at].Pinned = &pinned
			}
			for _, group := range pref.Groups {
				if !slices.Contains(merged[at].Groups, group) {
					merged[at].Groups = append(merged[at].Groups, group)
				}
			}
		} else {
			positions[pref.ID] = len(merged)
			merged = append(merged, pref)
		}
	}
	f.Dicts = merged
	for i := range f.Groups {
		order := []groupOrder{}
		positions := make(map[string]int)
		for _, item := range f.Groups[i].Order {
			if next, ok := remap[item.ID]; ok {
				item.ID = next
			}
			if at, ok := positions[item.ID]; ok {
				order[at].Pinned = order[at].Pinned || item.Pinned
			} else {
				positions[item.ID] = len(order)
				order = append(order, item)
			}
		}
		if !slices.Equal(order, f.Groups[i].Order) {
			changed = true
			f.Groups[i].Order = order
		}
	}
	for source, path := range selected {
		key := source + "\noriginal"
		gdKey := source + "\ngd"
		firstMigration := !f.IndexMigrated[source]
		gdRemoved := f.DSLRemoved[gdKey]
		// Prefer the enhanced variant's explicit removal decision if it existed.
		if gdRemoved || firstMigration && enhanced[source] {
			if f.DSLRemoved == nil {
				f.DSLRemoved = map[string]bool{}
			}
			removed := gdRemoved
			if f.DSLRemoved[key] != removed {
				changed = true
			}
			if removed {
				f.DSLRemoved[key] = true
			} else {
				delete(f.DSLRemoved, key)
			}
			delete(f.DSLRemoved, gdKey)
			changed = true
		}
		// A successful explicit removal deletes its prepared folder. A current
		// native index still present behind a removal flag is the old variant's
		// stale state, so make that usable again. Pending work keeps its block.
		if firstMigration && f.DSLRemoved[key] && !gdRemoved && path == source {
			_, nativePending := f.DSLPending[key]
			_, gdPending := f.DSLPending[gdKey]
			if !nativePending && !gdPending {
				if prepared, ok := store.FindPrepared(source); ok && len(prepared.TextStale(source)) == 0 {
					delete(f.DSLRemoved, key)
					if f.IndexMigrated == nil {
						f.IndexMigrated = map[string]bool{}
					}
					f.IndexMigrated[source] = true
					changed = true
				}
			}
		}
		needsNative := false
		if enhanced[source] && path == source {
			db, ready := store.PreparedFor(source)
			needsNative = !ready || len(store.Inspect(db).TextStale(source)) != 0
		}
		if plan, ok := plans[source]; ok && enhanced[source] && (firstMigration || needsNative) && path == source && !f.DSLRemoved[key] {
			if f.DSLPending == nil {
				f.DSLPending = map[string]indexOptions{}
			}
			if f.DSLRemoved == nil {
				f.DSLRemoved = map[string]bool{}
			}
			f.DSLPending[key] = indexOptions{Index: true, Contains: plan.Contains, FullText: plan.FullText}
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
	if !changed {
		return nil
	}
	return p.store(f)
}
