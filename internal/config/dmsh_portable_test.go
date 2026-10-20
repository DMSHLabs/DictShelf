// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRelativizePortableDictionaries(t *testing.T) {
	data := filepath.Join(t.TempDir(), "Data")
	if err := os.Mkdir(data, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, Name)
	internal := filepath.Join(data, "Dictionaries")
	external := filepath.Join(filepath.Dir(data), "External")
	for _, tc := range []struct {
		name       string
		dirs, want []string
	}{
		{"mixed", []string{external, internal, filepath.Join(internal, "Nested"), "Dictionaries/Relative", internal + "-external"}, []string{external, "Dictionaries", filepath.Join("Dictionaries", "Nested"), "Dictionaries/Relative", internal + "-external"}},
		{"external", []string{external}, []string{external}},
		{"empty", []string{}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte("DICT_DIR = "+FormatList(tc.dirs)+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := RelativizePortableDictionaries(path); err != nil {
				t.Fatal(err)
			}
			r, err := loadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, isList := r.lists["DICT_DIR"]
			if !isList {
				if value := r.vals["DICT_DIR"]; value != "" {
					got = []string{value}
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			before, _ := os.ReadFile(path)
			if err := RelativizePortableDictionaries(path); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("normalization is not idempotent")
			}
		})
	}
	text := "DICT_DIR = " + QuoteTOML(external) + " # keep external scalar\n"
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RelativizePortableDictionaries(path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != text {
		t.Fatal("external-only configuration was rewritten")
	}
}
