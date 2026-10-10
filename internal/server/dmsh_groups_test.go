// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestUserGroupsExcludeUpstreamRules(t *testing.T) {
	s := newGroupsServer(t)
	s.UseUserGroups()
	if _, code := callGroups(t, s, "PUT", "Upstream = server\nBroken = `(`\n"); code != 200 {
		t.Fatal(code)
	}
	if got := rowGroups(t, s); len(got) != 0 {
		t.Fatalf("upstream groups reached fork rows: %v", got)
	}
	if got := s.pickerGroupsProblems(); got != 0 {
		t.Fatalf("upstream diagnostics reached fork: %d", got)
	}
	info := dictInfo{Path: "English.dsl"}
	s.langFacts(&info, "English", "English", "")
	if info.ArticleLang == "" {
		t.Fatal("disabling groups disabled article language")
	}
	if len(info.Filters) == 0 || len(info.Groups) != 0 {
		t.Fatalf("editor filters must be separate from picker groups: %+v", info)
	}
	s.userGroupsOnly = false
	if got := rowGroups(t, s); !got["my groups/upstream"] {
		t.Fatalf("upstream implementation no longer works: %v", got)
	}
}

func groupCall(t *testing.T, s *Server, method, path string, body any, status int) []byte {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRecorder()
	s.ServeHTTP(r, newRequest(method, path, strings.NewReader(string(b))))
	if r.Code != status {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, r.Code, status, r.Body.String())
	}
	return r.Body.Bytes()
}

func createTestGroup(t *testing.T, s *Server, name string) groupView {
	t.Helper()
	var g groupView
	if err := json.Unmarshal(groupCall(t, s, "POST", "/api/user-groups", map[string]string{"name": name}, 200), &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Members) != 0 {
		t.Fatal("new group is not empty")
	}
	return g
}

func TestGroupOrderModesKeepPinsAndPlaceNewMembers(t *testing.T) {
	names := map[string]string{"a": "Alpha", "b": "Bravo", "c": "Charlie", "d": "Delta", "y": "Yankee", "z": "Zulu"}
	compare := groupOrderCompare(names, "en")
	alphabetical, manual := false, true
	cases := []struct {
		name  string
		group DictionaryGroup
		add   []string
		want  []string
		mode  bool
	}{
		{"new alphabetical group", DictionaryGroup{}, []string{"d", "a", "c"}, []string{"a", "c", "d"}, false},
		{"alphabetical tail below pin", DictionaryGroup{Order: []groupOrder{{ID: "b", Pinned: true}, {ID: "d"}, {ID: "c"}}, CustomOrder: &alphabetical}, []string{"a"}, []string{"b", "a", "c", "d"}, false},
		{"manual list appends new names alphabetically", DictionaryGroup{Order: []groupOrder{{ID: "b", Pinned: true}, {ID: "d"}, {ID: "c"}}, CustomOrder: &manual}, []string{"z", "a", "y"}, []string{"b", "d", "c", "a", "y", "z"}, true},
		{"legacy manual order survives", DictionaryGroup{Order: []groupOrder{{ID: "d"}, {ID: "c"}}}, []string{"a"}, []string{"d", "c", "a"}, true},
		{"legacy alphabetical order accepts insertion", DictionaryGroup{Order: []groupOrder{{ID: "a"}, {ID: "d"}}}, []string{"c"}, []string{"a", "c", "d"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addGroupOrderMembers(&tc.group, tc.add, compare)
			if got := groupOrderIDs(tc.group.Order); !slices.Equal(got, tc.want) || tc.group.CustomOrder == nil || *tc.group.CustomOrder != tc.mode {
				t.Fatalf("order=%v customOrder=%v, want %v/%v", got, tc.group.CustomOrder, tc.want, tc.mode)
			}
		})
	}
}

