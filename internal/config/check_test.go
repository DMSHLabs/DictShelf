// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCheckTOML(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string // substrings, one per problem, in order
	}{
		{"the template", configTemplate, nil},
		{"a clean file", "DICT_DIR = [\"/a\",\n  \"/b\"]\nSERVER_PORT = 6888 # comment\n[section]\n", nil},
		{"a typo'd key", "DICT_DIRS = \"/a\"", []string{"line 1: unknown setting DICT_DIRS"}},
		{"lower case is not the key", "server_port = 1", []string{"unknown setting server_port"}},
		{"no equals sign", "\n\nDICT_DIR \"/a\"", []string{"line 3: not KEY = value"}},
		{"an open list", "DICT_DIR = [\"/a\",\n\"/b\"\n", []string{"line 1: the list for DICT_DIR is not closed"}},
		{"an open string", "SERVER_PORT = \"6888", []string{"not closed with its quote"}},
		{"the env-only key", "AUTH_TOKEN = \"x\"", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkTOML(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("problems %q, want %q", got, tc.want)
			}
			for i := range got {
				if !strings.Contains(got[i], tc.want[i]) {
					t.Errorf("problem %d = %q, want it to say %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// knownKeys is the template's keys: so every key Load reads must be in the
// template, or a correct setting would be reported as a typo.
func TestEveryKeyReadIsKnown(t *testing.T) {
	src, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`(?:get|getList)\("([A-Z_]+)"\)`).FindAllStringSubmatch(string(src), -1) {
		if !knownKeys[m[1]] {
			t.Errorf("Load reads %s, which the template does not document: it would be reported as unknown", m[1])
		}
	}
}

// Load carries the problems of the file in effect.
func TestLoadReportsProblems(t *testing.T) {
	p := t.TempDir() + "/wudict.toml"
	os.WriteFile(p, []byte("SERVER_PORT = 7000\nDICT_DIRS = \"/x\"\n"), 0o644)
	cfg, err := Load(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Problems) != 1 || !strings.Contains(cfg.Problems[0], "DICT_DIRS") || cfg.Port != "7000" {
		t.Errorf("problems %q, port %q", cfg.Problems, cfg.Port)
	}
}
