// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"fmt"
	"strings"
	"testing"
	"time"

	nethtml "golang.org/x/net/html"
)

// TestTransformRepair pins how malformed markup renders (D165): tags are
// zones, the HTML is always well formed, and a broken construct costs no
// more than itself.
func TestTransformRepair(t *testing.T) {
	ab := &abbrevMap{
		exact: map[string]string{"общ.": "общее"},
		fold:  map[string]string{"общ.": "общее", "pl": "plural"},
		count: 2,
	}
	m1 := `<p class="wu-m" style="--wd-m:1">`
	m2 := `<p class="wu-m" style="--wd-m:2">`
	cases := []struct{ name, in, want string }{
		// The label's zone and the translation's overlap. The translation
		// lasts longer, so it goes outside, and nothing needs splitting.
		{"overlap, no split", "[m1]1) [p][trn]общ.[/p] разговор, речь[/trn][/m]",
			m1 + `1) <span class="wu-trn"><span class="wu-p"><abbr class="wu-abbr" title="общее">общ.</abbr></span> разговор, речь</span></p>`},
		{"overlap, formatting", "[b][i]x[/b]y[/i] z", `<i><b>x</b>y</i> z`},
		{"label inside a longer zone", "[p][i]pl[/p][/i]",
			`<i><span class="wu-p"><abbr class="wu-abbr" title="plural">pl</abbr></span></i>`},
		{"label around a zone it outlasts", "[lang id=1049][p]сущ.[/lang][/p]",
			`<span class="wu-p"><span class="wu-lang" data-lang="1049" lang="ru">сущ.</span></span>`},
		// A true crossing splits the later zone once.
		{"crossing", "[b]a[i]b[/b]c[/i]", `<b>a<i>b</i></b><i>c</i>`},
		{"stray closing tags", "a [/c]b[/m]c", `a bc`},
		{"open at end of line", "[i]a\nb", `<i>a</i><br/>b`},
		{"open at end of article", "a [c red]c", `a <span class="wu-c" style="--wd-c:red">c</span>`},
		{"open with nothing in it", "[c]\nx", `<br/>x`},
		{"written empty", "[c][/c][b][/b]", `<span class="wu-c"></span><b></b>`},
		{"zone across paragraphs", "[m1][c]x\n[m2]y[/c]",
			m1 + `<span class="wu-c">x</span></p>` + m2 + `<span class="wu-c">y</span></p>`},
		{"zone opened before a paragraph", "[*][m1]x[/m][/*]", m1 + `<span class="wu-sec">x</span></p>`},
		{"unknown, unpaired", "see [1] and [sic] here", `see [1] and [sic] here`},
		{"unknown, paired", "[x1]kept[/x1]", `kept`},
		{"known in the wrong case", "[B]x[/B] [I] see", `<b>x</b> [I] see`},
		{"failed tag swallows nothing", "a [ b [b]bold[/b]", `a [ b <b>bold</b>`},
		{"broken closing tag", "[/[m1]x[/m] [/] [/ y [/trn", m1 + `x</p> [/] [/ y [/trn`},
		{"tags inside a link", "[ref][i]word[/i][/ref]", `<a href="entry://word"><i>word</i></a>`},
		{"link inside a link", "[ref]a [ref]b[/ref] c[/ref]",
			`<a href="entry://a">a </a><a href="entry://b">b</a> c`},
		{"unterminated <<", "<<a b", `&lt;&lt;a b`},
		{"<< with a zone crossing it", "<<a [b]x>> y[/b]",
			`<a href="entry://a x">a <b>x</b></a><b> y</b>`},
		{"link text is verbatim", `[ref]~ ^a\[[/ref]`, `<a href="entry://K ^a[">K ^a[</a>`},
		{"~ and ^~ in links", "<<~ly>> [ref]^~[/ref] [ref]\\~[/ref]", `<a href="entry://Kly">Kly</a> <a href="entry://k">k</a> <a href="entry://~">~</a>`},
		{"audio inside a link", "[ref]a [s]x.wav[/s] b[/ref]",
			`<a href="entry://a b">a </a><a class="wu-audio" href="x.wav">&#128266;</a><a href="entry://a b"> b</a>`},
		{"Noah's Ark", "[i][i][i][i]x", `<i><i><i>x</i></i></i>`},
		// Legacy-font transcriptions only.
		{"legacy IPA, sentinel", "[t]k‡t[/t]", `<span class="wu-ipa">kæt</span>`},
		{"legacy IPA, mixed script", "[t]Џэp[/t]", `<span class="wu-ipa">ʃɪp</span>`},
		{"pinyin untouched", "[t]ma3[/t]", `<span class="wu-ipa">ma3</span>`},
		{"Cyrillic transcription untouched", "[t]кэт[/t]", `<span class="wu-ipa">кэт</span>`},
		{"Unicode transcription with a low quote untouched", "[t]„ʃɪp“[/t]", `<span class="wu-ipa">„ʃɪp“</span>`},
		{"Cyrillic note in a transcription untouched", "[t]tʃɪp, брит.[/t]", `<span class="wu-ipa">tʃɪp, брит.</span>`},
		// The lexer.
		{"escaped line break is no link text", "[ref]a\\\n  b[/ref] c", `<a href="entry://a b">a<br/>b</a> c`},
		{"escaped line break: no << across lines", "<<a\\\nb>> c", `&lt;&lt;a&nbsp;<br/>b&gt;&gt; c`},
		{"link target, blanks collapsed", "[ref] a  b\\ [/ref] [ref target=\" x \"]y[/ref]", `<a href="entry://a b"> a  b </a> <a href="entry://x">y</a>`},
		{"[<<] is an unknown tag", "[<<]x[/<<] [<<]y", `x [&lt;&lt;]y`},
		{"[/<<] closes no <<", "<<a [/<<]b>>", `<a href="entry://a b">a b</a>`},
		{"a link opened in a link takes over its verbatim text", "[ref]a[url]b[/url] ^a[/ref]", `<a href="entry://a">a</a><a href="http://b">b</a> A`},
		{"[m...] that is no paragraph keeps its line break", "foo\n[me] bar", `foo<br/>[me] bar`},
	}
	for _, c := range cases {
		got, _, err := transformBodyAbbrev(c.in, "K", ab)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: transformBody(%q)\n got %s\nwant %s", c.name, c.in, got, c.want)
		}
		if err := checkWellFormed(got); err != nil {
			t.Errorf("%s: %v in %s", c.name, err, got)
		}
	}
}