func TestGroupOrderModePersistsThroughAPI(t *testing.T) {
	s, state := newPrefsServer(t)
	entries := s.reg.all()
	group := createTestGroup(t, s, "Reading")
	for _, e := range entries[:2] {
		groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": group.ID, "dict": e.ID, "member": true}, 200)
	}
	var groups []groupView
	getJSON(t, s, "/api/user-groups", &groups)
	if groups[1].CustomOrder == nil || *groups[1].CustomOrder {
		t.Fatal("new group did not start in alphabetical mode")
	}
	order := slices.Clone(groups[1].Members)
	slices.Reverse(order)
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": group.ID, "members": order, "customOrder": true}, 200)
	s.reg.prefs = LoadPrefs(state)
	getJSON(t, s, "/api/user-groups", &groups)
	if !slices.Equal(groups[1].Members, order) || groups[1].CustomOrder == nil || !*groups[1].CustomOrder {
		t.Fatalf("manual order or mode was lost: %+v", groups[1])
	}
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": group.ID, "members": order, "customOrder": false}, 200)
	getJSON(t, s, "/api/user-groups", &groups)
	f, _ := s.reg.prefs.data()
	names := groupOrderNames(f.Dicts, entries)
	for _, entry := range entries {
		names[entry.ID] = s.baseDictInfo(entry).Name
	}
	expected := groupOrderIDs(sortedGroupOrder(groupOrderEntries(order, nil), groupOrderCompare(names, f.Language)))
	if groups[1].CustomOrder == nil || *groups[1].CustomOrder || !slices.Equal(groups[1].Members, expected) {
		t.Fatalf("A-Z did not restore alphabetical mode: %+v", groups[1])
	}
}

