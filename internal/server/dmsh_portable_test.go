// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/wuweidict/wudict/internal/config"
	"github.com/wuweidict/wudict/internal/store"
)

func TestPortableLayout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows volume paths")
	}
	for _, tc := range []struct{ roots, want []string }{
		{[]string{`C:\xxx\yyy\zzz\fff\ttt`, `C:\xxx\yyy\zzz2\fff\ttt`}, []string{`zzz\fff\ttt`, `zzz2\fff\ttt`}},
		{[]string{`C:\xxx\yyy\zzz\fff\ttt`, `C:\xxx\yyy\zzz2\fff\ttt`, `D:\xxx\yyy\zzz2\fff\ttt`}, []string{`C\xxx\yyy\zzz\fff\ttt`, `C\xxx\yyy\zzz2\fff\ttt`, `D\xxx\yyy\zzz2\fff\ttt`}},
		{[]string{`C:\Dicts`}, []string{"."}},
		{[]string{`C:\Dicts`, `C:\Dicts\nested`}, []string{".", "nested"}},
		{[]string{`\\host\share\one`, `\\other\share\two`}, []string{`host\share\one`, `other\share\two`}},
	} {
		if got := portableLayout(tc.roots); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: got %v want %v", tc.roots, got, tc.want)
		}
	}
}

func TestPortableCopiesResourcesAndSettings(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "profile")
	root := filepath.Join(base, "dicts")
	target := filepath.Join(base, "portable")
	for p, text := range map[string]string{filepath.Join(source, config.Name): "DICT_DIR = []\n", filepath.Join(source, "settings.json"): "{}", filepath.Join(root, "book.mdx"): "dictionary", filepath.Join(root, "book.mdd"): "resources", filepath.Join(root, "images", "picture.png"): "image", filepath.Join(base, "app.exe"): "executable"} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	old := store.DefaultDBDir()
	store.SetDBDir(filepath.Join(source, "db"))
	defer store.SetDBDir(old)
	s := &Server{User: UserDir(source), ConfigPath: filepath.Join(source, config.Name)}
	j := &job{ctx: context.Background(), t: &jobTable{}, changed: make(chan struct{})}
	if err := s.buildPortable(j, source, filepath.Join(base, "app.exe"), target, []string{root}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"app.exe", "Data/settings.json", "Data/Dictionaries/book.mdx", "Data/Dictionaries/book.mdd", "Data/Dictionaries/images/picture.png"} {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.Stat(filepath.Join(root, "book.mdx"))
	after, _ := os.Stat(filepath.Join(target, "Data", "Dictionaries", "book.mdx"))
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("source timestamp changed")
	}
	original, _ := os.ReadFile(filepath.Join(source, config.Name))
	if string(original) != "DICT_DIR = []\n" {
		t.Fatal("source configuration changed")
	}
}

func TestPortableCancellationKeepsDestination(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "output")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "keep")
	os.WriteFile(sentinel, []byte("keep"), 0644)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j := &job{ctx: ctx, t: &jobTable{}, changed: make(chan struct{})}
	if err := portableCopy(j, base, filepath.Join(base, "nested"), map[string]bool{}); err == nil {
		t.Fatal("canceled copy succeeded")
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
		t.Fatal("destination changed")
	}
}

func TestPortableRefusesPopulatedDestination(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows operation")
	}
	base := t.TempDir()
	source, target := filepath.Join(base, "profile"), filepath.Join(base, "output")
	for _, dir := range []string{source, target} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(target, "keep")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	s := &Server{User: UserDir(source)}
	argument, _ := json.Marshal(map[string]any{"action": "start", "path": target})
	if _, err := s.createPortable(string(argument)); err == nil {
		t.Fatal("accepted a populated output folder")
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "unchanged" {
		t.Fatal("existing output changed")
	}
}

func TestPortableDictionaryCheckboxInsideProfile(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "profile")
	root := filepath.Join(source, "Dictionaries")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	for p, text := range map[string]string{filepath.Join(root, "book.mdx"): "dictionary", filepath.Join(source, config.Name): "DICT_DIR = " + config.FormatList([]string{root}) + "\n", filepath.Join(base, "app.exe"): "exe"} {
		if err := os.WriteFile(p, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	old := store.DefaultDBDir()
	store.SetDBDir(filepath.Join(source, "db"))
	defer store.SetDBDir(old)
	s := &Server{User: UserDir(source), ConfigPath: filepath.Join(source, config.Name), reg: &Registry{roots: []Root{{Path: root}}}}
	for _, checked := range []bool{false, true} {
		name := "without"
		var roots []string
		if checked {
			name = "with"
			roots = []string{root}
		}
		target := filepath.Join(base, name)
		j := &job{ctx: context.Background(), t: &jobTable{}, changed: make(chan struct{})}
		if err := s.buildPortable(j, source, filepath.Join(base, "app.exe"), target, roots); err != nil {
			t.Fatal(err)
		}
		_, err := os.Stat(filepath.Join(target, "Data", "Dictionaries", "book.mdx"))
		if checked && err != nil {
			t.Fatal(err)
		}
		if !checked && !os.IsNotExist(err) {
			t.Fatal("unchecked dictionaries were copied")
		}
		if !checked {
			cfg, err := config.Load(filepath.Join(target, "Data", config.Name), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.DictDirs) != 1 || cfg.DictDirs[0] != root {
				t.Fatalf("external dictionary paths changed: %v", cfg.DictDirs)
			}
		}
	}
}
