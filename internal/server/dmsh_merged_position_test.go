// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

func TestMergedFindSeeksByBasePosition(t *testing.T) {
	db, err := store.OpenWildcardCacheDB(filepath.Join(t.TempDir(), "merged.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE merged_find_word(ordinal INTEGER PRIMARY KEY,source_position INTEGER NOT NULL,base_position INTEGER NOT NULL);
		CREATE INDEX merged_find_base_position ON merged_find_word(base_position);
		INSERT INTO merged_find_word VALUES(0,1000,2),(1,2000,5),(2,3000,9)`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		cursor, direction, want int
		found                   bool
	}{
		{5, 1, 2, true},
		{9, -1, 1, true},
		{5, 0, 1, true},
		{2, -1, 0, false},
		{9, 1, 0, false},
	} {
		got, err := mergedFindOrdinalAtBasePosition(db, tc.cursor, tc.direction)
		if tc.found && (err != nil || got != tc.want) || !tc.found && !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("base position %d direction %d: got %d, %v; want %d, found=%v", tc.cursor, tc.direction, got, err, tc.want, tc.found)
		}
	}
	var plan string
	if err := db.QueryRow(`EXPLAIN QUERY PLAN SELECT ordinal FROM merged_find_word WHERE base_position>? ORDER BY base_position LIMIT 1`, 5).
		Scan(new(int), new(int), new(int), &plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "merged_find_base_position") {
		t.Errorf("base-position lookup misses its index: %s", plan)
	}
}
