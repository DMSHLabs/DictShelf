// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/htmlref"
)

func renderMD(md string) string {
	var b bytes.Buffer
	src := []byte(md)
	if err := mdRenderer.Render(&b, src, mdParser.Parse(src)); err != nil {
		panic(err)
	}
	return b.String()
}

// TestClean: R6.6-R6.7, each case as written and as the stock parser reads it
// back.
var cleanCases = []struct{ name, html, md, back string }{
	{"spec §10 run", `<span class="pos">v.</span> <b>1</b> to move quickly on foot <a href="bword://walk">walk</a>`,
		"v. **1** to move quickly on foot [walk](entry://walk)",
		"<p>v. <strong>1</strong> to move quickly on foot <a href=\"entry://walk\">walk</a></p>\n"},
	{"spec §10 C#: h2 demoted, list lookalike escaped", `<p>A language.</p><h2>History</h2><p>2000.</p>`,
		"A language.\n\n### History\n\n2000\\.",
		"<p>A language.</p>\n<h3>History</h3>\n<p>2000.</p>\n"},
	{"divs are paragraphs; emphasis that cannot bind stays a tag",
		`<div class="sense"><b>1</b> <i>(of a person)</i> go fast</div><div class="ex">he <b>ran</b>!</div>`,
		"**1** <em>(of a person)</em> go fast\n\nhe **ran**!",
		"<p><strong>1</strong> <em>(of a person)</em> go fast</p>\n<p>he <strong>ran</strong>!</p>\n"},
	{"tight lists; a list after a list alternates", `<ul><li>a</li><li>b<ul><li>c</li></ul></li></ul><ul><li>d</li></ul>`,
		"- a\n- b\n  - c\n\n* d",
		"<ul>\n<li>a</li>\n<li>b\n<ul>\n<li>c</li>\n</ul>\n</li>\n</ul>\n<ul>\n<li>d</li>\n</ul>\n"},
	{"loose ordered list with start", `<ol start="3"><li><p>x</p><p>y</p></li><li>z</li></ol>`,
		"3. x\n\n   y\n\n4. z",
		"<ol start=\"3\">\n<li>\n<p>x</p>\n<p>y</p>\n</li>\n<li>\n<p>z</p>\n</li>\n</ol>\n"},
	{"table: pipes, flattened cell, alignment",
		`<table><tr><th align="center">a|b</th><th>c</th></tr><tr><td><p>1</p><p>2</p></td><td><a href="x|y">l</a></td></tr></table>`,
		"| a\\|b | c |\n| :-: | --- |\n| 1<br>2 | [l](entry://x%7Cy) |", ""},
	{"code block keeps a ## line", "<pre><code class=\"language-go\">## not a heading\nx := 1</code></pre>",
		"```go\n## not a heading\nx := 1\n```",
		"<pre><code class=\"language-go\">## not a heading\nx := 1\n</code></pre>\n"},
	{"inline tags without markdown", `bank<sup>1</sup> H<sub>2</sub>O <s>old</s> <u>u</u>`,
		"bank<sup>1</sup> H<sub>2</sub>O <del>old</del> <u>u</u>",
		"<p>bank<sup>1</sup> H<sub>2</sub>O <del>old</del> <u>u</u></p>\n"},
	{"details holds markdown", `<details><summary>More &amp; more</summary><p>hidden <b>x</b></p></details>`,
		"<details>\n<summary>More &amp; more</summary>\n\nhidden **x**\n\n</details>",
		"<details>\n<summary>More &amp; more</summary>\n<p>hidden <strong>x</strong></p>\n</details>"},
	{"a body that reads like a redirect", `see: run`, "see\\: run", "<p>see: run</p>\n"},
	{"escapes", `# hash 1. one - dash > quote * star _ und | pipe ~ tilde &amp; amp [br] \ bs`,
		"\\# hash 1. one - dash \\> quote \\* star \\_ und \\| pipe \\~ tilde \\& amp \\[br\\] \\\\ bs",
		"<p># hash 1. one - dash &gt; quote * star _ und | pipe ~ tilde &amp; amp [br] \\ bs</p>\n"},
	{"line-start escapes after a hard break", `a<br>- b<br># c<br>+ d<br>= e<br>12) f`,
		"a\\\n\\- b\\\n\\# c\\\n\\+ d\\\n\\= e\\\n12\\) f", ""},
	{"breaks: doubled and trailing ones go", `a<br>b<br><br>c<br>`, "a\\\nb\\\nc", "<p>a<br>\nb<br>\nc</p>\n"},
	{"blockquote", `<blockquote><p>q1</p><p>q2</p></blockquote>`, "> q1\n>\n> q2",
		"<blockquote>\n<p>q1</p>\n<p>q2</p>\n</blockquote>\n"},
	{"adjacent and nested emphasis", `<b>bold <i>both</i></b> <i>(paren)</i> <b>x</b><b>y</b>`,
		"<strong>bold *both*</strong> <em>(paren)</em> **x**<strong>y</strong>",
		"<p><strong>bold <em>both</em></strong> <em>(paren)</em> <strong>x</strong><strong>y</strong></p>\n"},
	{"image, bang before a link, media",
		`<img src="a.png" alt="an [image]"> !<a href="entry://x">x</a> <audio src="sound://a.mp3"></audio>`,
		"![an \\[image\\]](a.png) \\![x](entry://x) [▶](a.mp3)",
		"<p><img src=\"a.png\" alt=\"an [image]\"> !<a href=\"entry://x\">x</a> <a href=\"a.mp3\">▶</a></p>\n"},
	{"h1 demoted; a closing-sequence lookalike escaped", `<h1>Big</h1><h4>Small #</h4>`,
		"### Big\n\n#### Small \\#", "<h3>Big</h3>\n<h4>Small #</h4>\n"},
	{"script, style and comments go", `<script>alert(1)</script><style>p{}</style>text<!-- c -->`, "text", "<p>text</p>\n"},
	{"a wrapper around a block is laid out as its blocks (block-in-inline)", `<span>x<div>block in span</div></span>after`,
		"x\n\nblock in span\n\nafter", "<p>x</p>\n<p>block in span</p>\n<p>after</p>\n"},
	{"a block inside markup breaks the line", `<b>x<div>block in bold</div></b>after`,
		"**x\\\nblock in bold**after", "<p><strong>x<br>\nblock in bold</strong>after</p>\n"},
	{"spaced destination, title, lookup respelled", `<a href="bword://long run" title='say "hi"'>x</a>`,
		`[x](<entry://long run> "say \"hi\"")`, "<p><a href=\"entry://long%20run\" title=\"say &quot;hi&quot;\">x</a></p>\n"},
	{"code span with backticks", "<code>a`b</code> <code>`x</code>", "``a`b`` `` `x ``",
		"<p><code>a`b</code> <code>`x</code></p>\n"},
	{"empty body", `<div> </div><!-- only a comment -->`, "", ""},
	{"kbd is kept as inline HTML", `Press <kbd>☰</kbd> then <kbd class="k">Rescan *all*</kbd>`,
		"Press <kbd>☰</kbd> then <kbd>Rescan \\*all\\*</kbd>",
		"<p>Press <kbd>☰</kbd> then <kbd>Rescan *all*</kbd></p>\n"},
	{"space at the edges of markup goes outside it",
		`a<b> x </b>y <a href="entry://upset"> at upset</a>, z<sup> 2</sup> q<b> </b>r a<a href="entry://b"><i> b</i></a>`,
		"a **x** y [at upset](entry://upset), z <sup>2</sup> q r a [*b*](entry://b)",
		"<p>a <strong>x</strong> y <a href=\"entry://upset\">at upset</a>, z <sup>2</sup> q r a <a href=\"entry://b\"><em>b</em></a></p>\n"},
	{"a relative href that names no file is a headword (R6.9)",
		`<a href="cooking apple#e67_cooking">c</a> <a href="apple%20pie">p</a> <a href="a.MP3?v=2">s</a> <a href="#top">t</a> <a href="%20#x">w</a> <a href="/browse?dict=x">b</a> <a href="Res/y">r</a>`,
		"[c](<entry://cooking apple#e67_cooking>) [p](<entry://apple pie>) [s](a.MP3?v=2) [t](#top) [w](< #x>) [b](/browse?dict=x) [r](Res/y)",
		"<p><a href=\"entry://cooking%20apple#e67_cooking\">c</a> <a href=\"entry://apple%20pie\">p</a> <a href=\"a.MP3?v=2\">s</a> <a href=\"#top\">t</a> <a href=\"%20#x\">w</a> <a href=\"/browse?dict=x\">b</a> <a href=\"Res/y\">r</a></p>\n"},
}

