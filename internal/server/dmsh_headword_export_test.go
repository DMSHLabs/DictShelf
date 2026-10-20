// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"github.com/wuweidict/wudict/internal/dict"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type exportTestDictionary struct {
	dict.Dictionary
	stop bool
}

func (d exportTestDictionary) Meta() dict.Meta { return dict.Meta{Name: "fixture", EntryCount: 1} }
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
			err := (&Server{}).runHeadwordExport(j, entries, dir, separate)
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
				if err != nil || string(data) != "слово\n" {
					t.Fatalf("completed export = %q, %v", data, err)
				}
			}
		})
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
