// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"github.com/DMSHLabs/DictShelf/internal/dict"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type exportTestDictionary struct {
	dict.Dictionary
	stop bool
	name string
}

func (d exportTestDictionary) Meta() dict.Meta {
	name := d.name
	if name == "" {
		name = "fixture"
	}
	return dict.Meta{Name: name, EntryCount: 1}
}
func (d exportTestDictionary) ExportHeadwords(_ context.Context, w io.Writer, progress func(int64)) error {
	if _, err := io.WriteString(w, "слово\n"); err != nil {
		return err
	}
	if d.stop {
		return context.Canceled
	}
	progress(1)
	return nil
}

func TestExportCancellationRemovesOnlyUnfinishedFile(t *testing.T) {
	for _, separate := range []bool{false, true} {
		t.Run(map[bool]string{false: "combined", true: "separate"}[separate], func(t *testing.T) {
			dir := t.TempDir()
			table := &jobTable{}
			j := &job{t: table, ctx: context.Background(), changed: make(chan struct{})}
			entries := []*entry{{ID: "a", Path: "a.fixture", d: exportTestDictionary{}}, {ID: "b", Path: "b.fixture", d: exportTestDictionary{stop: true}}}
			err := (&Server{}).runHeadwordExport(j, entries, nil, dir, separate)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("export error = %v", err)
			}
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if separate {
				want = 1
			}
			if len(files) != want {
				t.Fatalf("remaining files = %d, want %d", len(files), want)
			}
			if want == 1 {
				data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
				if err != nil || string(data) != "### #fixture\nслово\n" {
					t.Fatalf("completed export = %q, %v", data, err)
				}
			}
		})
	}
}

func TestExportUsesSourceFilenameAndDictionaryHeader(t *testing.T) {
	dir := t.TempDir()
	table := &jobTable{}
	j := &job{t: table, ctx: context.Background(), changed: make(chan struct{})}
	d := exportTestDictionary{}
	e := &entry{ID: "a", Path: filepath.Join("dicts", "Oxford.mdx"), d: d}
	if err := (&Server{}).runHeadwordExport(j, []*entry{e}, nil, dir, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Oxford.mdx.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "### #fixture\nслово\n" {
		t.Fatalf("export = %q", data)
	}
}

func TestCombinedExportAddsEachDictionaryHeader(t *testing.T) {
	dir := t.TempDir()
	table := &jobTable{}
	j := &job{t: table, ctx: context.Background(), changed: make(chan struct{})}
	entries := []*entry{
		{ID: "a", Path: filepath.Join("dicts", "Oxford.mdx"), d: exportTestDictionary{name: "Oxford Advanced Learner's Dictionary"}},
		{ID: "b", Path: filepath.Join("dicts", "Collins.mdx"), d: exportTestDictionary{name: "Collins English Dictionary"}},
	}
	if err := (&Server{}).runHeadwordExport(j, entries, nil, dir, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "headwords.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := "### #Oxford Advanced Learner's Dictionary\nслово\n### #Collins English Dictionary\nслово\n"
	if string(data) != want {
		t.Fatalf("combined export = %q, want %q", data, want)
	}
}

func TestExportIncludesHiddenAbbreviationCompanion(t *testing.T) {
	sourceDir := t.TempDir()
	mainPath := filepath.Join(sourceDir, "Mini.dsl")
	companionPath := filepath.Join(sourceDir, "Mini_abrv.dsl")
	main := "#NAME \"Mini\"\n#INDEX_LANGUAGE \"English\"\n\nword\n\tentry\n"
	companion := "#NAME \"Mini abbreviations\"\n#INDEX_LANGUAGE \"English\"\n\npl\n\tplural\n\nAdj.\n\tadjective\n"
	if err := os.WriteFile(mainPath, []byte(main), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(companionPath, []byte(companion), 0o600); err != nil {
		t.Fatal(err)
	}

	e := &entry{ID: "main", Path: mainPath, d: exportTestDictionary{}}
	companions := headwordExportCompanions([]*entry{e})
	if len(companions) != 1 || companions[0].Path != companionPath {
		t.Fatalf("companion plan = %#v", companions)
	}

	dir := t.TempDir()
	table := &jobTable{}
	j := &job{t: table, ctx: context.Background(), changed: make(chan struct{})}
	if err := (&Server{}).runHeadwordExport(j, []*entry{e}, companions, dir, false); err != nil {
		t.Fatal(err)
	}
	want := "### #Mini abbreviations\nAdj.\npl\n"
	data, err := os.ReadFile(filepath.Join(dir, "Mini_abrv.dsl.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("abbreviation export = %q, want %q", data, want)
	}
}

func TestExportFileNamesKeepExistingFiles(t *testing.T) {
	dir := t.TempDir()
	first, err := createExportFile(dir, `../CON:словарь`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.WriteString("keep"); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := createExportFile(dir, `../CON:словарь`)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	if first.Name() == second.Name() || filepath.Dir(first.Name()) != dir {
		t.Fatal("unsafe or reused output name")
	}
	data, err := os.ReadFile(first.Name())
	if err != nil || string(data) != "keep" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
}
