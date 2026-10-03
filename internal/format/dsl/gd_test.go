// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
	"github.com/wuweidict/wudict/internal/store/goldentest"
)

func TestGDMarkup(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"crossed", `[b][i]one[/b] two[/i]`, `<b><i>one</i></b><i> two</i>`},
		{"unclosed", `[b]one`, `<b>one</b>`},
		{"nested ref", `[ref]some [b]word[/b][/ref]`, `<a href="entry://some word">some <b>word</b></a>`},
		{"target ref", `[ref target="C#"]the [i]word[/i][/ref]`, `<a href="entry://C%23">the <i>word</i></a>`},
		{"nested url", `[url target="https://example.org"]some [b]word[/b][/url]`, `<a href="https://example.org">some <b>word</b></a>`},
		{"spaces", `[b]some [/b]word`, `<b>some </b>word`},
		{"ipa", "[t]\u2020\u040f:'[/t]", `<span class="wu-ipa">` + "\u0259\u0283\u02d0\u02c8" + `</span>`},
		{"outside ipa", "\u2020\u040f:'", "\u2020\u040f:'"},
		{"escaped ipa space", `[t]a\ b[/t]`, `<span class="wu-ipa">a` + "\u00a0" + `b</span>`},
		{"blank line", "one\n\t\\\n\ttwo", "one<br/>\u00a0<br/>two"},
		{"escaped", `[[b]] \~ \ `, `[b] ~ ` + "\u00a0"},
		{"empty margin", `[m1][/m][m2]one[/m]`, `<p class="wu-m" style="--wd-m:2">one</p>`},
		{"margin repair", `[b]one[m1]two[/b] three[/m]`, `<b>one</b><p class="wu-m" style="--wd-m:1"><b>two</b> three</p>`},
		{"unknown", `[custom]one[/custom]`, `<span class="wu-unknown">[custom]one</span>`},
		{"literal angles", `one >> two`, `one &gt;&gt; two`},
		{"literal caret", `the symbol ^ , a sign (^), ^fathers-in-law, 10^-18, ^`, `the symbol ^ , a sign (^), ^fathers-in-law, 10^-18, ^`},
		{"angle link", `<<some [b]word[/b]>>`, `<a href="entry://some word">some <b>word</b></a>`},
		{"line breaks", "[m1]one[/m]\n\t[m2]two[/m]", `<p class="wu-m" style="--wd-m:1">one</p><p class="wu-m" style="--wd-m:2">two</p>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := transformGDBody(c.input, "key", nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q; want %q", got, c.want)
			}
		})
	}
}

func TestGDClosedMarginsWithTranslationWrapper(t *testing.T) {
	body := `	[m2][trn][!trs]\[pɔt\][/!trs] [p]n[/p] ([i]for cooking, flowers[/i]) горш[']о[/']к[p]*[/p]; [b]([/b][i]also:[/i] [b]teapot)[/b] (зав[']а[/']рочный) ч[']а[/']йник; [b]([/b][i]also:[/i] [b]coffeepot)[/b] коф[']е[/']йник; ([i]bowl, container[/i]) б[']а[/']нка; ([p][i]inf[/i][/p]: [i]marijuana[/i]) план[/m]
	[m2]◆ [p]vt[/p] ([i]plant[/i]) саж[']а[/']ть (посад[']и[/']ть[p]*[/p] [p][i]perf[/i][/p]);[/m]
	[m1][b]a pot of tea[/b] ч[']а[/']йник ч[']а[/']я;[/m]
	[m1][b]to go to pot[/b] ([p][i]inf[/i][/p]: [i]work, performance[/i]) разв[']а[/']ливаться (развал[']и[/']ться[p]*[/p] [p][i]perf[/i][/p]);[/m]
	[m1][b]pots of[/b] ([p]BRIT[/p]: [p][i]inf[/i][/p]) к[']у[/']ча [i]+ [p]gen[/i][/p], [']у[/']йма [i]+ [p]gen[/i][/p][/m]
	[m1][b]potash[/b] [!trs]\['pɔtжʃ\][/!trs] [p]n[/p] пот[']а[/']ш[/trn][/m]`
	for _, enhance := range []bool{false, true} {
		options := GDOptions{Enhance: enhance}
		closed, _, err := transformGDWithOptions(body, "pot", nil, options)
		if err != nil {
			t.Fatal(err)
		}
		open, _, err := transformGDWithOptions(strings.ReplaceAll(body, "[/m]", ""), "pot", nil, options)
		if err != nil {
			t.Fatal(err)
		}
		if closed != open || strings.Contains(closed, "<br") {
			t.Fatalf("enhance=%v: closed %q; open %q", enhance, closed, open)
		}
		for _, separator := range []string{"[br]", "\\ \n"} {
			got, _, err := transformGDWithOptions("[m1][trn]one[/m]"+separator+"[m1]two[/trn][/m]", "pot", nil, options)
			if err != nil {
				t.Fatal(err)
			}
			if separator == "[br]" && !strings.Contains(got, "<br") || separator != "[br]" && !strings.Contains(got, "\u00a0") {
				t.Fatalf("explicit separator lost: %q", got)
			}
		}
	}
}
func TestGDMediaAndAbbrev(t *testing.T) {
	ab := &abbrevMap{exact: map[string]string{"pl": "plural"}}
	html, media, err := transformGDBody(`[p][b]pl[/b][/p] [s]sound.wav[/s] [s]image.png[/s]`, "key", ab)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<abbr class="wu-abbr" title="plural"><b>pl</b></abbr>`) ||
		!strings.Contains(html, `class="wu-audio"`) || !strings.Contains(html, `src="image.png"`) {
		t.Fatal(html)
	}
	if !reflect.DeepEqual(media, []string{"sound.wav", "image.png"}) {
		t.Fatal(media)
	}
}

func TestGDHTMLReferenceFixes(t *testing.T) {
	ab := &abbrevMap{exact: map[string]string{"n.": "noun - short"}, fold: map[string]string{"n.": "noun - short"}}
	for _, c := range []struct{ input, want string }{
		{`[ref target="  some   word  "]label[/ref]`, `<a href="entry://some word">label</a>`},
		{`[url target="  example.org  "]label[/url]`, `<a href="http://example.org">label</a>`},
		{`[p]n.[/p]`, "<span class=\"wu-p\"><abbr class=\"wu-abbr\" title=\"noun\u00a0\u2011\u00a0short\">n.</abbr></span>"},
		{`[p] N. [/p]`, `<span class="wu-p"> N. </span>`},
		{`[xyz foo="bar"]body[/xyz]`, `<span class="wu-unknown">[xyz foo="bar"]body</span>`},
		{`[b][xyz foo="bar"]one[/b]two[/xyz]`, `<b><span class="wu-unknown">[xyz foo="bar"]one</span></b><span class="wu-unknown">[xyz foo="bar"]two</span>`},
		{`[xyz foo="bar"]one[m1]two[/xyz][/m]`, `<span class="wu-unknown">[xyz foo="bar"]one</span><p class="wu-m" style="--wd-m:1"><span class="wu-unknown">[xyz foo="bar"]two</span></p>`},
		{`[trs]body[/trs]`, `<span class="wu-unknown">[trs]body</span>`},
		{`[lang name=Russian]body[/lang]`, `<span class="wu-lang">body</span>`},
		{`[lang id=2 name="English"]body[/lang]`, `<span class="wu-lang">body</span>`},
		{"e\u0301 [b]o\u0308[/b]", "\u00e9 <b>\u00f6</b>"},
		{`foo[u] bar[/u]`, `foo <u> bar</u>`},
		{`[ref][t]0[/t][/ref]`, "<a href=\"entry://\u03b2\"><span class=\"wu-ipa\">\u03b2</span></a>"},
	} {
		got, _, err := transformGDBody(c.input, "key", ab)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q (%v); want %q", c.input, got, err, c.want)
		}
	}
}

func TestGDMediaEscapesAndTilde(t *testing.T) {
	html, media, err := transformGDBody(`[s]best\ man.wav[/s] [s]~.wav[/s] [s]^~\ file.wav[/s]`, "Give", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(media, []string{"best man.wav", "Give.wav", "give file.wav"}) {
		t.Fatal(media)
	}
	if !strings.Contains(html, `href="best man.wav"`) || strings.Contains(html, `best\`) {
		t.Fatal(html)
	}
}

func TestGDHeadingVariants(t *testing.T) {
	if got := gdTitle(`a(b(c))`).Keys; !reflect.DeepEqual(got, []string{"abc", "ab", "a"}) {
		t.Fatal(got)
	}
	if got := gdTitle(`(a)(b)(c)(d)(e)(f)z`).Keys; len(got) != 32 {
		t.Fatalf("keys: %d", len(got))
	}
	if got := gdTitle(`a{[c red]x[/c]}(b)`).Keys; !reflect.DeepEqual(got, []string{"ab", "a"}) {
		t.Fatal(got)
	}
}

func TestGDCaseInvertedHeadings(t *testing.T) {
	for _, c := range []struct{ input, parent, want string }{
		{"^~ up", "Give", "give up"},
		{"^~ up", "give", "Give up"},
		{`\^~`, "Give", "^Give"},
		{`^\~`, "Give", "^~"},
		{"^~", "", ""},
		{"~", "a(b)", "a(b)"},
	} {
		got := gdTitle(expandGDTitleTilde(c.input, c.parent)).first()
		if got != c.want {
			t.Errorf("%q with %q: got %q want %q", c.input, c.parent, got, c.want)
		}
	}
	r := &Reader{gd: true}
	main, sub, err := r.parseBlock([]string{"Give", "^~ up"}, []string{"\ttext", "\t@ ^~ away", "\tchild", "\t@"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(main.Headwords, []string{"Give", "give up"}) || len(sub) != 1 || sub[0].Headwords[0] != "give away" {
		t.Fatal(main.Headwords, sub)
	}
}

func TestGDEmptyClosedTags(t *testing.T) {
	for _, text := range []string{`[b][/b]`, `[b][i][/b][/i]`, `[m1][/m]`} {
		p := gdParser{}
		if err := p.parse(text); err != nil {
			t.Fatal(err)
		}
		if len(p.root.children) != 0 {
			t.Fatalf("%s retained empty nodes", text)
		}
		html, _, err := transformGDBody(text, "key", nil)
		if err != nil || html != "" {
			t.Fatal(html, err)
		}
	}
}

func TestGDLanguageAttributeSyntax(t *testing.T) {
	for _, c := range []struct{ text, id string }{
		{`[lang id 2]text[/lang]`, ""},
		{`[lang id=2]text[/lang]`, "2"},
	} {
		p := gdParser{}
		if err := p.parse(c.text); err != nil {
			t.Fatal(err)
		}
		if got := p.root.children[0].attrs["id"]; got != c.id {
			t.Fatalf("%s: id=%q", c.text, got)
		}
	}
}

func TestGDReaderAlternateHeadings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.dsl")
	if err := os.WriteFile(p, []byte("#NAME \"Test\"\ngive\n~ up\n\t[b]~[/b]\n\t@ ~ away\n\t[i]child[/i]\n\t@\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := NewGDReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	main, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(main.Headwords, []string{"give", "give up"}) {
		t.Fatal(main.Headwords)
	}
	sub, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sub.Headwords, []string{"give away"}) {
		t.Fatal(sub.Headwords)
	}
	if r.Meta().Name != "Test GD" || r.Meta().Format != "dsl-gd" {
		t.Fatal(r.Meta())
	}
	legacy, err := NewReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	entry, err := legacy.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entry.Headwords, []string{"give", "~ up"}) {
		t.Fatal(entry.Headwords)
	}
}

func TestGDIndependentPreparation(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	dir := t.TempDir()
	p := filepath.Join(dir, "test.dsl")
	if err := os.WriteFile(p, []byte("#NAME \"Test\"\ngive\n~ up\n\t[ref]some [b]word[/b][/ref]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	comparison, err := dict.ComparisonSource(p)
	if err != nil {
		t.Fatal(err)
	}
	first, err := dict.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := dict.Open(comparison)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	a, ok := store.LookupDir(p)
	if !ok {
		t.Fatal("missing current index")
	}
	b, ok := store.LookupDir(comparison)
	if !ok || a == b {
		t.Fatal("shared index", a, b)
	}
	if first.Meta().Name != "Test" || second.Meta().Name != "Test GD" {
		t.Fatal(first.Meta(), second.Meta())
	}
	rc, _, err := second.Resource("image.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(data) != "asset" {
		t.Fatal(string(data), err)
	}
	before, err := os.Stat(comparison)
	if err != nil {
		t.Fatal(err)
	}
	again, err := dict.ComparisonSource(p)
	if err != nil || again != comparison {
		t.Fatal(again, err)
	}
	after, _ := os.Stat(comparison)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged descriptor rewritten")
	}
	if err := os.WriteFile(p, []byte("#NAME \"Test\"\ngive\n~ away\n\tchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := dict.ComparisonSource(p); err != nil {
		t.Fatal(err)
	}
	if !store.SourceChanged(store.TextDBPath(b), comparison) {
		t.Fatal("GD source edit not detected")
	}
	if !store.SourceChanged(store.TextDBPath(a), p) {
		t.Fatal("current source edit not detected")
	}
}

func TestGDNestingBound(t *testing.T) {
	_, _, err := transformGDBody(strings.Repeat("[b]", 258)+"x", "key", nil)
	if err == nil {
		t.Fatal("unbounded nesting")
	}
}

func TestGDReaderGolden(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	p := writeDSL(t, "mini.dsl", []byte(sampleDSL))
	comparison, err := gdComparison(p)
	if err != nil {
		t.Fatal(err)
	}
	goldentest.Check(t, "dsl-gd", comparison, goldentest.Golden{
		Versions: "reader=6 ingest=1 markup=2 fold=1",
		Hash:     "75103e7559a7977e011ea070bf1ce3e830e8d24399ec9b35c0e6310b703d0a9d",
	})
}

func TestGDLocalCorpus(t *testing.T) {
	dir := os.Getenv("WUDICT_DSL_CORPUS")
	if dir == "" {
		t.Skip("set WUDICT_DSL_CORPUS to check local dictionaries")
	}
	for _, name := range []string{"Oxford (En-Ru).dsl.dz", "Zimmerman (Ru-En).dsl.dz", "An Asperger dictionary of everyday expressions (En-En).dsl.dz"} {
		t.Run(name, func(t *testing.T) {
			r, err := NewGDReader(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			count := 0
			for {
				_, err := r.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				count++
			}
			t.Logf("%d entries parsed", count)
		})
	}
}
