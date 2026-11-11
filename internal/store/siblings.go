// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// A library copied beside itself - db and then db1, db2, the names a file
// manager gives a second copy - is dead weight: the application works in the
// configured folder only, so the sibling is never read, never cleaned and, in a
// library of a few gigabytes, is the largest thing on the disk that nothing
// uses.
//
// CleanSiblingLibraries removes them, and recognises one by its CONTENTS: at
// least one folder holding a text.db. A folder of anything else - a backup of
// sources, a data folder of another kind - is never touched, and the active
// library is never a candidate. Called once at startup, before anything opens
// a library file.
func CleanSiblingLibraries() (removed []string, freed int64) {
	root := DefaultDBDir()
	base := filepath.Base(root)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return nil, 0
	}
	candidate := regexp.MustCompile("^" + regexp.QuoteMeta(base) + "[0-9]+$")
	parent := filepath.Dir(root)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, 0
	}
	var found []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !candidate.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(parent, entry.Name())
		if fsx.SamePath(dir, root) || !holdsPreparedDictionary(dir) {
			continue
		}
		found = append(found, dir)
	}
	sort.Strings(found)
	for _, dir := range found {
		size := TreeSize(dir)
		if err := os.RemoveAll(dir); err != nil {
			continue
		}
		removed = append(removed, filepath.Base(dir))
		freed += size
	}
	return removed, freed
}

// holdsPreparedDictionary reports that dir looks like a library: at least one
// folder in it holds a text.db. The evidence is the CONTENT, not the name - a
// sibling that never held prepared dictionaries is somebody else's folder.
func holdsPreparedDictionary(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(TextDBPath(filepath.Join(dir, entry.Name()))); err == nil {
			return true
		}
	}
	return false
}
