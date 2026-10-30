// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

func TestWildcardCachePruneLinkedLayouts(t *testing.T) {
	for _, oldest := range []string{"separate", "merged"} {
		t.Run(oldest, func(t *testing.T) {
			dir := t.TempDir()
			id := "separatecache1234"
			mergedID := "mergedcache12345"
			entry, err := openWildcardCache(id, wildcardCacheSignature{query: "*dog*", group: "all", wildcards: true}, indexconst.WildcardCacheBlockSize, 1, dir)
			if err != nil {
				t.Fatal(err)
			}
			separate := entry.path
			if err := entry.db.Close(); err != nil {
				t.Fatal(err)
			}
			merged := filepath.Join(dir, "wildcard_merged_"+mergedID+".db")
			db, err := store.OpenWildcardCacheDB(merged)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TABLE merged_meta(signature TEXT,total INTEGER,alphabet_json TEXT)`); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := rememberMergedCacheGroup(merged, "all", false, false); err != nil {
				t.Fatal(err)
			}
			find := filepath.Join(dir, "wildcard_merged_find_findcache123456.db")
			db, err = store.OpenWildcardCacheDB(find)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TABLE merged_find_meta(signature TEXT,total INTEGER,alphabet_json TEXT)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO merged_find_meta VALUES(?,0,'[]')`, mergedID+"\x00*dog*\x00\x00true\x00false"); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-2 * time.Hour)
			if oldest == "separate" {
				if err := os.Chtimes(separate, old, old); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Chtimes(merged, old, old); err != nil {
				t.Fatal(err)
			}
			cache := &wildcardPageCache{dir: dir, maxBytes: 1}
			cache.pruneLocked(time.Now(), find, false)
			for _, path := range []string{separate, merged, find} {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("active group lost %s: %v", path, err)
				}
			}
			cache.pruneLocked(time.Now(), "", false)
			for _, path := range []string{separate, merged, find} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("linked cache retained %s: %v", path, err)
				}
			}
		})
	}
}
