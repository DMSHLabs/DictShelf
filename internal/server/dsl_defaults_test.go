// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitDSLDefaults(t *testing.T, r *Registry) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.prefs.mu.RLock()
		n := len(r.prefs.dslPending)
		r.prefs.mu.RUnlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("new DSL preparation did not finish")
}

func TestDSLDefaultsNewDictionaries(t *testing.T) {
	for _, mode := range []string{"original", "gd", "both"} {
		t.Run(mode, func(t *testing.T) {
			isolatedDBDir(t)
			dir := t.TempDir()
			state := filepath.Join(t.TempDir(), StateFile)
			reg, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
			if err != nil {
				t.Fatal(err)
			}
			s := New(reg)
			defaults := dslDefaults{}
			if mode != "gd" {
				defaults.Original = dslIndexOptions{Index: true, Contains: true, FullText: true}
			}
			if mode != "original" {
				defaults.GD = dslIndexOptions{Index: true, Contains: true, FullText: true}
			}
			groupCall(t, s, "PUT", "/api/dsl-defaults", defaults, 200)
			path := filepath.Join(dir, "first.dsl")
			if err := os.WriteFile(path, []byte(sampleDSL), 0600); err != nil {
				t.Fatal(err)
			}
			if err := reg.Rescan(); err != nil {
				t.Fatal(err)
			}
			waitDSLDefaults(t, reg)
			if len(reg.all()) != 2 {
				t.Fatal("DSL pair missing")
			}
			for _, e := range reg.all() {
				selected := mode == "both" || mode == e.dslVariant
				if reg.dslAvailable(e) != selected || e.indexBlocked() == selected {
					t.Fatal("wrong variant availability", e.dslVariant)
				}
				_, prepared := preparedFor(e.Path)
				if prepared != selected {
					t.Fatal("wrong prepared variant", e.dslVariant)
				}
				if selected {
					f := s.currentFeatures(e)
					if !f.Contains || !f.FullText {
						t.Fatal("selected features missing", e.dslVariant, f)
					}
				}
			}
			// New defaults must not enable the old family's missing variant.
			groupCall(t, s, "PUT", "/api/dsl-defaults", dslDefaults{Original: dslIndexOptions{Index: true}, GD: dslIndexOptions{Index: true}}, 200)
			if err := reg.Rescan(); err != nil {
				t.Fatal(err)
			}
			waitDSLDefaults(t, reg)
			for _, e := range reg.all() {
				if reg.dslAvailable(e) != (mode == "both" || mode == e.dslVariant) {
					t.Fatal("existing family changed")
				}
			}
			loaded := LoadPrefs(state)
			if !loaded.newDSLDefaults().GD.Index || !loaded.dslKnown[cleanAbs(path)] {
				t.Fatal("defaults/discovery not persisted")
			}
		})
	}
}

func TestDSLDefaultsValidationAndExisting(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg)
	if got := reg.prefs.newDSLDefaults(); !got.Original.Index || got.GD.Index || got.Original.Contains || got.Original.FullText {
		t.Fatal("wrong UI default", got)
	}
	groupCall(t, s, "PUT", "/api/dsl-defaults", dslDefaults{}, 400)
	groupCall(t, s, "PUT", "/api/dsl-defaults", dslDefaults{Original: dslIndexOptions{Index: true}, GD: dslIndexOptions{FullText: true}}, 400)
	groupCall(t, s, "PUT", "/api/dsl-defaults", reg.prefs.newDSLDefaults(), 200)
	if err := reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitDSLDefaults(t, reg)
	for _, e := range reg.all() {
		if e.indexBlocked() {
			t.Fatal("existing dictionary changed")
		}
	}
}

func TestDSLFirstSetupParserSelection(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), StateFile)
	r, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	s := New(r)
	for _, mode := range []string{"original", "gd", "both"} {
		next := dslDefaults{Original: dslIndexOptions{Index: mode != "gd"}, GD: dslIndexOptions{Index: mode != "original"}}
		groupCall(t, s, "PUT", "/api/dsl-defaults", next, 200)
		if r.prefs.parserSelection() != mode || LoadPrefs(state).parserSelection() != mode {
			t.Fatal("first setup parser not synchronized", mode)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "first.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitDSLDefaults(t, r)
	if LoadPrefs(state).dslInitialSetup {
		t.Fatal("initial setup not completed")
	}
	groupCall(t, s, "PUT", "/api/dsl-mode", map[string]any{"global": true, "mode": "gd"}, 200)
	groupCall(t, s, "PUT", "/api/dsl-defaults", dslDefaults{Original: dslIndexOptions{Index: true}}, 200)
	if r.prefs.parserSelection() != "gd" || LoadPrefs(state).parserSelection() != "gd" {
		t.Fatal("later defaults changed parser")
	}
}