// voidElements are the elements this package emits without a closing tag.
var voidElements = map[string]bool{"br": true, "img": true}

var inlineElements = map[string]bool{
	"span": true, "b": true, "i": true, "u": true, "sup": true, "sub": true, "a": true, "abbr": true,
}

// checkWellFormed is invariant I2: every element closes in order, no
// paragraph sits inside an inline element, no link inside a link.
func checkWellFormed(s string) error {
	z := nethtml.NewTokenizer(strings.NewReader(s))
	var stack []string
	for {
		switch z.Next() {
		case nethtml.ErrorToken:
			if len(stack) > 0 {
				return fmt.Errorf("unclosed %v", stack)
			}
			return nil
		case nethtml.StartTagToken:
			name, _ := z.TagName()
			n := string(name)
			if voidElements[n] {
				continue
			}
			for _, open := range stack {
				if n == "p" && inlineElements[open] {
					return fmt.Errorf("<p> inside <%s>", open)
				}
				if n == "a" && open == "a" {
					return fmt.Errorf("<a> inside <a>")
				}
			}
			stack = append(stack, n)
		case nethtml.EndTagToken:
			name, _ := z.TagName()
			if len(stack) == 0 || stack[len(stack)-1] != string(name) {
				return fmt.Errorf("</%s> closes %v", name, stack)
			}
			stack = stack[:len(stack)-1]
		}
	}
}

