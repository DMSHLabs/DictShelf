// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"

	"github.com/wuweidict/wudict/internal/config"
	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/intake"
	"github.com/wuweidict/wudict/internal/store"
)

// ConfigureLibrary selects the prepared database and protects it from being
// scanned as a source dictionary folder. It must precede NewRuntime.
func ConfigureLibrary(cfg config.Config) (string, error) {
	store.SetDBDir(cfg.DBDir)
	libDir := store.DefaultDBDir()
	for _, d := range cfg.DictDirs {
		if dict.SameDir(d, libDir) {
			return "", fmt.Errorf("DICT_DIR and DB_DIR are the same folder:\n"+
				"  %s\n"+
				"  DB_DIR contains internally cached dictionaries; DICT_DIR must point at\n"+
				"  the folder with your dictionary files (.mdx, .ifo, .slob, .dsl, .bgl, .zim).\n"+
				"  To only use cached dictionaries, leave DICT_DIR unset and set USE_CACHED = \"1\".", libDir)
		}
	}
	dict.ExcludeDir(libDir)
	return libDir, nil
}

// RecoverLibrary removes interrupted imports and adopts loose prepared
// databases before the registry discovers dictionaries.
func RecoverLibrary(cfg config.Config) ([]store.Adopted, error) {
	intake.Sweep(cfg.DictDirs)
	return store.AdoptLoose()
}
