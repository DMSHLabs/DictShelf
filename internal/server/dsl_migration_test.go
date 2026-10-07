// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
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

func TestSingleDSLMigratesGroupsAndRemoval(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared", true: "removed"}[removed], func(t *testing.T) {
			isolatedDBDir(t)
			source, descriptor := legacyDSL(t)
			state := filepath.Join(t.TempDir(), StateFile)
			p := LoadPrefs(state)
			if err := p.mutate(func(f *prefsFile) {
				f.DSLRemoved = map[string]bool{cleanAbs(source) + "\ngd": removed, cleanAbs(source) + "\noriginal": true}
				f.Dicts = []DictPref{{ID: pathID(source), Path: source, Groups: []string{"a"}, Off: true}, {ID: pathID(descriptor), Path: descriptor, Groups: []string{"b"}}}
				f.Groups = []DictionaryGroup{{ID: "a", Order: []string{pathID(descriptor), pathID(source)}}}
			}); err != nil {
				t.Fatal(err)
			}
			r, err := NewRegistry([]string{filepath.Dir(source)}, true, WithPrefs(p))
			if err != nil {
				t.Fatal(err)
			}
			closeBackends(t, r)
			waitIndexPreparation(t, r)
			if r.Count() != 1 {
				t.Fatal("legacy variant still listed")
			}
			if folder, cache := r.Counts(); folder != 1 || cache != 0 {
				t.Fatal("wrong migrated source counts", folder, cache)
			}
			e := r.all()[0]
			if e.Path != source || e.indexBlocked() != removed {
				t.Fatal("wrong source/removal state", e.Path, e.indexBlocked())
			}
			if !removed {
				srv := New(r)
				global := srv.rowsGlobal(srv.groupsNow())
				info, _ := srv.dictInfoFor(e, global, rowKey(e, global))
				if !info.Caps.Contains || !info.Caps.FTS {
					t.Fatal("lost optional indexes")
				}
			}
			f, _ := p.data()
			if len(f.Dicts) != 1 || !reflect.DeepEqual(f.Dicts[0].Groups, []string{"a", "b"}) || f.Dicts[0].Off {
				t.Fatal("lost preferences", f.Dicts)
			}
			if !reflect.DeepEqual(f.Groups[0].Order, []string{pathID(source)}) {
				t.Fatal("lost group order", f.Groups)
			}
			if !removed {
				if err := e.setIndexRemoved(true); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.Rescan(); err != nil {
				t.Fatal(err)
			}
			if !r.all()[0].indexBlocked() {
				t.Fatal("legacy cache resurrected removed native index")
			}
			if _, err := os.Stat(descriptor); err != nil {
				t.Fatal("migration removed legacy data", err)
			}
		})
	}
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
	waitIndexPreparation(t, r)
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
			if f.DSLRemoved[key] || len(f.Dicts) != 1 || f.Dicts[0].Off {
				t.Fatalf("old parser choice still hides current index: %+v", f)
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
			e := &entry{Path: source, dslSource: cleanAbs(source), reg: r}
			if err := e.setIndexRemoved(true); err != nil {
				t.Fatal(err)
			}
			if err := r.migrateDSLPreferences(map[string]string{cleanAbs(source): source}); err != nil {
				t.Fatal(err)
			}
			f, _ = p.data()
			if !f.DSLRemoved[key] {
				t.Fatal("later explicit removal was undone")
			}
		})
	}
}

func TestSingleDSLMigrationKeepsRemovalWithoutPreparedIndex(t *testing.T) {
	isolatedDBDir(t)
	source := filepath.Join(t.TempDir(), "old.dsl")
	if err := os.WriteFile(source, []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	p := LoadPrefs(filepath.Join(t.TempDir(), StateFile))
	key := cleanAbs(source) + "\noriginal"
	if err := p.mutate(func(f *prefsFile) { f.DSLRemoved = map[string]bool{key: true} }); err != nil {
		t.Fatal(err)
	}
	r := &Registry{prefs: p}
	if err := r.migrateDSLPreferences(map[string]string{cleanAbs(source): source}); err != nil {
		t.Fatal(err)
	}
	f, _ := p.data()
	if !f.DSLRemoved[key] {
		t.Fatal("explicit removal without a prepared index was undone")
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
	if f, _ := p.data(); f.Version != prefsVersion || f.IndexDefaults == nil || !f.IndexDefaults.Index {
		t.Fatalf("migrated state = %+v, want version %d with unified base index", f, prefsVersion)
	}
	if defaults := p.newIndexDefaults(); !defaults.Index || defaults.Contains || defaults.FullText {
		t.Fatalf("old variant defaults affected current indexes: %+v", defaults)
	}
}
