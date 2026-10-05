// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/wuweidict/wudict/internal/format/wmd"
)

func waitIndexPreparation(t *testing.T, r *Registry) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := r.prefs.data()
		if len(rec.DSLPending) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("new DSL preparation did not finish")
}

func TestIndexDefaultsApplyToNewDSLAndMarkdownOnly(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), StateFile)
	reg, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg)
	// Index cannot be disabled, even by an older or malformed client.
	groupCall(t, s, "PUT", "/api/index-defaults", indexOptions{Contains: true, FullText: true}, 200)
	if got := LoadPrefs(state).newIndexDefaults(); !got.Index || !got.Contains || !got.FullText {
		t.Fatalf("defaults not persisted: %+v", got)
	}
	for name, body := range map[string]string{
		"new.dsl":       sampleDSL,
		"new.wudict.md": "# Markdown test\nwudict: 1\nfrom: en\nto: en\n\n## word\n\nA definition.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitIndexPreparation(t, reg)
	seen := 0
	for _, e := range reg.all() {
		if got := s.currentFeatures(e); !got.Contains || !got.FullText || e.indexBlocked() {
			t.Fatalf("%s: %+v", e.Path, got)
		}
		seen++
	}
	if seen != 2 {
		t.Fatalf("expected DSL and Markdown, got %d", seen)
	}
	groupCall(t, s, "PUT", "/api/index-defaults", indexOptions{Index: true}, 200)
	if err := reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitIndexPreparation(t, reg)
	for _, e := range reg.all() {
		if got := s.currentFeatures(e); !got.Contains || !got.FullText {
			t.Fatal("existing dictionary changed", e.Path, got)
		}
	}
}