func TestLinkedGroupRescanInsertsAlphabeticallyBelowPins(t *testing.T) {
	dir := t.TempDir()
	writeDSL := func(name string) {
		t.Helper()
		body := "#NAME \"" + name + "\"\n#INDEX_LANGUAGE \"English\"\n#CONTENTS_LANGUAGE \"Russian\"\n\nword\n\tmeaning\n"
		if err := os.WriteFile(filepath.Join(dir, name+".dsl"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeDSL("Bravo")
	writeDSL("Delta")
	isolate := filepath.Join(t.TempDir(), StateFile)
	isolatedDBDir(t)
	reg, err := NewRegistry([]string{dir}, false, WithPrefs(LoadPrefs(isolate)), WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, reg)
	s := New(reg)
	s.UseUserGroups()
	var group groupView
	if err := json.Unmarshal(groupCall(t, s, "POST", "/api/user-groups", map[string]any{"name": "English", "filter": map[string]string{"facet": "lang", "value": "en"}, "linked": true}, 200), &group); err != nil {
		t.Fatal(err)
	}
	nameOrder := func(ids []string) []string {
		byID := make(map[string]string)
		for _, e := range s.reg.all() {
			byID[e.ID] = s.baseDictInfo(e).Name
		}
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = byID[id]
		}
		return out
	}
	if got := nameOrder(group.Members); !slices.Equal(got, []string{"Bravo", "Delta"}) {
		t.Fatalf("initial linked order: %v", got)
	}
	bravo := group.Members[0]
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": group.ID, "members": group.Members, "pinned": []string{bravo}, "customOrder": false}, 200)
	writeDSL("Alpha")
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	var groups []groupView
	getJSON(t, s, "/api/user-groups", &groups)
	if got := nameOrder(groups[1].Members); !slices.Equal(got, []string{"Bravo", "Alpha", "Delta"}) || groups[1].CustomOrder == nil || *groups[1].CustomOrder {
		t.Fatalf("new linked member was not inserted below pin: %v, mode=%v", got, groups[1].CustomOrder)
	}
	manual := slices.Clone(groups[1].Members)
	manual[1], manual[2] = manual[2], manual[1]
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": group.ID, "members": manual, "pinned": []string{bravo}, "customOrder": true}, 200)
	writeDSL("Charlie")
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	getJSON(t, s, "/api/user-groups", &groups)
	if got := nameOrder(groups[1].Members); !slices.Equal(got, []string{"Bravo", "Delta", "Alpha", "Charlie"}) || groups[1].CustomOrder == nil || !*groups[1].CustomOrder {
		t.Fatalf("new linked member did not append after manual order: %v, mode=%v", got, groups[1].CustomOrder)
	}
}

func TestGroupsPersistenceAndCollection(t *testing.T) {
	s, state := newPrefsServer(t)
	e := s.reg.all()[0]
	putPrefs(t, s, `{"dicts":[{"id":"`+e.ID+`","off":true}],"ui":{"fontSize":24}}`)
	a, b := createTestGroup(t, s, "Essential"), createTestGroup(t, s, "Full")
	for _, g := range []groupView{a, b} {
		groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": g.ID, "dict": e.ID, "member": true}, 200)
	}
	// Legacy clients saving order/enabled settings must preserve membership.
	putPrefs(t, s, `{"dicts":[{"id":"`+e.ID+`","off":true}]}`)
	s.reg.prefs = LoadPrefs(state)
	var groups []groupView
	getJSON(t, s, "/api/user-groups", &groups)
	if len(groups) != 3 || !groups[0].Readonly || len(groups[0].Members) != 2 || !slices.Contains(groups[1].Members, e.ID) || !slices.Contains(groups[2].Members, e.ID) {
		t.Fatalf("restart: %+v", groups)
	}
	if !s.reg.prefs.Off(e.ID, e.Path) || s.reg.prefs.UI().FontSize != 24 {
		t.Fatal("search/UI prefs changed")
	}
	groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": a.ID, "dict": e.ID, "member": false}, 200)
	getJSON(t, s, "/api/user-groups", &groups)
	if len(groups[1].Members) != 0 || len(groups[2].Members) != 1 || !s.reg.has(e.ID) {
		t.Fatal("removing membership affected dictionary or other group")
	}
	// Registry changes are reflected automatically without a stored All list.
	path := filepath.Join(filepath.Dir(e.Path), "new.dsl")
	if err := os.WriteFile(path, []byte(sampleDSL), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	getJSON(t, s, "/api/user-groups", &groups)
	if len(groups[0].Members) != 3 {
		t.Fatal("new dictionary missing from All")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.Path); err != nil {
		t.Fatal(err)
	}
	if err := s.reg.Rescan(); err != nil {
		t.Fatal(err)
	}
	getJSON(t, s, "/api/user-groups", &groups)
	if len(groups[0].Members) != 1 || len(groups[2].Members) != 0 {
		t.Fatalf("removed dictionaries still visible: %+v", groups)
	}
}

func TestGroupOrderIsIndependentAndPersistent(t *testing.T) {
	s, state := newPrefsServer(t)
	entries := s.reg.all()
	if len(entries) < 2 {
		t.Fatal("need two dictionaries")
	}
	a, b := createTestGroup(t, s, "Essential"), createTestGroup(t, s, "Other")
	for _, g := range []groupView{a, b} {
		for _, e := range entries[:2] {
			groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": g.ID, "dict": e.ID, "member": true}, 200)
		}
	}
	order := []string{entries[1].ID, entries[0].ID}
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": a.ID, "members": order}, 200)
	s.reg.prefs = LoadPrefs(state)
	var groups []groupView
	getJSON(t, s, "/api/user-groups", &groups)
	if !slices.Equal(groups[0].Members[:2], []string{entries[0].ID, entries[1].ID}) || !slices.Equal(groups[1].Members, order) || !slices.Equal(groups[2].Members, []string{entries[0].ID, entries[1].ID}) {
		t.Fatalf("independent order lost: %+v", groups)
	}
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": "all", "members": order}, 400)
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": a.ID, "members": []string{entries[0].ID}}, 409)
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": a.ID, "members": []string{entries[0].ID, entries[0].ID}}, 400)
	s.reg.prefs.file.path = func() string { return t.TempDir() }
	groupCall(t, s, "PUT", "/api/user-groups/order", map[string]any{"group": a.ID, "members": []string{entries[0].ID, entries[1].ID}}, 500)
	// The failed save left the file alone: read it back through the real path
	// (fresh, so the recheck window does not serve the temp path's empty view).
	s.reg.prefs.file.path = func() string { return state }
	rec := s.reg.prefs.file.fresh().val
	if !slices.Equal(groupOrderIDs(rec.Groups[0].Order[:2]), order) {
		t.Fatal("failed save changed group order")
	}
	groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": a.ID, "dict": entries[1].ID, "member": false}, 200)
	getJSON(t, s, "/api/user-groups", &groups)
	if !slices.Equal(groups[1].Members, []string{entries[0].ID}) || !slices.Equal(groups[2].Members, []string{entries[0].ID, entries[1].ID}) {
		t.Fatalf("membership changed other order: %+v", groups)
	}
}

func TestGroupOrderStoresPinWithMemberAndReadsLegacyOrder(t *testing.T) {
	var legacy DictionaryGroup
	if err := json.Unmarshal([]byte(`{"id":"g","order":["d1","d2"]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(groupOrderIDs(legacy.Order), []string{"d1", "d2"}) || len(groupPinnedIDs(legacy.Order)) != 0 {
		t.Fatalf("legacy order was not read: %+v", legacy.Order)
	}

	group := DictionaryGroup{ID: "g", Order: []groupOrder{{ID: "d1", Pinned: true}, {ID: "d2"}}}
	data, err := json.Marshal(group)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["pinned"]; ok {
		t.Fatalf("pin state was serialized separately: %s", data)
	}
	var order []map[string]json.RawMessage
	if err := json.Unmarshal(stored["order"], &order); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || string(order[0]["id"]) != `"d1"` || string(order[0]["pinned"]) != "true" || string(order[1]["id"]) != `"d2"` {
		t.Fatalf("member pin state was not stored alongside its ID: %s", data)
	}
}

func TestGroupValidationAndRollback(t *testing.T) {
	s, state := newPrefsServer(t)
	for _, name := range []string{"", "  ", strings.Repeat("x", 101), "a\nb"} {
		groupCall(t, s, "POST", "/api/user-groups", map[string]string{"name": name}, 400)
	}
	createTestGroup(t, s, " Essential ")
	for _, name := range []string{"essential", "All Dictionaries", " ALL DICTIONARIES "} {
		groupCall(t, s, "POST", "/api/user-groups", map[string]string{"name": name}, 409)
	}
	e := s.reg.all()[0]
	groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": "all", "dict": e.ID, "member": false}, 400)
	groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": "missing", "dict": e.ID, "member": true}, 404)
	groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": "all", "dict": e.ID}, 400)
	// A directory cannot be atomically replaced by a JSON file.
	s.reg.prefs.file.path = func() string { return t.TempDir() }
	groupCall(t, s, "POST", "/api/user-groups", map[string]string{"name": "Failed"}, 500)
	// Nothing was written: the file through the real path is what it was.
	s.reg.prefs.file.path = func() string { return state }
	rec := s.reg.prefs.file.fresh().val
	if len(rec.Groups) != 1 {
		t.Fatal("failed save changed the stored groups")
	}
	g := rec.Groups[0]
	s.reg.prefs.file.path = func() string { return t.TempDir() }
	groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": g.ID, "dict": e.ID, "member": true}, 500)
	s.reg.prefs.file.path = func() string { return state }
	rec = s.reg.prefs.file.fresh().val
	if len(rec.Dicts) != 0 {
		t.Fatal("failed membership save changed the stored records")
	}
}

func TestGroupsConcurrentPrefs(t *testing.T) {
	s, state := newPrefsServer(t)
	g := createTestGroup(t, s, "Essential")
	e := s.reg.all()[0]
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			groupCall(t, s, "PUT", "/api/user-groups/member", map[string]any{"group": g.ID, "dict": e.ID, "member": true}, 200)
		})
		wg.Go(func() { putPrefs(t, s, `{"dicts":[{"id":"`+e.ID+`","off":true}]}`) })
	}
	wg.Wait()
	p := LoadPrefs(state)
	rec, _ := p.data()
	if len(rec.Dicts) != 1 || !rec.Dicts[0].Off || !slices.Contains(rec.Dicts[0].Groups, g.ID) {
		t.Fatalf("lost concurrent save: %+v", rec.Dicts)
	}
}

func TestGroupsHealAndRetainUnavailable(t *testing.T) {
	s, state := newPrefsServer(t)
	e := s.reg.all()[0]
	g := createTestGroup(t, s, "Essential")
	orderedGroup := g.DictionaryGroup
	orderedGroup.Order = []groupOrder{{ID: "offline"}, {ID: "old-id"}}
	seed := prefsFile{Version: 1, Groups: []DictionaryGroup{orderedGroup}, Dicts: []DictPref{
		{ID: "old-id", Path: filepath.Join("/old", filepath.Base(e.Path)), Off: true, Groups: []string{g.ID}},
		{ID: "offline", Path: "/offline/large.mdx", Groups: []string{g.ID}},
	}}
	data, _ := json.Marshal(seed)
	if err := os.WriteFile(state, data, 0600); err != nil {
		t.Fatal(err)
	}
	s.reg.prefs = LoadPrefs(state)
	var groups []groupView
	getJSON(t, s, "/api/user-groups", &groups)
	if !slices.Equal(groups[1].Members, []string{e.ID}) {
		t.Fatalf("identity was not healed: %+v", groups)
	}
	p := LoadPrefs(state)
	rec, _ := p.data()
	if len(rec.Dicts) != 2 || rec.Dicts[0].ID != e.ID || !rec.Dicts[0].Off || !slices.Contains(rec.Dicts[0].Groups, g.ID) || !slices.Contains(rec.Dicts[1].Groups, g.ID) || !slices.Equal(groupOrderIDs(rec.Groups[0].Order), []string{"offline", e.ID}) {
		t.Fatalf("healing lost state: %+v", rec.Dicts)
	}
}
