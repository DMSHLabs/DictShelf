// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestDesktopGroupEditsPersist(t *testing.T) {
	s, state := newPrefsServer(t)
	call := func(method string, body any) []byte {
		t.Helper()
		argument, _ := json.Marshal(body)
		result, err := s.DesktopJSON("http://127.0.0.1:6888", method, string(argument))
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		data, _ := json.Marshal(result)
		return data
	}
	var group groupView
	if err := json.Unmarshal(call("createGroup", map[string]any{"name": "Native group"}), &group); err != nil {
		t.Fatal(err)
	}
	entries := s.reg.all()
	first, second := entries[0].ID, entries[1].ID
	call("renameGroup", map[string]any{"group": group.ID, "name": "Renamed"})
	call("setGroupMember", map[string]any{"group": group.ID, "dict": first, "member": true})
	call("addGroupMembers", map[string]any{"group": group.ID, "members": []string{second}})
	call("replaceGroupMembership", map[string]any{"group": group.ID, "members": []string{first, second}})
	call("saveGroupOrder", map[string]any{"group": group.ID, "members": []string{second, first}, "pinned": []string{second}})
	call("linkGroup", map[string]any{"group": group.ID, "filter": nil})
	s.reg.prefs = LoadPrefs(state)
	var groups []groupView
	if err := json.Unmarshal(call("userGroups", nil), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[1].Name != "Renamed" || !slices.Equal(groups[1].Members, []string{second, first}) {
		t.Fatalf("native edits lost after reload: %+v", groups)
	}
	call("deleteGroup", map[string]any{"group": group.ID})
	if err := json.Unmarshal(call("userGroups", nil), &groups); err != nil || len(groups) != 1 {
		t.Fatalf("native delete: %+v error=%v", groups, err)
	}
	if !s.reg.has(first) || !s.reg.has(second) {
		t.Fatal("deleting the group removed dictionaries")
	}
}
