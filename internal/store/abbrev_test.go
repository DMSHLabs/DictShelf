// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// The abbreviation companion's identity is its CONTENT, and its timestamp only
// decides for a database built before the hash was recorded - the fallback that
// keeps an old library judged exactly as it was, never more weakly. These cases
// pin both halves of that sentence, because each one alone is a rule with a hole:
// content-only would strand old databases on nothing, and time-first would keep
// rebuilding a dictionary whose companion was merely touched.
func TestAbbrevChanged(t *testing.T) {
	dir := t.TempDir()
	companion := filepath.Join(dir, "dict_abrv.dsl")
	if err := os.WriteFile(companion, []byte("pl\n\tplural\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(companion)
	if err != nil {
		t.Fatal(err)
	}
	size := strconv.FormatInt(st.Size(), 10)
	mtime := st.ModTime().UTC().Truncate(time.Second).Format(time.RFC3339)
	hash := fsx.HashSample(companion)
	if hash == "" {
		t.Fatal("the fixture must hash")
	}
	withHash := map[string]string{
		"abbrev_path": companion, "abbrev_size": size,
		"abbrev_mtime": mtime, "abbrev_sha256": hash,
	}
	touched := func() string {
		later := time.Now().Add(2 * time.Hour)
		if err := os.Chtimes(companion, later, later); err != nil {
			t.Fatal(err)
		}
		return later.UTC().Truncate(time.Second).Format(time.RFC3339)
	}
	edited := func() {
		if err := os.WriteFile(companion, []byte("pl\n\tplurel\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name string
		meta map[string]string
		step func()
		want bool
	}{
		{"matching content is not a change", withHash, nil, false},
		{"a touched companion is not a change", withHash, func() { touched() }, false},
		{"an edited companion of the same size is a change", withHash, edited, true},
		{"a size difference is a change on its own", map[string]string{
			"abbrev_size": "1", "abbrev_sha256": hash}, nil, true},
		{"no hash: a touched companion still decides by time", map[string]string{
			"abbrev_size": size, "abbrev_mtime": mtime}, func() { touched() }, true},
		{"no hash: an untouched companion is not a change", map[string]string{
			"abbrev_size": size, "abbrev_mtime": mtime}, nil, false},
		{"a companion that is gone is a change", withHash, func() {
			if err := os.Remove(companion); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"a companion that appeared is a change", map[string]string{}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Each case starts from the fixture as written above.
			if err := os.Remove(companion); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.WriteFile(companion, []byte("pl\n\tplural\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			st, err := os.Stat(companion)
			if err != nil {
				t.Fatal(err)
			}
			meta := map[string]string{}
			for k, v := range tc.meta {
				meta[k] = v
			}
			if m, ok := meta["abbrev_mtime"]; ok && m == mtime {
				meta["abbrev_mtime"] = st.ModTime().UTC().Truncate(time.Second).Format(time.RFC3339)
			}
			if _, ok := meta["abbrev_sha256"]; ok {
				meta["abbrev_sha256"] = fsx.HashSample(companion)
			}
			if _, ok := meta["abbrev_size"]; ok && meta["abbrev_size"] == size {
				meta["abbrev_size"] = strconv.FormatInt(st.Size(), 10)
			}
			if tc.step != nil {
				tc.step()
			}
			if got := abbrevChangedMeta(meta, companion); got != tc.want {
				t.Errorf("AbbrevChanged = %v, want %v (meta %v)", got, tc.want, meta)
			}
		})
	}
}
