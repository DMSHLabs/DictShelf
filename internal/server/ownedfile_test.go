// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wuweidict/wudict/internal/facet"
)

func countingFile(path string, parses *int) *ownedFile[string] {
	return &ownedFile[string]{
		name: "t.txt", path: func() string { return path }, max: 1 << 10, perm: 0o644,
		parse:  func(text string) (string, []facet.Problem) { *parses++; return "parsed:" + text, nil },
		absent: func() string { return "absent" },
	}
}

// Read on every use (recheck 0) but parsed only when the text changed - also
// while there is no file at all.
func TestOwnedFileParsesOnlyOnChange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.txt")
	parses := 0
	f := countingFile(p, &parses)
	for range 3 {
		if v := f.now().val; v != "absent" {
			t.Fatalf("no file: %q", v)
		}
	}
	os.WriteFile(p, []byte("one"), 0o644)
	for range 3 {
		if v := f.now().val; v != "parsed:one" {
			t.Fatalf("file: %q", v)
		}
	}
	if parses != 1 {
		t.Errorf("%d parses for one text, want 1", parses)
	}
	if st, err := f.save("two", false); err != nil || st.val != "parsed:two" {
		t.Fatalf("save: %+v %v", st, err)
	}
	if v := f.now().val; v != "parsed:two" || parses != 2 {
		t.Errorf("after save: %q, %d parses (the saved text is not parsed twice)", v, parses)
	}
	if st, err := f.remove(); err != nil || st.val != "absent" || st.exists {
		t.Errorf("remove: %+v %v", st, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("remove left the file")
	}
}

// No folder: the value lives in memory, saves included.
func TestOwnedFileInMemory(t *testing.T) {
	parses := 0
	f := countingFile("", &parses)
	if st := f.now(); st.val != "absent" || st.exists {
		t.Fatalf("empty: %+v", st)
	}
	f.save("kept", false)
	if st := f.now(); st.val != "parsed:kept" || !st.exists {
		t.Errorf("in memory: %+v", st)
	}
}