// visibleText is what a reader sees of an HTML fragment.
func visibleText(s string) string {
	z := nethtml.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	for {
		switch z.Next() {
		case nethtml.ErrorToken:
			return b.String()
		case nethtml.TextToken:
			b.Write(z.Text()) // already unescaped by the tokenizer
		}
	}
}

// flatContent is the content of an article with every zone ignored: what
// emit must write exactly once, in order, whatever the nesting.
func flatContent(text, key string) string {
	tr := new(transformer)
	tr.reset(stripComments(text), key, nil)
	tr.lex()
	tr.resolve()
	var b strings.Builder
	for _, t := range tr.toks {
		switch t.kind {
		case tkText:
			s := tr.input[t.a:t.b]
			if t.ipa {
				s = legacyIPA(s)
			}
			b.WriteString(escape(s))
		case tkLit:
			b.WriteString(escape(t.s))
		case tkHTML, tkBreak:
			b.WriteString(t.s)
		}
	}
	return b.String()
}

// FuzzTransformBody holds invariants I2 (well formed) and I3 (content kept,
// in order, once) for any input.
func FuzzTransformBody(f *testing.F) {
	for _, s := range []string{
		"[m1]1) [p][trn]общ.[/p] разговор[/trn][/m]",
		"[b][i]x[/b]y[/i]", "[ref]a [ref]b[/ref] c[/ref]", "<<a [b]x>> y[/b]",
		"[m1][c]x\n\t[m2]y[/c]", "[ref]a [s]x.wav[/s] b[/ref]", "[t]Џэp[/t] [sic] [1]",
		"[i][i][i][i]x[/i]", "[/[m1]x [ b [b]y", "[c red]a\\ b\\\nc~^d[/c]", "[s]x.pdf[/s][url]a[/url]",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, _, err := transformFragment(in, "K~[b]")
		if err != nil {
			t.Fatal(err)
		}
		if err := checkWellFormed(got); err != nil {
			t.Fatalf("I2: %v\nin:  %q\nout: %s", err, in, got)
		}
		if a, b := visibleText(got), visibleText(flatContent(in, "K~[b]")); a != b {
			t.Fatalf("I3: content changed\nin:   %q\nout:  %q\nflat: %q", in, a, b)
		}
	})
}

// TestTransformLinear is invariant I4: hostile input costs time in
// proportion to its size.
func TestTransformLinear(t *testing.T) {
	const n = 20000
	for name, in := range map[string]string{
		"openers": strings.Repeat("[b]x", n),
		"distinct names": func() string {
			var b strings.Builder
			for i := range n {
				fmt.Fprintf(&b, "[x%d]y", i)
			}
			return b.String()
		}(),
		"closers":         strings.Repeat("[b]", 100) + strings.Repeat("x[/b]", n),
		"brackets":        strings.Repeat("[", n),
		"angle brackets":  strings.Repeat("<<", n),
		"links":           strings.Repeat("[ref]a", n),
		"crossing":        strings.Repeat("[b]a[i]b[/b]c[/i]", n/4),
		"paragraphs":      strings.Repeat("[m1][c][i]x\n", n/4),
		"labels":          strings.Repeat("[p]a", n),
		"unknown, nested": strings.Repeat("[sic]", n) + strings.Repeat("[/sic]", n),
	} {
		start := time.Now()
		got, _, _ := transformBody(in, "K")
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("%s: %v for %d bytes", name, d, len(in))
		}
		if err := checkWellFormed(got); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func BenchmarkTransformBody(b *testing.B) {
	art := "[b]I[/b] [p]союз[/p]\n" +
		"\t[m1]1) [p][trn]противит.[/p] а, [p]реже[/p] но; [com]([i]между предложениями[/i])[/com] же[/trn][/m]\n" +
		"\t[m2][*][ex][lang id=1049]собаки гавкают, а караван идёт[/lang] — the dogs bark[/ex][/*][/m]\n" +
		"\t[m1]2) [trn]and; [ref]also[/ref], <<as well>>[/trn] [s]a.wav[/s][/m]\n"
	b.SetBytes(int64(len(art)))
	for b.Loop() {
		transformBody(art, "а")
	}
}
