// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"strings"
	"testing"
)

func TestWildcardCachePositionSeek(t *testing.T) {
	dir := t.TempDir()
	cache := &wildcardPageCache{}
	cache.init(dir)
	id := "0123456789abcdef"
	signature := wildcardCacheSignature{query: "*d*g*", group: "all", wildcards: true}
	entry, token, err := cache.begin(id, signature, browsePageSize, 1, "dict", "Dictionary", 0, "dog", false, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer entry.db.Close()
	for _, event := range []browseFindStreamEvent{
		{Words: []string{"one"}, Positions: []int{2}, Total: 1},
		{Words: []string{"two", "three"}, Positions: []int{5, 9}, Total: 3},
		{Words: []string{"four"}, Positions: []int{20}, Total: 4, Done: true},
	} {
		if err := entry.append("dict", "dog", false, token, event); err != nil {
			t.Fatal(err)
		}
	}
	key := wildcardStreamKey{dictID: "dict", within: "dog"}
	cache.finish(entry, &key, token)
	assertWildcardPositionSeeks(t, cache, id, signature)

	page, ok, err := cache.page(id, signature, "dict", "dog", false, 1, 0, 1)
	if err != nil || !ok || len(page.Positions) != 4 || page.Positions[0] != 2 || page.Positions[1] != 5 || page.Positions[2] != 9 || page.Positions[3] != 20 {
		t.Fatalf("logical page 1 = %+v, available=%v, error=%v", page, ok, err)
	}
	var plan string
	rows, err := entry.db.Query(`EXPLAIN QUERY PLAN SELECT positions_json,start_offset,word_count
		FROM wildcard_block WHERE within_text=? AND within_wildcards=? AND dict_id=? AND last_position>?
		ORDER BY last_position LIMIT 1`, "dog", 0, "dict", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, parent, unused int
		if err := rows.Scan(&id, &parent, &unused, &plan); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(plan, "wildcard_block_last_position") {
			return
		}
	}
	t.Fatalf("position lookup did not use the SQLite index: %s", plan)
}

func assertWildcardPositionSeeks(t *testing.T, cache *wildcardPageCache, id string, signature wildcardCacheSignature) {
	t.Helper()
	for _, test := range []struct {
		cursor, direction, want int
		found                   bool
	}{
		{5, 1, 2, true},
		{9, -1, 1, true},
		{20, 0, 3, true},
		{2, -1, 0, false},
		{20, 1, 0, false},
		{8, 0, 0, false},
	} {
		got, found, err := cache.findPosition(id, signature, "dict", "dog", false, test.cursor, test.direction, 0, 1)
		if err != nil || found != test.found || found && got != test.want {
			t.Errorf("findPosition(%d,%d) = (%d,%v,%v), want (%d,%v)",
				test.cursor, test.direction, got, found, err, test.want, test.found)
		}
	}
}
