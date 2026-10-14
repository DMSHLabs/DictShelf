// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
)

func TestDesktopGroupSnapshotsWithoutRouter(t *testing.T) {
	s, _ := newPrefsServer(t)
	for _, tc := range []struct{ method, path string }{
		{"groups", "/api/groups"},
		{"userGroups", "/api/user-groups"},
	} {
		t.Run(tc.method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			s.ServeHTTP(recorder, newRequest("GET", tc.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("HTTP status %d: %s", recorder.Code, recorder.Body.String())
			}
			var httpValue any
			if err := json.Unmarshal(recorder.Body.Bytes(), &httpValue); err != nil {
				t.Fatal(err)
			}

			mux := s.mux
			s.mux = http.NewServeMux()
			defer func() { s.mux = mux }()
			nativeValue, err := s.DesktopJSON("", tc.method, "")
			if err != nil {
				t.Fatal(err)
			}
			nativeJSON, err := json.Marshal(nativeValue)
			if err != nil {
				t.Fatal(err)
			}
			var decodedNative any
			if err := json.Unmarshal(nativeJSON, &decodedNative); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(httpValue, decodedNative) {
				t.Fatalf("HTTP and native snapshots differ:\nHTTP: %#v\nnative: %#v", httpValue, decodedNative)
			}
		})
	}
}

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
