// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wuweidict/wudict/internal/config"
)

func TestPortableConfigAllowsExternalDictionaries(t *testing.T) {
	data := filepath.Join(t.TempDir(), "Data")
	internal := filepath.Join(data, "Dictionaries")
	if err := os.MkdirAll(internal, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, config.Name)
	external := filepath.Join(filepath.Dir(data), "External")
	if err := os.WriteFile(path, []byte("DICT_DIR = "+config.FormatList([]string{internal, external})+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DICT_DIR", "WUDICT_DB_DIR", "DB_DIR", "LEMMA_DIR"} {
		t.Setenv(key, "")
	}
	t.Setenv("WUDICT_HOME", data)
	t.Setenv(windowsPortableHomeEnv, data)
	if err := normalizeWindowsPortableConfig(path); err != nil {
		t.Fatal(err)
	}
	if err := validateWindowsPortableConfig(data, path); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DictDirs) != 2 || !sameWindowsPath(cfg.DictDirs[0], internal) || cfg.DictDirs[1] != external {
		t.Fatalf("dictionary paths changed: %v", cfg.DictDirs)
	}
}