// styledCases convert with a display table (R6.6): what the dictionary's
// stylesheet hides is dropped, and what it lays out as a block is set off.
var styledStyles = htmlref.ParseCSS(`.blk { display: block } .inl { display: inline } .hid { display: none }
	span.gap { margin-right: 3px }`, nil)

var styledCases = []struct{ name, html, md string }{
	{"block spans inside an inline one split it",
		`<span class="e"><span class="blk">apple</span><span>ap‧ple</span><span class="hid">FOOD</span> <span class="inl">noun</span></span>`,
		"apple\n\nap‧ple noun"},
	{"block spans inside markup break the line", `<i>a<span class="blk">b</span>c</i>`, "*a\\\nb\\\nc*"},
	{"block spans at the top are paragraphs; a block <b> keeps its markup",
		`<span class="blk">one</span><span class="blk"><b class="blk">he ate</b> it</span>`,
		"one\n\n**he ate**\n\nit"},
	{"the highest of an element's classes wins", `<span class="inl blk">a</span><span class="blk">b</span><span class="hid inl">c</span>`,
		"a\n\nb\n\nc"},
	{"hidden items, rows and cells", `<ul><li class="hid">x</li><li>y</li></ul><table><tr><th>h</th><th class="hid">k</th></tr><tr class="hid"><td>z</td></tr></table>`,
		"- y\n\n| h | |\n| --- | --- |"},
	{"a block inside a heading is a space", `<h3>a<span class="blk">b</span>c</h3>`, "### a b c"},
	{"what the stylesheet sets apart is set apart by a space",
		`<span class="gap">2</span><span>be the apple</span> x<a href="entry://y"><span class="gap">1</span>y</a>`,
		"2 be the apple x [1 y](entry://y)"},
}

