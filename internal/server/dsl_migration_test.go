// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

func legacyDSL(t *testing.T) (string, string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "old.dsl")
	if err := os.WriteFile(source, []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	descriptor := filepath.Join(t.TempDir(), "old.dslgd")
	data, _ := json.Marshal(map[string]string{"source": source})
	if err := os.WriteFile(descriptor, data, 0600); err != nil {
		t.Fatal(err)
	}
	d, err := dict.Open(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	yes := true
	if _, err := store.Reconcile(descriptor, store.Target{Contains: &yes, FullText: &yes}, store.Hooks{}); err != nil {
		t.Fatal(err)
	}
	return source, descriptor
}

func TestSingleDSLCachedReceiptThenSourceReturns(t *testing.T) {
	isolatedDBDir(t)
	source, _ := legacyDSL(t)
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	p := LoadPrefs(filepath.Join(t.TempDir(), StateFile))
	r, err := NewRegistry(nil, true, WithPrefs(p))
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, r)
	if r.Count() != 1 {
		t.Fatal("cached receipt lost")
	}
	srv := New(r)
	global := srv.rowsGlobal(srv.groupsNow())
	info, _ := srv.dictInfoFor(r.all()[0], global, rowKey(r.all()[0], global))
	if info.DSL.SourceAvailable || info.Unavailable || !info.Caps.FTS || !info.Caps.Contains {
		t.Fatal("cached receipt unavailable", info)
	}
	if err := os.WriteFile(source, []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, r)
	if r.Count() != 1 || r.all()[0].Path != source {
		t.Fatal("source return duplicated dictionary")
	}
	srv = New(r)
	global = srv.rowsGlobal(srv.groupsNow())
	info, _ = srv.dictInfoFor(r.all()[0], global, rowKey(r.all()[0], global))
	if !info.Caps.FTS || !info.Caps.Contains || info.Unavailable {
		t.Fatal("source return lost optional indexes", info)
	}
}

func TestSingleDSLMigrationPreservesSourceName(t *testing.T) {
	isolatedDBDir(t)
	source, descriptor := legacyDSL(t)
	for _, path := range []string{source, descriptor} {
		p := LoadPrefs(filepath.Join(t.TempDir(), StateFile))
		if err := p.mutate(func(f *prefsFile) {
			f.Dicts = []DictPref{{ID: pathID(path), Path: path, Name: "Dictionary GD"}}
		}); err != nil {
			t.Fatal(err)
		}
		r := &Registry{prefs: p}
		if err := r.migrateDSLPreferences(map[string]string{cleanAbs(source): source}); err != nil {
			t.Fatal(err)
		}
		want := "Dictionary GD"
		if path == descriptor {
			want = "Dictionary"
		}
		f, _ := p.data()
		if len(f.Dicts) != 1 || f.Dicts[0].Name != want {
			t.Fatalf("migration changed source name: %+v, want %q", f.Dicts, want)
		}
	}
}

func TestSingleDSLMigrationIgnoresOldParserChoice(t *testing.T) {
	for _, mode := range []string{"original", "gd", "both", ""} {
		t.Run(mode, func(t *testing.T) {
			isolatedDBDir(t)
			source := filepath.Join(t.TempDir(), "old.dsl")
			if err := os.WriteFile(source, []byte(sampleDSL), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Reconcile(source, store.Target{}, store.Hooks{}); err != nil {
				t.Fatal(err)
			}
			state := filepath.Join(t.TempDir(), StateFile)
			key := cleanAbs(source) + "\noriginal"
			legacy, err := json.Marshal(map[string]any{
				"dslParser":  mode,
				"dsl":        map[string]string{cleanAbs(source): mode},
				"dslRemoved": map[string]bool{key: true},
				"dicts":      []DictPref{{ID: pathID(source), Path: source, Off: true}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(state, legacy, 0600); err != nil {
				t.Fatal(err)
			}
			p := LoadPrefs(state)
			r := &Registry{prefs: p}
			if err := r.migrateDSLPreferences(map[string]string{cleanAbs(source): source}); err != nil {
				t.Fatal(err)
			}
			f, _ := p.data()
			if len(f.Dicts) != 1 || f.Dicts[0].Off {
				t.Fatalf("old parser choice still hides the dictionary: %+v", f)
			}
			saved, err := os.ReadFile(state)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(saved, &fields); err != nil {
				t.Fatal(err)
			}
			if _, kept := fields["dslParser"]; kept {
				t.Fatal("legacy parser choice was saved again")
			}
			if _, kept := fields["dsl"]; kept {
				t.Fatal("legacy per-source choice was saved again")
			}
			// A marker that says "no index right now" survives the parser-choice
			// migration untouched: it is not a decision the migration interprets,
			// and the queue acts on it (TestAMissingOrRememberedIndexIsPrepared).
		})
	}
}

func TestSingleDSLRetiresLegacySelectionWithoutDictionaries(t *testing.T) {
	isolatedDBDir(t)
	state := filepath.Join(t.TempDir(), StateFile)
	if err := os.WriteFile(state, []byte(`{"version":1,"dslParser":"gd","dsl":{"old.dsl":"original"},"dslInitialSetup":true,"dslDefaults":{"original":{"index":false,"contains":true},"gd":{"index":true,"fullText":true}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	p := LoadPrefs(state)
	r := &Registry{prefs: p}
	if err := r.migrateDSLPreferences(nil); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(saved, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"dslParser", "dsl", "dslInitialSetup", "dslDefaults"} {
		if _, kept := fields[key]; kept {
			t.Fatalf("legacy %s still saved", key)
		}
	}
	if f, _ := p.data(); f.Version != prefsVersion || f.IndexDefaults == nil {
		t.Fatalf("migrated state = %+v, want version %d with the index defaults recorded", f, prefsVersion)
	}
	if defaults := p.newIndexDefaults(); !defaults.Contains || defaults.FullText {
		t.Fatalf("old variant defaults affected current indexes: %+v", defaults)
	}
}
