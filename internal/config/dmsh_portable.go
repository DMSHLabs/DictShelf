// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"path/filepath"
	"strings"
)

// RelativizePortableDictionaries changes only absolute dictionary folders
// inside Data/Dictionaries. Other configured paths and their order survive.
func RelativizePortableDictionaries(configPath string) error {
	r, err := loadFile(configPath)
	if err != nil {
		return err
	}
	dirs, exists := r.lists["DICT_DIR"]
	if !exists {
		if value := r.vals["DICT_DIR"]; value != "" {
			dirs = []string{value}
		}
	}
	data, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return err
	}
	internal := filepath.Join(data, "Dictionaries")
	changed := false
	for i, dir := range dirs {
		if !filepath.IsAbs(dir) {
			continue
		}
		rel, e := filepath.Rel(internal, dir)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		dirs[i] = filepath.Join("Dictionaries", rel)
		changed = true
	}
	if !changed {
		return nil
	}
	return SaveKeyRaw(configPath, "DICT_DIR", FormatList(dirs))
}
