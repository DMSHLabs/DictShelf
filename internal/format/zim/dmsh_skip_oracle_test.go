// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package zim

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
	"os"
	"path/filepath"
	"testing"
)

// These digests were captured with the complete sequential reader before
// skipTo. They include every hit, frequency, document length, score and order.
func TestRealSkipOracle(t *testing.T) {
	dir := os.Getenv("WUDICT_TEST_ZIM_DIR")
	if dir == "" {
		t.Skip("WUDICT_TEST_ZIM_DIR not set")
	}
	raw, err := os.ReadFile("testdata/skip-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Archive, Query, SHA256 string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	var d *Dict
	var db *glass.DB
	current := ""
	defer func() {
		if d != nil {
			d.Close()
		}
	}()
	for _, c := range cases {
		if c.Archive != current {
			if d != nil {
				d.Close()
			}
			d, err = Open(filepath.Join(dir, c.Archive))
			if err != nil {
				t.Fatal(err)
			}
			w, n, e := d.XapianIndex()
			if e != nil {
				t.Fatal(e)
			}
			db, err = glass.OpenSource(w, n)
			if err != nil {
				t.Fatal(err)
			}
			current = c.Archive
		}
		t.Run(c.Archive+"/"+c.Query, func(t *testing.T) {
			r, err := db.Search(c.Query)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(raw))
			if got != c.SHA256 {
				t.Fatalf("answer differs: sha256=%s want=%s", got, c.SHA256)
			}
		})
	}
}
