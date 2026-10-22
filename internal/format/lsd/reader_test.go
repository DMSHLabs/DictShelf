// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"io"
	"reflect"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

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
		{Headwords: []string{"A"}, Body: "abcd", Kind: dict.BodyDSL},
		{Headwords: []string{"B"}, Body: "aabb33", Kind: dict.BodyDSL},
		{Headwords: []string{"C"}, Body: "1234", Kind: dict.BodyDSL},
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
				if !reflect.DeepEqual(entry.Headwords, []string{wantWords[i]}) || entry.Body != tc.bodies[i] || entry.Kind != dict.BodyDSL {
					t.Errorf("entry %d = %#v", i, entry)
				}
			}
		})
	}
}
