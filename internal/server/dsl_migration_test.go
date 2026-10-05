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
				f.DSLParser = "both"
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
			waitDSLDefaults(t, r)
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
				info := New(r).dictInfoFor(e)
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
			if f.DSLParser != "" || f.DSLDefaults != nil || len(f.DSL) != 0 {
				t.Fatal("selection not retired")
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
	if err := p.mutate(func(f *prefsFile) { f.DSLParser = "gd" }); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry(nil, true, WithPrefs(p))
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, r)
	if r.Count() != 1 {
		t.Fatal("cached receipt lost")
	}
	info := New(r).dictInfoFor(r.all()[0])
	if info.DSL.SourceAvailable || info.Unavailable || !info.Caps.FTS || !info.Caps.Contains {
		t.Fatal("cached receipt unavailable", info)
	}
	if err := os.WriteFile(source, []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Rescan(); err != nil {
		t.Fatal(err)
	}
	waitDSLDefaults(t, r)
	if r.Count() != 1 || r.all()[0].Path != source {
		t.Fatal("source return duplicated dictionary")
	}
	info = New(r).dictInfoFor(r.all()[0])
	if !info.Caps.FTS || !info.Caps.Contains || info.Unavailable {
		t.Fatal("source return lost optional indexes", info)
	}
}
