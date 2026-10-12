// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/wuweidict/wudict/internal/format/wmd"
	"github.com/wuweidict/wudict/internal/store"
)

func waitNewDictionaryIndexing(t *testing.T, r *Registry) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := r.prefs.data()
		if len(rec.DSLPending) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("new dictionary indexing did not finish")
}

func TestNewDictionariesGetQueuedIndexWithOptionalDefaults(t *testing.T) {
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
		"new.zim":       "ZIM's native index needs no prepared database",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, reg)
	seen := 0
	for _, e := range reg.all() {
		if entryHasSelfIndex(e) {
			if _, ok := store.LookupDir(e.Path); ok || e.indexBlocked() {
				t.Fatalf("ZIM entered index preparation: %s", e.Path)
			}
			continue
		}
		if got := s.currentFeatures(e); !got.Contains || !got.FullText || e.indexBlocked() {
			t.Fatalf("%s: %+v", e.Path, got)
		}
		seen++
	}
	if seen != 2 {
		t.Fatalf("expected DSL and Markdown, got %d", seen)
	}
	for _, e := range reg.all() {
		if !entryHasSelfIndex(e) {
			continue
		}
		p := reg.prefs
		p.editMu.Lock()
		err := p.mutate(func(f *prefsFile) {
			if f.DSLPending == nil {
				f.DSLPending = map[string]indexOptions{}
			}
			if f.DSLRemoved == nil {
				f.DSLRemoved = map[string]bool{}
			}
			f.DSLPending[e.indexRemovalKey()] = indexOptions{Index: true}
			f.DSLRemoved[e.indexRemovalKey()] = true
		})
		p.editMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if err := reg.Rescan(); err != nil {
			t.Fatal(err)
		}
		rec, _ := p.data()
		if _, queued := rec.DSLPending[e.indexRemovalKey()]; queued || e.indexBlocked() {
			t.Fatal("saved ZIM preparation was not discarded")
		}
		if _, ok := store.LookupDir(e.Path); ok {
			t.Fatal("saved ZIM preparation created a database")
		}
	}
	groupCall(t, s, "PUT", "/api/index-defaults", indexOptions{Index: true}, 200)
	if err := reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, reg)
	for _, e := range reg.all() {
		if entryHasSelfIndex(e) {
			continue
		}
		if got := s.currentFeatures(e); !got.Contains || !got.FullText {
			t.Fatal("existing dictionary changed", e.Path, got)
		}
	}
}