func TestCleanStyles(t *testing.T) {
	for _, tc := range styledCases {
		t.Run(tc.name, func(t *testing.T) {
			md, err := cleanBody(tc.html, styledStyles)
			if err != nil {
				t.Fatal(err)
			}
			if md != tc.md {
				t.Errorf("md:\n got %q\nwant %q", md, tc.md)
			}
		})
	}
	// Without a table the same markup is one run of text.
	if md, _ := cleanBody(`<span class="blk">one</span><span class="blk"><b class="blk">he ate</b> it</span>`, nil); md != "one**he ate** it" {
		t.Errorf("no styles: %q", md)
	}
}

func TestClean(t *testing.T) {
	tests := cleanCases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md, err := cleanBody(tc.html, nil)
			if err != nil {
				t.Fatal(err)
			}
			if md != tc.md {
				t.Errorf("md:\n got %q\nwant %q", md, tc.md)
			}
			if tc.back != "" {
				if got := renderMD(md); got != tc.back {
					t.Errorf("read back:\n got %q\nwant %q", got, tc.back)
				}
			}
		})
	}
}

// TestHTMLMode: R6.5.
var htmlModeCases = []struct{ name, html, md string }{
	{"inline body is wrapped", `<span>x</span>`, "<div>\n<span>x</span>\n</div>"},
	{"a single block element is not", `<div class="e">x</div>`, `<div class="e">x</div>`},
	{"two blocks are wrapped", `<div>a</div><div>b</div>`, "<div>\n<div>a</div><div>b</div>\n</div>"},
	{"pre opens a type-1 block: wrapped", "<pre>x</pre>", "<div>\n<pre>x</pre>\n</div>"},
	{"a leading comment: wrapped", "<!-- c --><div>x</div>", "<div>\n<!-- c --><div>x</div>\n</div>"},
	{"blank lines: entity in pre, gone elsewhere",
		"<div>\n<pre>a\n\nb</pre>\n\n<p>c</p>\n<script>x\n\ny</script></div>",
		"<div>\n<pre>a\n&#10;b</pre>\n<p>c</p>\n<script>x\ny</script></div>"},
	{"CRLF and outer space", "\r\n  <div>a\r\n\r\nb</div>  \r\n", "<div>a\nb</div>"},
	{"lookup links respelled", `<a href="bword://@sub">s</a> <a href="BWORD:run">r</a>`,
		"<div>\n<a href=\"entry:@sub\">s</a> <a href=\"entry://run\">r</a>\n</div>"},
	{"empty", " \n ", ""},
}

