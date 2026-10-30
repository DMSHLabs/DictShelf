// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/DMSHLabs/DictShelf/internal/format/wmd"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

func waitNewDictionaryIndexing(t *testing.T, r *Registry) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = r.prefs.data() // observe the persisted state after the worker's write
		queue, queueExists := r.jobs.status("initial-index-queue")
		if !queueExists || !queue.Running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("new dictionary indexing did not finish")
}

func TestNewDictionariesGetBaseIndexWithoutSavedDefaults(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), StateFile)
	reg, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, reg)
	closeBackends(t, reg)
	if len(reg.all()) != 1 {
		t.Fatalf("expected one discovered dictionary, got %d", len(reg.all()))
	}
	e := reg.all()[0]
	textDB, ok := store.PreparedFor(e.Path)
	if !ok {
		t.Fatal("new dictionary did not get its mandatory headword index")
	}
	index, err := store.Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	if !index.Caps().Exact || !index.Caps().Prefix || index.Caps().Contains || index.Caps().FTS {
		t.Fatalf("unexpected first-run index features: %+v", index.Caps())
	}
	if defaults := reg.prefs.newIndexDefaults(); !defaults.Index || defaults.Contains || defaults.FullText {
		t.Fatalf("unsaved optional defaults were not left disabled: %+v", defaults)
	}
}

func TestSearchDoesNotPrepareDictionaryWithoutIndex(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), StateFile)
	if err := os.WriteFile(filepath.Join(dir, "empty.dsl"), []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(nil, false, WithPrefs(LoadPrefs(state)))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.SetDirsDeferred([]string{dir}); err != nil {
		t.Fatal(err)
	}
	defer closeBackends(t, reg)
	if len(reg.all()) != 1 {
		t.Fatalf("expected one discovered dictionary, got %d", len(reg.all()))
	}
	e := reg.all()[0]
	s := New(reg)
	var events []SearchEvent
	if err := s.Search(context.Background(), SearchRequest{Query: "corazon", Dictionary: e.ID}, func(event SearchEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	foundUnavailable := false
	for _, event := range events {
		if event.T == "hit" && event.Skipped && len(event.Results) == 0 {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatalf("search should skip the missing index without querying the source: %+v", events)
	}
	if _, ok := store.LookupDir(e.Path); ok {
		t.Fatal("search prepared a dictionary whose index was missing")
	}
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
	waitNewDictionaryIndexing(t, reg)
	for _, e := range reg.all() {
		if got := s.currentFeatures(e); !got.Contains || !got.FullText {
			t.Fatal("existing dictionary changed", e.Path, got)
		}
	}
}
