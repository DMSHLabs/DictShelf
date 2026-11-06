// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/ftsq"
)

func TestPrefsFailedMutationKeepsActiveRecord(t *testing.T) {
	p := LoadPrefs("")
	original := prefsFile{
		IndexDefaults: &indexOptions{Contains: true},
		IndexMigrated: map[string]bool{"source": true},
		DSLKnown:      map[string]bool{"source": true}, DSLPending: map[string]indexOptions{},
		DSLRemoved: map[string]bool{"source": true},
		UI:         &UIPrefs{GroupsOff: []string{"lang"}},
		Groups:     []DictionaryGroup{{ID: "g", Name: "Reading", Order: []groupOrder{{ID: "d"}}}},
		Dicts:      []DictPref{{ID: "d", Groups: []string{"g"}}},
	}
	if err := p.store(original); err != nil {
		t.Fatal(err)
	}
	before, _ := p.data()
	bytes, _ := json.Marshal(before)
	var want prefsFile
	if err := json.Unmarshal(bytes, &want); err != nil {
		t.Fatal(err)
	}
	blocked := t.TempDir()
	calls := 0
	// Read the active in-memory record, then fail the atomic write over a directory.
	p.file.path = func() string {
		calls++
		if calls == 1 {
			return ""
		}
		return blocked
	}
	err := p.mutate(func(f *prefsFile) {
		f.IndexDefaults.Contains = false
		delete(f.IndexMigrated, "source")
		delete(f.DSLKnown, "source")
		delete(f.DSLPending, "source")
		delete(f.DSLRemoved, "source")
		f.UI.GroupsOff[0] = "pair"
		f.Groups[0].Name = "Changed"
		f.Groups[0].Order[0].ID = "other"
		f.Dicts[0].Groups[0] = "other"
	})
	p.file.path = func() string { return "" }
	if err == nil {
		t.Fatal("expected save failure")
	}
	got, _ := p.data()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("failed save changed active preferences: got %+v, want %+v", got, want)
	}
}

func TestPrefsFailedHealKeepsGroupOrder(t *testing.T) {
	s, _ := newPrefsServer(t)
	p := s.reg.prefs
	e := s.reg.all()[0]
	if err := p.store(prefsFile{Dicts: []DictPref{{ID: "old", Path: e.Path, Groups: []string{"g"}}}, Groups: []DictionaryGroup{{ID: "g", Order: []groupOrder{{ID: "old"}}}}}); err != nil {
		t.Fatal(err)
	}
	blocked := t.TempDir()
	activePath := p.file.path
	calls := 0
	p.file.path = func() string {
		calls++
		if calls <= 2 {
			return activePath()
		}
		return blocked
	}
	p.heal(s.reg)
	p.file.path = func() string { return "" }
	f, _ := p.data()
	if f.Dicts[0].ID != "old" || f.Groups[0].Order[0].ID != "old" {
		t.Fatalf("failed heal changed active state: %+v", f)
	}
}

// The near window is bounded where the constants that define the bounds live
// (internal/ftsq), and the stored value is what the settings panel reads back.
// Two things are pinned here: a hand-edited number past the ceiling comes back
// as the ceiling, and a zero STAYS zero - "nobody chose one" is a state the file
// may keep, and freezing the default into it would stop a later change of that
// default from ever reaching that reader.
func TestNearWordsSettingIsBounded(t *testing.T) {
	cases := []struct{ stored, want int }{
		{0, 0},
		{-4, 0},
		{ftsq.NearMin, ftsq.NearMin},
		{7, 7},
		{ftsq.NearMax, ftsq.NearMax},
		{ftsq.NearMax + 1, ftsq.NearMax},
		{9999, ftsq.NearMax},
	}
	for _, tc := range cases {
		u := &UIPrefs{NearWords: tc.stored}
		u.normalize()
		if u.NearWords != tc.want {
			t.Errorf("normalize() left nearWords=%d from %d, want %d", u.NearWords, tc.stored, tc.want)
		}
	}
}