func TestHTMLMode(t *testing.T) {
	tests := htmlModeCases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md := htmlBody(tc.html)
			if md != tc.md {
				t.Errorf("md:\n got %q\nwant %q", md, tc.md)
			}
			if md == "" {
				return
			}
			// One HTML block, rendered verbatim.
			doc := mdParser.Parse([]byte(md))
			if doc.ChildCount() != 1 || doc.FirstChild().Kind().String() != "HTMLBlock" {
				t.Errorf("not a single HTML block: %s", doc.FirstChild().Kind())
			}
			if got := renderMD(md); strings.TrimSuffix(got, "\n") != md {
				t.Errorf("render changed it:\n got %q", got)
			}
		})
	}
}

// writeDict writes entries with the Writer in mode and returns the file.
func writeDict(t *testing.T, h Head, mode Mode, add func(w *Writer)) string {
	t.Helper()
	w, err := NewWriter(h, mode, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	add(w)
	var b bytes.Buffer
	if _, err := w.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// specSources are the source articles of spec §10.
func specSources(w *Writer) {
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(w.Article([]string{"run", "runs"}, `<span class="pos">v.</span> <b>1</b> to move quickly on foot <a href="bword://walk">walk</a>`))
	w.Redirect([]string{"ran"}, "run")
	must(w.Article([]string{"C#"}, `<p>A language.</p><h2>History</h2><p>2000.</p>`))
}

// TestWriteSpecExamples: the writer turns the §10 sources into the example
// files, byte for byte, in both modes.
func TestWriteSpecExamples(t *testing.T) {
	for _, mode := range []Mode{ModeClean, ModeHTML} {
		t.Run(mode.String(), func(t *testing.T) {
			want, err := os.ReadFile("../../../docs/wudict-markdown/examples/" + mode.String() + Ext)
			if err != nil {
				t.Fatal(err)
			}
			if got := writeDict(t, Head{Name: "Sample"}, mode, specSources); got != string(want) {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// TestNamesRoundTrip: R6.2 - every name reads back as written, whatever it
// holds.
func TestNamesRoundTrip(t *testing.T) {
	names := []string{
		"C#", "#", "##", "a #", "C ##", "*", "**bold**", "_x_", "[x]", "a\\b", "\\", "<b>x</b>", "&amp;",
		"x~y", "`code`", "1.", "- dash", "> quote", "see: me", "@home", "100%", "a|b", "вода́", "漢字",
		"  spaced  ", "tab\there", "line\nbreak", "ctl\x01char", "bad\xffutf8", "ends with \\",
	}
	text := writeDict(t, Head{Name: "N"}, ModeClean, func(w *Writer) {
		for _, n := range names {
			if err := w.Article([]string{n}, "body"); err != nil {
				t.Fatal(err)
			}
		}
	})
	r, es := all(t, writeTemp(t, "n"+Ext, text))
	if w, _ := r.Warnings(); len(w) != 0 {
		t.Errorf("warnings: %q", w)
	}
	cw := &Writer{}
	if len(es) != len(names) {
		t.Fatalf("read %d entries, wrote %d:\n%s", len(es), len(names), text)
	}
	for i, n := range names {
		if want := cw.clean(n); es[i].Headwords[0] != want {
			t.Errorf("name %q read back as %q, want %q", n, es[i].Headwords[0], want)
		}
	}
}

// TestFold: R6.4.
func TestFold(t *testing.T) {
	text := writeDict(t, Head{Name: "F"}, ModeClean, func(w *Writer) {
		w.Redirect([]string{"banks"}, "bank") // before its target
		w.Article([]string{"bank"}, "river")
		w.Article([]string{"bank"}, "money") // homograph: both get it
		w.Article([]string{"Colour"}, "hue")
		w.Redirect([]string{"color"}, "colour") // case-folded
		w.Redirect([]string{"colr"}, "color")   // chain
		w.Redirect([]string{"loop1"}, "loop2")  // cycle
		w.Redirect([]string{"loop2"}, "loop1")  //
		w.Redirect([]string{"gone", "went"}, "Atlantis")
		w.Article([]string{"empty"}, "<p> </p>") // dropped
		w.Article(nil, "nameless")               // dropped
	})
	want := `# F
wudict: 1

## bank
## banks

river

## bank
## banks

money

## Colour
## color
## colr

hue

## loop1
see: loop2

## loop2
see: loop1

## gone
## went
see: Atlantis
`
	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}
}

// TestHeader: R6.1, R6.3.
func TestWriterHeader(t *testing.T) {
	text := writeDict(t, Head{
		Name: "", Stem: "My Dict #", From: "en-GB", To: "ru",
		Fields: []dict.Field{
			{Name: "Author", Value: "A. N. Other"},
			{Name: "Creation Date", Value: "2026\n09"},
			{Name: "wudict", Value: "x"},
			{Name: "2nd", Value: "y"},
			{Name: "作者", Value: "某人"},
			{Name: "meta", Value: "作者: 某人"},
			{Name: "empty", Value: " "},
		},
		Description: "<p>About <b>it</b>.</p>",
	}, ModeClean, func(w *Writer) {})
	want := `# My Dict \#
wudict: 1
from: en-GB
to: ru
author: A. N. Other
creation-date: 2026 09
x-wudict: x
x-2nd: y
meta: 作者: 某人
meta: 作者: 某人

About **it**.
`
	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}
	r, _ := all(t, writeTemp(t, "h"+Ext, text))
	if m := r.Meta(); m.Name != "My Dict #" || m.Description != "<p>About <strong>it</strong>.</p>\n" || len(m.Header) != 6 {
		t.Errorf("read back: %+v", m)
	}
}

// TestIdempotent: R7.3 - a file the writer wrote, read and written again in
// the same mode, is the same file.
func TestIdempotent(t *testing.T) {
	bodies := []string{
		`<span class="pos">v.</span> <b>1</b> to move quickly on foot <a href="bword://walk">walk</a>`,
		`<p>A language.</p><h2>History</h2><p>2000.</p>`,
		`<ul><li>a</li><li>b<ul><li>c</li></ul></li></ul><ul><li>d</li></ul><ol start="3"><li><p>x</p><p>y</p></li></ol>`,
		`<table><tr><th align="center">a|b</th><th>c</th></tr><tr><td><p>1</p><p>2</p></td><td><a href="x|y">l</a></td></tr></table>`,
		"<pre><code class=\"language-go\">## not a heading\n\nx := 1</code></pre><blockquote><p>q1</p><p>q2</p></blockquote>",
		`# hash 1. one - dash > * _ | ~ &amp; [br] \ bs<br>- b<br>12) f see: me`,
		`<b>bold <i>both</i></b> <i>(paren)</i> <b>x</b><b>y</b> <img src="a.png" alt="i"> !<a href="entry://x">x</a>`,
		`<details><summary>More</summary><p>hidden <b>x</b></p></details><h1>Big</h1>`,
		"<div>\n<pre>a\n\nb</pre>\n\n<script>x\n\ny</script></div>",
	}
	for _, mode := range []Mode{ModeClean, ModeHTML} {
		t.Run(mode.String(), func(t *testing.T) {
			first := writeDict(t, Head{Name: "I", From: "en", Description: "<p>d</p>"}, mode, func(w *Writer) {
				for i, b := range bodies {
					if err := w.Article([]string{"e" + string(rune('a'+i))}, b); err != nil {
						t.Fatal(err)
					}
				}
				w.Redirect([]string{"alias"}, "ea")
			})
			r, es := all(t, writeTemp(t, "i"+Ext, first))
			m := r.Meta()
			second := writeDict(t, Head{Name: m.Name, From: m.IndexLang, To: m.ContentsLang, Fields: m.Header, Description: m.Description}, mode, func(w *Writer) {
				for _, e := range es {
					if err := w.Article(e.Headwords, e.Body); err != nil {
						t.Fatal(err)
					}
				}
			})
			if first != second {
				t.Errorf("not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
		})
	}
}

// TestCleanRefusesSplit: a heading that would start an entry is an error, not
// a split entry (R6.8). No allowlisted HTML reaches it; the guard is on the
// markdown.
func TestCleanRefusesSplit(t *testing.T) {
	if err := splitsEntry("text\n\n## x\n"); err == nil {
		t.Error("an H2 passed")
	}
	if err := splitsEntry("x\n===\n"); err == nil {
		t.Error("a setext H1 passed")
	}
	if err := splitsEntry("### x\n\n```\n## y\n```\n"); err != nil {
		t.Errorf("an H3 and a fenced ## were refused: %v", err)
	}
	var ce *CleanError
	if !errors.As(&CleanError{Construct: "c", Reason: "r"}, &ce) {
		t.Error("CleanError does not match")
	}
}

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeHTML, "clean": ModeClean, "HTML": ModeHTML} {
		if m, err := ParseMode(in); err != nil || m != want {
			t.Errorf("ParseMode(%q) = %v, %v", in, m, err)
		}
	}
	if _, err := ParseMode("raw"); err == nil {
		t.Error("raw accepted")
	}
}

// FuzzClean: no HTML panics the converter or yields markdown that would split
// the entry, and converting the stock rendering of the result again gives the
// same markdown.
var cleanSeeds = []string{
	`<b>x</b> <i>y</i> <a href="entry://z">z</a>`, `<ul><li>a<ol><li>b</li></ol></li></ul>`,
	`<table><tr><td>a|b</td></tr></table>`, "<pre>## x\n\ny</pre>", `<h2>h</h2>#<br>- x`,
	`<details><summary>s</summary>x</details>`, "1) x<br>2. y", `<em>*</em><strong>**</strong>`,
	// Inputs the fuzzer found, each a rule the converter now keeps
	// (testdata/ is not committed, so they live here):
	"00<A0>00)0",                    // list-marker escape decided on the whole line
	"<A>#0000000",                   // an anchor without href is a wrapper, line start and all
	"<A href=0\">",                  // destinations in the renderer's canonical form
	"<A href=0\xdf>",                // invalid UTF-8 repaired before anything else
	"<ol><A>",                       // a stray node that renders nothing makes no item
	"<ul 000>0<ol><li>",             // an empty item is left out
	"<ul><ol><li></ol >0",           // likewise, before a following paragraph
	"<ol><ol>0</ol><ol>0",           // lists in one item alternate their markers
	"<ul>0</ul><li><ul>0</ul><ul>0", // alternation by the actual marker, across unwrapped flows
	"\\<Br>0",                       // a break after a text backslash is a tag
	"<A>0<dl>:-",                    // `:` at a line start could open a table delimiter row
	"<i>0<i>0</i> 0",                // nested emphasis keeps the outer one a tag
	"<tABle><tt>`<tr>0",             // a code span right after a backtick is a tag
}

func FuzzClean(f *testing.F) {
	for _, s := range cleanSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, h string) {
		md, err := cleanBody(h, nil)
		if err != nil {
			var ce *CleanError
			if !errors.As(err, &ce) || strings.HasPrefix(ce.Reason, "internal") {
				t.Fatalf("%q: %v", h, err)
			}
			return
		}
		if md == "" {
			return
		}
		md2, err := cleanBody(renderMD(md), nil)
		if err != nil || md2 != md {
			t.Fatalf("not a fixed point:\nhtml %q\nmd   %q\nback %q\nmd2  %q (%v)", h, md, renderMD(md), md2, err)
		}
	})
}
