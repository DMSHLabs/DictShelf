// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/favorites"
	_ "github.com/DMSHLabs/DictShelf/internal/format/mdx"
)

func TestExportMDX(t *testing.T) {
	s := favorites.New(t.TempDir())
	if err := s.EnsureNamed("Мои статьи"); err != nil {
		t.Fatal(err)
	}
	image := []byte(strings.Repeat("image", 250000))
	_, err := s.Add(favorites.Provenance{DictionaryID: "source", Headword: "Яблоко"}, `<p>Яблоко <img src="photo.png"></p>`, []favorites.AssetInput{{URL: "photo.png", Name: "photo.png", MIME: "image/png", Data: image}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// The name the dialog passes types the dictionary inside the export and on
	// its filename, and the progress the job shows is two steps per article.
	type sample struct{ done, total int }
	var progress []sample
	for i := 0; i < 2; i++ {
		progress = nil
		mdx, mdd, err := s.ExportMDX(context.Background(), dir, "Осенние вырезки", func(done, total int) {
			progress = append(progress, sample{done, total})
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := "Осенние вырезки"; !strings.HasPrefix(filepath.Base(mdx), want) {
			t.Fatalf("export named %q, want it to start with %q", filepath.Base(mdx), want)
		}
		if filepath.Base(mdx) == filepath.Base(mdd) {
			t.Fatal("extensions should differ")
		}
		d, err := dict.Open(mdx)
		if err != nil {
			t.Fatal(err)
		}
		if want := "Осенние вырезки"; d.Meta().Name != want {
			t.Fatalf("export title %q, want %q", d.Meta().Name, want)
		}
		hits, err := d.Exact("Яблоко", 5)
		if err != nil || len(hits) != 1 {
			t.Fatalf("hits: %v %v", hits, err)
		}
		if !strings.Contains(hits[0].Body, `src="_favourites/`) {
			t.Fatalf("article has no portable image: %s", hits[0].Body)
		}
		src := strings.Split(strings.Split(hits[0].Body, `src="`)[1], `"`)[0]
		imageReader, _, err := d.Resource(src)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := io.ReadAll(imageReader)
		imageReader.Close()
		if err != nil || string(loaded) != string(image) {
			t.Fatalf("companion image mismatch: %v", err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		m, err := dict.Open(mdd)
		if err != nil {
			t.Fatal(err)
		}
		resources := m.Keywords(0, 5)
		if len(resources) != 1 {
			t.Fatalf("resources: %v", resources)
		}
		r, _, err := m.Resource(resources[0])
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(r)
		r.Close()
		if err != nil || string(got) != string(image) {
			t.Fatalf("image mismatch: %v", err)
		}
		m.Close()
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 4 {
		t.Fatalf("exports: %v %v", files, err)
	}
	if len(progress) == 0 {
		t.Fatal("no progress was reported")
	}
	if last := progress[len(progress)-1]; last.done != last.total || last.total != 2 {
		t.Fatalf("last progress %+v, want the two steps of one article", last)
	}
	for i := 1; i < len(progress); i++ {
		if progress[i].done < progress[i-1].done {
			t.Fatalf("progress went backwards: %+v", progress)
		}
	}
}
