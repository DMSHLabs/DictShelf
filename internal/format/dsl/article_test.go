// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArticleLocalSamples(t *testing.T) {
	root := os.Getenv("WUDICT_ARTICLE_CORPUS")
	if root == "" {
		t.Skip("set WUDICT_ARTICLE_CORPUS to the dictionary folder")
	}
	for _, name := range []string{
		"En/En-En/Longman DOCE 5th Ed (En-En).dsl.dz",
		"En/En-Ru/Oxford (En-Ru).dsl.dz",
		"Ru/Ru-En/Zimmerman (Ru-En).dsl.dz",
	} {
		t.Run(name, func(t *testing.T) {
			reader, err := NewGDReader(filepath.Join(root, filepath.FromSlash(name)))
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			count := 0
			for count < 500 {
				entry, err := reader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := checkWellFormed(entry.Body); err != nil {
					t.Fatalf("%v: %v", entry.Headwords, err)
				}
				count++
			}
			if count == 0 {
				t.Fatal("empty sample")
			}
			t.Logf("%d articles checked", count)
		})
	}
}

func TestArticleRepair(t *testing.T) {
	for _, c := range []struct{ input, want string }{
		{`[b][i]x[/b]y[/i]`, `<i><b>x</b>y</i>`},
		{"[i]a\nb", `<i>a</i><br/>b`},
		{`[t]ma3[/t]`, `<span class="wu-ipa">ma3</span>`},
		{`[t]k‡t[/t]`, `<span class="wu-ipa">kæt</span>`},
		{`[t]aЋ[/t]`, `<span class="wu-ipa">aθ</span>`},
		{`[t]кэт[/t]`, `<span class="wu-ipa">кэт</span>`},
		{`[sic] [1] [B]x[/B]`, `[sic] [1] <b>x</b>`},
		{`[ref]some [b]word[/b][/ref]`, `<a href="entry://some word">some <b>word</b></a>`},
		{`[ref]a [ref]b[/ref] c[/ref]`, `<a href="entry://a">a </a><a href="entry://b">b</a> c`},
		{`a [ b [b]bold[/b]`, `a [ b <b>bold</b>`},
		{"e\u0301", "é"},
	} {
		got, _, err := transformArticleBody(c.input, "key", nil)
		if err != nil || got != c.want {
			t.Errorf("%q: got %q (%v), want %q", c.input, got, err, c.want)
		}
		if err := checkWellFormed(got); err != nil {
			t.Errorf("%q: %v", c.input, err)
		}
	}
}

func TestArticleDefinitionLinkIsNotHiddenAsExample(t *testing.T) {
	for _, enhance := range []bool{false, true} {
		body, _, err := transformArticleWithOptions(`[m1][ref]definition[/ref] [ex]example[/ex][/m]`, "word", nil, ArticleOptions{Enhance: enhance, Styles: true})
		if err != nil || strings.Contains(body, "wu-example-block") || !strings.Contains(body, "wu-inline-example") || !strings.Contains(body, `href="entry://definition"`) {
			t.Fatalf("enhance=%v: %s (%v)", enhance, body, err)
		}
	}
}

func TestArticleSecondaryFolding(t *testing.T) {
	for _, enhance := range []bool{false, true} {
		for _, c := range []struct {
			input string
			whole bool
		}{
			{`[m1][ex]visible example[/ex][/m]`, false},
			{`[m1][*]optional text[/*][/m]`, true},
			{`[m1][*][ex]example[/ex] translation[/*][/m]`, true},
			{`[m1]definition [*]optional[/*][/m]`, false},
			{`[m1][ref]definition[/ref] [*]optional[/*][/m]`, false},
			{`[m1][*][b]crossed[/*] visible[/b][/m]`, false},
		} {
			got, _, err := transformArticleWithOptions(c.input, "key", nil, ArticleOptions{Enhance: enhance, Styles: true})
			if err != nil || strings.Contains(got, "wu-xonly") != c.whole {
				t.Fatalf("enhance=%v %s: %s (%v)", enhance, c.input, got, err)
			}
			if err := checkWellFormed(got); err != nil {
				t.Fatal(err)
			}
		}
	}
	got, _, err := transformArticleBody("sense\n[*]optional[/*]\nnext", "key", nil)
	if err != nil || got != `sense<br/><span class="wu-sec">optional<br/></span>next` {
		t.Fatalf("secondary line break: %s (%v)", got, err)
	}
}

func FuzzArticleBody(f *testing.F) {
	for _, seed := range []string{`[b][i]x[/b]y[/i]`, `[m1][ex]example[/ex][/m]`, `[ref]a[s]x.wav[/s]b[/ref]`, `[t]ma3[/t]`, `[sic]`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		body, _, err := transformArticleWithOptions(input, "key", nil, ArticleOptions{Enhance: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := checkWellFormed(body); err != nil {
			t.Fatalf("%q: %v in %s", input, err, body)
		}
	})
}
