// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/format/dsl"
	"github.com/wuweidict/wudict/internal/store"
)

func TestDSLHeadingPreservesSupplementaryUnicode(t *testing.T) {
	for _, tc := range []struct {
		name       string
		sorted     []uint16
		extensions []extChar
		want       string
	}{
		{
			name:   "sorted surrogate pair",
			sorted: utf16.Encode([]rune("Baby Jae💖")),
			want:   "Baby Jae💖",
		},
		{
			name:       "unsorted surrogate pair",
			sorted:     []uint16{'A'},
			extensions: []extChar{{index: 0, value: 0xd83d}, {index: 1, value: 0xdc96}},
			want:       "{💖}A",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dslHeading(tc.sorted, tc.extensions); got != tc.want {
				t.Fatalf("dslHeading = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestReaderSimpleFixture(t *testing.T) {
	r, err := NewReader("testdata/simple_testdict1/test.lsd")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if got := r.Meta(); got.Name != "Country Capital Dictionary [en-en]" || got.EntryCount != 3 || got.IndexLang != "en" || got.ContentsLang != "en" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	want := []dict.Entry{
		{Headwords: []string{"A"}, Body: "abcd", Kind: dict.BodyHTML},
		{Headwords: []string{"B"}, Body: "aabb33", Kind: dict.BodyHTML},
		{Headwords: []string{"C"}, Body: "1234", Kind: dict.BodyHTML},
	}
	for i, expected := range want {
		got, err := r.Next()
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("entry %d = %#v; want %#v", i, got, expected)
		}
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("end = %v; want EOF", err)
	}
}

func TestReaderLingvo10Dictionary(t *testing.T) {
	path := `D:\Dictionaries\Test\(RUS-RUS) Русский экономический словарь (#2).lsd`
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Meta().EntryCount != 18370 {
		t.Fatalf("entry count = %d; want 18370", r.Meta().EntryCount)
	}
	first, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Headwords) == 0 || first.Headwords[0] != `"АВАНСЫ ОТ ЗАКАЗЧИКОВ"` || !strings.Contains(first.Body, "счет") {
		t.Fatalf("unexpected first entry: %#v", first)
	}
	count := 1
	for {
		_, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("entry %d: %v", count, err)
		}
		count++
	}
	if count != 19625 {
		t.Fatalf("read %d visible headings; want 19625", count)
	}
}

func TestReaderLingvo7Dictionaries(t *testing.T) {
	base := `D:\Dictionaries\Test3\Елистратов. Словарь русского арго (2002)\`
	for _, tc := range []struct {
		name  string
		dsl   string
		count int
	}{
		{name: "Словарь русского арго.lsd", dsl: "Словарь русского арго.dsl.dz", count: 15699},
		{name: "Словарь русского арго_abrv.lsd", dsl: "Словарь русского арго_abrv.dsl.dz", count: 131},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewReader(filepath.Join(base, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if r.Meta().EntryCount != tc.count {
				t.Fatalf("header count = %d; want %d", r.Meta().EntryCount, tc.count)
			}
			count := 0
			for {
				_, err := r.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("entry %d: %v", count, err)
				}
				count++
			}
			if count == 0 || count > tc.count {
				t.Fatalf("decoded %d article groups from %d headings", count, tc.count)
			}
			if err := compareLegacyHeadwords(filepath.Join(base, tc.name), filepath.Join(base, tc.dsl)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func compareLegacyHeadwords(lsdPath, dslPath string) error {
	lsdReader, err := NewReader(lsdPath)
	if err != nil {
		return err
	}
	defer lsdReader.Close()
	lsdKeys := make(map[string]struct{})
	for {
		entry, err := lsdReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		for _, word := range entry.Headwords {
			lsdKeys[strings.ToLower(word)] = struct{}{}
		}
	}
	dslReader, err := dsl.NewReader(dslPath)
	if err != nil {
		return err
	}
	defer dslReader.Close()
	dslKeys := make(map[string]struct{})
	for {
		entry, err := dslReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		for _, word := range entry.Headwords {
			dslKeys[strings.ToLower(word)] = struct{}{}
		}
	}
	for word := range lsdKeys {
		if _, ok := dslKeys[word]; !ok {
			return fmt.Errorf("LSD headword %q is missing from %s", word, filepath.Base(dslPath))
		}
	}
	for word := range dslKeys {
		if _, ok := lsdKeys[word]; !ok {
			return fmt.Errorf("DSL headword %q is missing from %s", word, filepath.Base(lsdPath))
		}
	}
	return nil
}
func TestReaderLingvo9Dictionary(t *testing.T) {
	path := `D:\Dictionaries\Test3\Грицанов. Энциклопедия история философии (2002)\hist_of_philosophyl_gricanov.lsd`
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Meta().EntryCount != 654 {
		t.Fatalf("header count = %d; want 654", r.Meta().EntryCount)
	}
	count := 0
	for {
		_, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("entry %d: %v", count, err)
		}
		count++
	}
	if count == 0 {
		t.Fatal("decoded no article groups")
	}
}

func TestLingvo9AlternateHeadwordVisible(t *testing.T) {
	path := `D:\Dictionaries\Test3\1\hist_of_philosophyl_gricanov.lsd`
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "philosophy.db")
	if _, err := store.IngestPlan(r, dbPath, store.Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const key = "АПОЛЛОНОВСКОЕ И ДИОНИСИЙСКОЕ"
	words, _, _, err := s.FindHeadwordsMode(context.Background(), key, "exact", 0, 10)
	if err != nil || len(words) != 1 || words[0] != key {
		t.Fatalf("browse search = %q, %v; want %q", words, err, key)
	}
	page, err := s.Page(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range page {
		if word == key {
			return
		}
	}
	t.Fatalf("%q is missing from the browse list", key)
}

func TestFormatRegistration(t *testing.T) {
	path := "testdata/simple_testdict1/test.lsd"
	r, err := dict.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Meta().Format != "lsd" {
		t.Fatalf("registered reader format = %q", r.Meta().Format)
	}
	if got := dict.ReaderVersion("lsd"); got != ReaderVersion {
		t.Fatalf("reader version = %d; want %d", got, ReaderVersion)
	}
	d, err := dict.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Meta().Format != "lsd" {
		t.Fatalf("registered dictionary format = %q", d.Meta().Format)
	}
}

func TestNonAdjacentAliasSurvivesIngest(t *testing.T) {
	path := "testdata/simple_testdict1/variants_testdict.lsd"
	r, err := dict.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "text.db")
	report, err := store.IngestPlan(r, dbPath, store.Plan{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.UnresolvedLinks != 0 {
		t.Fatalf("unresolved aliases = %d", report.UnresolvedLinks)
	}
	d, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	results, err := d.Exact("z", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("lookup for nonadjacent alias returned %d rows: %#v", len(results), results)
	}
}

func TestReaderSupportedFixtureVariants(t *testing.T) {
	for _, name := range []string{"headingsTestDict1_12.lsd", "headingsTestDict1_x3.lsd", "headingsTestDict1_x5.lsd", "testext.lsd"} {
		t.Run(name, func(t *testing.T) {
			r, err := NewReader("testdata/simple_testdict1/" + name)
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer r.Close()
			for i := 0; i < r.Meta().EntryCount; i++ {
				_, err = r.Next()
				if err != nil {
					t.Fatalf("read entry %d: %v", i, err)
				}
			}
			if _, err := r.Next(); err != io.EOF {
				t.Fatalf("end = %v; want EOF", err)
			}
		})
	}
}

func TestReaderHeadingFixtureContents(t *testing.T) {
	wantWords := []string{"Abc", "Abcde", "Abcdefg", "Abcdefg123", "Abcdefg123ZzzzZ", "anotherone", "Zzxx"}
	wantX12Bodies := []string{"[trn]abcd[/trn]", "[trn]aabb33[/trn]", "[trn]1234[/trn]", "[trn]54321[/trn]", "[trn]kjlkjkj[/trn]", "[trn]12345[/trn]", "[trn]32432[/trn]"}
	wantX5Bodies := []string{"abcd", "aabb33", "1234", "54321", "kjlkjkj", "12345", "32432"}
	for _, tc := range []struct {
		file   string
		bodies []string
	}{
		{"headingsTestDict1_12.lsd", wantX12Bodies},
		{"headingsTestDict1_x3.lsd", wantX12Bodies},
		{"headingsTestDict1_x5.lsd", wantX5Bodies},
	} {
		t.Run(tc.file, func(t *testing.T) {
			r, err := NewReader("testdata/simple_testdict1/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			for i := range wantWords {
				entry, err := r.Next()
				if err != nil {
					t.Fatalf("entry %d: %v", i, err)
				}
				body, err := dsl.NormalizeArticle(tc.bodies[i], wantWords[i])
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(entry.Headwords, []string{wantWords[i]}) || entry.Body != body || entry.Kind != dict.BodyHTML {
					t.Errorf("entry %d = %#v", i, entry)
				}
			}
		})
	}
}

func TestReaderExtendedTitleKeys(t *testing.T) {
	r, err := NewReader("testdata/simple_testdict1/testext.lsd")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for i, want := range [][]string{{"Abc e"}, {"bipolar affective disorder"}} {
		entry, err := r.Next()
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if !reflect.DeepEqual(entry.Headwords, want) {
			t.Errorf("entry %d headwords = %q; want %q", i, entry.Headwords, want)
		}
		if !strings.Contains(entry.Body, "<b>") {
			t.Errorf("entry %d lost its formatted display title: %q", i, entry.Body)
		}
	}
}

func TestOverlayResources(t *testing.T) {
	for _, file := range []string{"overlay_12.lsd", "overlay_x3.lsd", "overlay_x5.lsd"} {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join("testdata", "simple_testdict1", file)
			d, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			names := d.Resources()
			if !reflect.DeepEqual(names, []string{"image1.bmp", "image2.bmp"}) {
				t.Fatalf("resource names = %q", names)
			}
			for _, name := range names {
				got, _, err := d.Resource(name)
				if err != nil {
					t.Fatalf("read %s: %v", name, err)
				}
				gotBytes, readErr := io.ReadAll(got)
				closeErr := got.Close()
				if readErr != nil || closeErr != nil {
					t.Fatalf("read/close %s: %v / %v", name, readErr, closeErr)
				}
				wantBytes, err := os.ReadFile(filepath.Join("testdata", "simple_testdict1", name))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(gotBytes, wantBytes) {
					t.Errorf("%s bytes differ: got %d, want %d", name, len(gotBytes), len(wantBytes))
				}
			}
		})
	}
}

func TestOverlayResourcesSurviveMediaPacking(t *testing.T) {
	dir := t.TempDir()
	textDB := filepath.Join(dir, store.TextDBName)
	r, err := NewReader(filepath.Join("testdata", "simple_testdict1", "test.lsd"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.IngestPlan(r, textDB, store.Plan{}, nil); err != nil {
		r.Close()
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	uuid, err := store.ReadMetaValue(textDB, "dict_uuid")
	if err != nil {
		t.Fatal(err)
	}

	d, err := Open(filepath.Join("testdata", "simple_testdict1", "overlay_x5.lsd"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	mediaDB := filepath.Join(dir, store.MediaDBName)
	if err := store.IngestMedia(d, d.Resources(), mediaDB, uuid, nil); err != nil {
		t.Fatal(err)
	}

	prepared, err := store.Open(textDB)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	for _, name := range d.Resources() {
		got, _, err := prepared.Resource(name)
		if err != nil {
			t.Fatalf("packed resource %q: %v", name, err)
		}
		gotBytes, readErr := io.ReadAll(got)
		closeErr := got.Close()
		wantBytes, fileErr := os.ReadFile(filepath.Join("testdata", "simple_testdict1", name))
		if readErr != nil || closeErr != nil || fileErr != nil {
			t.Fatalf("read/close/fixture %q: %v / %v / %v", name, readErr, closeErr, fileErr)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("packed resource %q differs: got %d bytes, want %d", name, len(gotBytes), len(wantBytes))
		}
	}
}

func TestEmbeddedAnnotation(t *testing.T) {
	about, ok := dict.AboutFor("lsd", "testdata/simple_testdict1/test.lsd")
	if !ok {
		t.Fatal("embedded annotation not found")
	}
	if len(about.Sections) != 1 || about.Sections[0].Text != "yjsakfabcdaskdhabbdfjkgh1jkh33jkhj331ddj" {
		t.Fatalf("unexpected annotation: %#v", about)
	}
}

func TestReaderVariantFixtures(t *testing.T) {
	for _, tc := range []struct {
		file  string
		count int
	}{
		{"variants_testdict.lsd", 12},
		{"variants_testdict2.lsd", 6},
		{"unsorted_testdict.lsd", 10},
	} {
		t.Run(tc.file, func(t *testing.T) {
			r, err := NewReader("testdata/simple_testdict1/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			count := 0
			for {
				entry, err := r.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("entry %d: %v", count, err)
				}
				if entry.LinkTo != "" && (len(entry.Headwords) != 1 || entry.Body != "") {
					t.Errorf("redirect entry is malformed: %#v", entry)
				}
				count++
			}
			if count != tc.count {
				t.Fatalf("entries = %d; want %d", count, tc.count)
			}
		})
	}
}
