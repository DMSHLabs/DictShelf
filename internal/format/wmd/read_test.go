// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

func writeTemp(t *testing.T, name, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// all reads every entry of path.
func all(t *testing.T, path string) (*Reader, []dict.Entry) {
	t.Helper()
	r, err := NewReader(path)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	var out []dict.Entry
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			return r, out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
}

func readText(t *testing.T, text string) (*Reader, []dict.Entry) {
	t.Helper()
	return all(t, writeTemp(t, "x.wudict.md", text))
}

// specExample returns the code block that follows `**`mode`:**` in spec §10.
func specExample(t *testing.T, mode string) string {
	t.Helper()
	b, err := os.ReadFile("../../../docs/WUDICT-MARKDOWN.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "**`"+mode+"`:**\n\n```\n")
	if i < 0 {
		t.Fatalf("spec §10: %s example not found", mode)
	}
	s = s[i+len("**`"+mode+"`:**\n\n```\n"):]
	return s[:strings.Index(s, "```\n")]
}

// TestExamples: the example files are spec §10 byte for byte, and import as
// the spec says.
func TestExamples(t *testing.T) {
	want := map[string][]dict.Entry{
		"clean": {
			{Headwords: []string{"run", "runs", "ran"}, Kind: dict.BodyHTML,
				Body: "<p>v. <strong>1</strong> to move quickly on foot <a href=\"entry://walk\">walk</a></p>\n"},
			{Headwords: []string{"C#"}, Kind: dict.BodyHTML,
				Body: "<p>A language.</p>\n<h3>History</h3>\n<p>2000.</p>\n"},
		},
		"html": {
			{Headwords: []string{"run", "runs", "ran"}, Kind: dict.BodyHTML,
				Body: "<div>\n<span class=\"pos\">v.</span> <b>1</b> to move quickly on foot <a href=\"entry://walk\">walk</a>\n</div>\n"},
			{Headwords: []string{"C#"}, Kind: dict.BodyHTML,
				Body: "<div>\n<p>A language.</p><h2>History</h2><p>2000.</p>\n</div>\n"},
		},
	}
	for mode, entries := range want {
		t.Run(mode, func(t *testing.T) {
			path := "../../../docs/wudict-markdown/examples/" + mode + Ext
			file, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if spec := specExample(t, mode); string(file) != spec {
				t.Errorf("%s differs from spec §10:\n got %q\nwant %q", path, file, spec)
			}
			r, got := all(t, path)
			if !reflect.DeepEqual(got, entries) {
				t.Errorf("entries:\n got %+v\nwant %+v", got, entries)
			}
			if m := r.Meta(); m.Name != "Sample" || m.Format != Format || m.EntryCount != 2 {
				t.Errorf("meta = %+v", m)
			}
			if w, n := r.Warnings(); len(w)+n != 0 {
				t.Errorf("warnings: %v", w)
			}
		})
	}
}

// TestHeader: R3.1-R3.2.
func TestHeader(t *testing.T) {
	fatal := []struct {
		name, text, code string
	}{
		{"empty file", "", "E-format"},
		{"no title", "Dict\nwudict: 1\n", "E-format"},
		{"empty title", "#\nwudict: 1\n", "E-format"},
		{"blank title", "#  \nwudict: 1\n", "E-format"},
		{"indented title", " # Dict\nwudict: 1\n", "E-format"},
		{"level-2 title", "## Dict\nwudict: 1\n", "E-format"},
		{"title only", "# Dict\n", "E-format"},
		{"other field on line 2", "# Dict\nfrom: en\nwudict: 1\n", "E-format"},
		{"blank line 2", "# Dict\n\nwudict: 1\n", "E-format"},
		{"setext trap", "# Dict\nwudict: 1\n===\n", "E-format"},
		{"markup-only title", "# <b></b>\nwudict: 1\n", "E-format"},
		{"near miss: case", "# Dict\nWudict: 1\n", "E-version"},
		{"near miss: no space", "# Dict\nwudict:1\n", "E-version"},
		{"near miss: space before colon", "# Dict\nwudict : 1\n", "E-version"},
		{"major 2", "# Dict\nwudict: 2\n", "E-version"},
		{"major 0", "# Dict\nwudict: 0\n", "E-version"},
		{"flag, not a version", "# Dict\nwudict: true\n", "E-version"},
		{"bad minor", "# Dict\nwudict: 1.x\n", "E-version"},
		{"trailing dot", "# Dict\nwudict: 1.\n", "E-version"},
	}
	for _, tc := range fatal {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewReader(writeTemp(t, "x.wudict.md", tc.text))
			var fe *Error
			if !errors.As(err, &fe) || fe.Code != tc.code {
				t.Errorf("err = %v, want %s", err, tc.code)
			}
		})
	}

	r, entries := readText(t, "# *Pocket* English\nwudict: 01.7\nfrom: en-GB\nto: ru\nauthor: A. N. Other\nfrom: de\nlicense: CC0\n\nThe **description**.\n")
	m := r.Meta()
	if m.Name != "Pocket English" || m.IndexLang != "en" || m.ContentsLang != "ru" || m.EntryCount != 0 || len(entries) != 0 {
		t.Errorf("meta = %+v, entries %v", m, entries)
	}
	if want := []dict.Field{{Name: "author", Value: "A. N. Other"}, {Name: "license", Value: "CC0"}}; !reflect.DeepEqual(m.Header, want) {
		t.Errorf("header = %v, want %v", m.Header, want)
	}
	if m.Description != "<p>The <strong>description</strong>.</p>\n" {
		t.Errorf("description = %q", m.Description)
	}

	r, _ = readText(t, "# D\nwudict: 1\nkey: ok\nnot a field\nlater: ignored\n")
	if w, _ := r.Warnings(); len(w) != 1 || len(r.Meta().Header) != 1 {
		t.Errorf("malformed header line: warnings %v, header %v", w, r.Meta().Header)
	}
	if r, err := NewReader(writeTemp(t, "x.wudict.md", "\xEF\xBB\xBF# D\r\nwudict: 1\r\n")); err != nil {
		t.Errorf("BOM + CRLF header: %v", err)
	} else {
		r.Close()
	}
}

func names(es []dict.Entry) [][]string {
	var out [][]string
	for _, e := range es {
		n := append([]string{}, e.Headwords...)
		if e.LinkTo != "" {
			n = append(n, "→"+e.LinkTo)
		}
		out = append(out, n)
	}
	return out
}

// TestEntries: R3.4-R3.7 on the stock syntax tree.
func TestEntries(t *testing.T) {
	const doc = `# D
wudict: 1

intro

## run
## runs

## ran
body of run

## C\# &amp; *em* ` + "`x y`" + `
[r]: entry://C%23

text [r]

## homograph
one

## homograph
two

## a
<!-- a comment is a block -->
## b
b body

## code

` + "```" + `
## not an entry
` + "```" + `

<div>
## not an entry either
</div>

Setext
------

- list

  ## still in the list? no: a heading at column 2 is inside the item

## see
## seen
see: run

## not redirect
see: run
more

## also article

see: run

para

##
## named after blank
x

## dup
## dup
y

## tail`
	r, es := readText(t, doc)
	want := [][]string{
		{"run", "runs", "ran", "see", "seen"},
		{"C# & em x y"},
		{"homograph"},
		{"homograph"},
		{"a"},
		{"b"},
		{"code"},
		{"not redirect"},
		{"also article"},
		{"named after blank"},
		{"dup"},
	}
	if got := names(es); !reflect.DeepEqual(got, want) {
		t.Errorf("entries:\n got %q\nwant %q", got, want)
	}
	if es[1].Body != "<p>text <a href=\"entry://C%23\">r</a></p>\n" {
		t.Errorf("C# body = %q", es[1].Body)
	}
	if es[4].Body != "<!-- a comment is a block -->\n" {
		t.Errorf("a body = %q", es[4].Body)
	}
	if !strings.Contains(es[6].Body, "<pre><code>## not an entry\n</code></pre>") ||
		!strings.Contains(es[6].Body, "<div>\n## not an entry either\n</div>") {
		t.Errorf("code body = %q", es[6].Body)
	}
	if !strings.Contains(es[6].Body, "<h2>Setext</h2>") || !strings.Contains(es[6].Body, "<li>") {
		t.Errorf("a setext heading is content: code body = %q", es[6].Body)
	}
	if m := r.Meta(); m.EntryCount != len(es) || m.Description != "<p>intro</p>\n" {
		t.Errorf("meta = %+v (entries %d)", m, len(es))
	}
	// The <div> line is content (R3.5) and is said to be: an entry line inside
	// an HTML block is nearly always one the block swallowed by accident.
	if w, _ := r.Warnings(); len(w) != 2 || !strings.Contains(w[0], `"## not an entry either" is inside the HTML block`) ||
		!strings.Contains(w[1], "without a body") {
		t.Errorf("warnings = %q, want the swallowed line and the trailing group", w)
	}
}

// TestSetextIsAnEntry: a setext level-2 heading at top level starts an entry,
// as a stock parser sees it (R3.5).
// A setext heading is content (R3.5): text over a `---` line neither starts
// an entry nor joins the heading group above it.
func TestSetextIsContent(t *testing.T) {
	_, es := readText(t, "# D\nwudict: 1\n\n## a\n\ntext\n---\n\nmore\n")
	if got := names(es); !reflect.DeepEqual(got, [][]string{{"a"}}) {
		t.Fatalf("entries = %q", got)
	}
	if want := "<h2>text</h2>\n<p>more</p>\n"; es[0].Body != want {
		t.Errorf("body = %q, want %q", es[0].Body, want)
	}
	if _, err := read([]byte("# D\nwudict: 1\nfrom: en\n---\n")); err == nil || !strings.Contains(err.Error(), "blank line") {
		t.Errorf("header under a --- line: %v", err)
	}
}

func TestDecode(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"\xEF\xBB\xBFbom", "bom"},
		{"a\r\nb\rc\n", "a\nb\nc\n"},
		{"n\x00ul", "n�ul"},
		{"bad\xffbyte", "bad�byte"},
		{"ok é \U0001F600", "ok é \U0001F600"},
	}
	for _, tt := range tests {
		if got := string(decode([]byte(tt.in))); got != tt.want {
			t.Errorf("decode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func gz(t *testing.T, parts ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, p := range parts {
		zw := NewGzipWriter(&b)
		if _, err := zw.Write([]byte(p)); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return b.Bytes()
}

// NewGzipWriter is the test's gzip; the writer's own comes with the writer.
func NewGzipWriter(w io.Writer) *gzip.Writer { return gzip.NewWriter(w) }

// TestCompressed: R2.4.
func TestCompressed(t *testing.T) {
	const doc = "# D\nwudict: 1\n\n## w\n\nbody\n"
	half := len(doc) / 2
	for _, name := range []string{"x" + ExtGz, "x" + ExtDz, "X.WUDICT.MD.GZ"} {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, gz(t, doc[:half], doc[half:]), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, es := all(t, p); len(es) != 1 || es[0].Headwords[0] != "w" {
			t.Errorf("%s: %v", name, es)
		}
	}
	bad := filepath.Join(t.TempDir(), "bad"+ExtGz)
	if err := os.WriteFile(bad, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	var fe *Error
	if _, err := NewReader(bad); !errors.As(err, &fe) || fe.Code != "E-format" {
		t.Errorf("corrupt gzip: %v", err)
	}
	big := filepath.Join(t.TempDir(), "big"+ExtGz)
	if err := os.WriteFile(big, gz(t, doc+strings.Repeat("x", 4096)), 0o644); err != nil {
		t.Fatal(err)
	}
	saved := maxMarkdown
	maxMarkdown = 1024
	defer func() { maxMarkdown = saved }()
	if _, err := NewReader(big); !errors.As(err, &fe) || fe.Code != "E-format" {
		t.Errorf("over the bound: %v", err)
	}
}

// TestClaim: a file is claimed only by title + version field (R2.2),
// whatever it is called.
func TestClaim(t *testing.T) {
	tests := []struct {
		name, head string
		want       bool
	}{
		{"version field", "# Dict\nwudict: 1\n", true},
		{"minor version", "# Dict\nwudict: 1.3\n\n## word\n", true},
		{"no trailing LF", "# Dict\nwudict: 1", true},
		{"BOM", "\xEF\xBB\xBF# Dict\nwudict: 1\n", true},
		{"CRLF", "# Dict\r\nwudict: 1\r\n", true},
		{"CR only", "# Dict\rwudict: 1\r", true},
		{"near miss claims, E-version follows", "# Dict\nWudict:1\n", true},
		{"README with sections", "# My Project\n\nSome text.\n\n## Install\n", false},
		{"CHANGELOG", "# Changelog\n\n## [1.0.0] - 2026-01-01\n- x\n", false},
		{"entries without version", "# Dict\n\n## word\n\ndef\n", false},
		{"other field first", "# Dict\nfrom: en\nwudict: 1\n", false},
		{"key prefix only", "# Dict\nwudictionary: 1\n", false},
		{"no title", "Dict\nwudict: 1\n", false},
		{"level-2 first", "## word\nwudict: 1\n", false},
		{"title only", "# Notes\n", false},
		{"empty", "", false},
		{"title past the head", "# " + strings.Repeat("x", claimHead) + "\nwudict: 1\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			head := tt.head
			if len(head) > claimHead {
				head = head[:claimHead]
			}
			if err := claim([]byte(head)); (err == nil) != tt.want {
				t.Errorf("claim = %v, want claimed %v", err, tt.want)
			}
		})
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte("# Dict\nwudict: 1\n"))
	zw.Close()
	if err := Claim("d.wudict.md.gz", bytes.NewReader(gz.Bytes())); err != nil {
		t.Errorf("gzip: %v", err)
	}
	if err := Claim("d.wudict.md.gz", strings.NewReader("# Dict\nwudict: 1\n")); err == nil {
		t.Error("plain text claimed as gzip")
	}
	// The gate is the same for every spelling: a `.wudict.md` without the
	// field is not a dictionary, and its open says why.
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	for _, name := range []string{"n.wudict.md", "n.md"} {
		p := writeTemp(t, name, "# Dict\n\n## word\n\ndef\n")
		if dict.IsDictionaryFile(p) {
			t.Errorf("%s without the field is a dictionary", name)
		}
		if _, err := dict.Open(p); err == nil || !strings.Contains(err.Error(), "line 2 must be `wudict: 1`") {
			t.Errorf("%s: open = %v", name, err)
		}
	}
}

// TestPlainMarkdown: a qualifying `.md` is read like a `.wudict.md`, and the
// registry routes `.md` by content.
func TestPlainMarkdown(t *testing.T) {
	dictMD := writeTemp(t, "g.md", "# Dict\nwudict: 1\n\n## word\n\ndef\n")
	if _, es := all(t, dictMD); len(es) != 1 || es[0].Headwords[0] != "word" {
		t.Errorf("plain .md: %v", es)
	}
	// Closed, not discarded: an open reader keeps the file open, and Windows
	// refuses to delete an open file - t.TempDir's cleanup failed on it.
	if r, err := dict.OpenReader(dictMD); err != nil {
		t.Errorf("dict.OpenReader(.md) = %v", err)
	} else {
		r.Close()
	}
	readme := writeTemp(t, "README.md", "# Project\n\n## Install\n")
	if dict.IsDictionaryFile(readme) {
		t.Error("a README is a dictionary")
	}
}

// TestOpenPrepared: the first Open prepares a library folder, and a later Open
// serves from it without reading the source again (the folder keeps the
// title, description and header fields).
func TestOpenPrepared(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	src := writeTemp(t, "Glossary.wudict.md", "# Glossary\nwudict: 1\nfrom: en\nauthor: me\n\nAbout it.\n\n## run\n## runs\n\n*v.* to run\n\n## ran\nsee: run\n\n## gone\n## went\nsee: C# & more\n")
	d, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	check := func(d *Dict) {
		t.Helper()
		m := d.Meta()
		if m.Name != "Glossary" || m.Format != Format || m.Path != src || m.Description != "<p>About it.</p>\n" ||
			len(m.Header) != 1 || m.Header[0] != (dict.Field{Name: "author", Value: "me"}) {
			t.Errorf("meta = %+v", m)
		}
		for _, w := range []string{"run", "runs", "ran"} {
			res, err := d.Exact(w, 10)
			if err != nil || len(res) != 1 || !strings.Contains(res[0].Body, "<em>v.</em> to run") {
				t.Errorf("Lookup(%q) = %v, %+v", w, err, res)
			}
		}
	}
	check(d)
	// R3.6: a redirect to nothing in the dictionary is a lookup link, under
	// every name it has.
	for _, w := range []string{"gone", "went"} {
		res, err := d.Exact(w, 10)
		if err != nil || len(res) != 1 || !strings.Contains(res[0].Body, `href="entry://C%23 &amp; more"`) {
			t.Errorf("Exact(%q) = %v, %+v", w, err, res)
		}
	}
	d.Close()

	// Unreadable now: the prepared open must not need it.
	if err := os.Chmod(src, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(src, 0o644)
	if _, err := os.ReadFile(src); err == nil {
		t.Skip("running with privileges that ignore file modes")
	}
	d, err = Open(src)
	if err != nil {
		t.Fatalf("prepared Open read the source: %v", err)
	}
	check(d)
	d.Close()
}

// TestRedirectFolding: R3.6 - a redirect's names become aliases of every
// article its target names; exact before case-folded; no chains; a redirect
// that reaches nothing is a lookup link.
func TestRedirectFolding(t *testing.T) {
	_, es := readText(t, `# D
wudict: 1

## bank
river

## bank
money

## banks
see: bank

## Colour
hue

## color
see: colour

## step
see: banks

## nowhere
## never
see: Atlantis
`)
	want := [][]string{
		{"bank", "banks"},
		{"bank", "banks"},
		{"Colour", "color"},
		{"step"},
		{"nowhere", "never"},
	}
	if got := names(es); !reflect.DeepEqual(got, want) {
		t.Fatalf("entries:\n got %q\nwant %q", got, want)
	}
	for i, target := range map[int]string{3: "banks", 4: "Atlantis"} {
		if want := lookupLink(target); es[i].Body != want {
			t.Errorf("%v body = %q, want %q", es[i].Headwords, es[i].Body, want)
		}
	}
}

// TestConcurrentReaders: the parser and renderer are shared by every Reader.
func TestConcurrentReaders(t *testing.T) {
	path := "../../../docs/wudict-markdown/examples/clean" + Ext
	_, want := all(t, path)
	errs := make(chan string, 16)
	done := make(chan struct{})
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 20 {
				r, err := NewReader(path)
				if err != nil {
					errs <- err.Error()
					return
				}
				for i := 0; ; i++ {
					e, err := r.Next()
					if err != nil {
						break
					}
					if !reflect.DeepEqual(e, want[i]) {
						errs <- "entry differs under concurrency"
						return
					}
				}
			}
		}()
	}
	for range 8 {
		<-done
	}
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// FuzzRead: no input panics the reader, yields an entry without a name, or
// reads differently in the smallest chunks than as a whole.
func FuzzRead(f *testing.F) {
	for _, s := range []string{
		"# D\nwudict: 1\n\n## a\n\nb\n",
		"# D\nwudict: 1\nfrom: en\n\n## a\n## b\nsee: c\n",
		"# D\nwudict: 1\n\n## a\n\n```\n## b\n",
		"# D\nwudict: 1\n\n## a\n\n<div>\n\n## b\n",
		"# D\nwudict: 1\n\n## a\n\n| x |\n| - |\n| y |\n",
		"# D\r\nwudict: 1.0\r\n\r\n##\t\\#\r\n\r\n[r]: <entry://x>\r\n",
		"# D\nwudict: 1\n\n## a\n[x]\n## b\n\n[x]: y\n",
	} {
		f.Add([]byte(s))
	}
	saved := chunkSize
	defer func() { chunkSize = saved }()
	readAll := func(t *testing.T, data []byte, size int) ([]dict.Entry, dict.Meta, error) {
		chunkSize = size
		r, err := read(decode(data))
		if err != nil {
			return nil, dict.Meta{}, err
		}
		var es []dict.Entry
		for {
			e, err := r.Next()
			if err != nil {
				return es, r.Meta(), nil
			}
			if len(e.Headwords) == 0 || e.Headwords[0] == "" {
				t.Fatalf("nameless entry %+v", e)
			}
			es = append(es, e)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		e1, m1, err1 := readAll(t, data, 1<<30)
		e2, m2, err2 := readAll(t, data, 1)
		if (err1 == nil) != (err2 == nil) || !reflect.DeepEqual(e1, e2) || !reflect.DeepEqual(m1, m2) {
			t.Fatalf("chunked read differs:\nwhole %v %+v %+v\nchunk %v %+v %+v", err1, m1, e1, err2, m2, e2)
		}
	})
}

// chunkDocs puts at a potential cut every construct that could read
// differently there.
var chunkDocs = map[string]string{
	"spec examples": "", // filled from docs/wudict-markdown/examples in the test
	"blocks at cuts": `# D
wudict: 1

intro with [a forward reference][later]

## list
- a
- b

## loose list
1. a

2. b
## code
` + "```" + `
## inside a fence
## still inside
` + "```" + `
## html
<div>
## inside html
</div>

## quote
> lazy
continuation
## table
| a | b |
| - | - |
| 1 | 2 |
## setext
text

Setext heading
--------------
body
## group
## of
## three
body [later]
## tail
x

[later]: entry://later "defined last"
`,
	"unclosed fence swallows the rest": "# D\nwudict: 1\n\n## a\n\n```\n## b\n\n## c\n",
	"deep containers":                  "# D\nwudict: 1\n\n## a\n\n> - > - x\n>\n> ## inside\n## b\n\ny\n",
	"redirects across cuts":            "# D\nwudict: 1\n\n## x\nsee: y\n\n## y\n\nbody\n\n## z\n## Z2\nsee: Y\n",
}

// TestChunkingIsInvisible: reading with the smallest chunks gives exactly what
// reading the whole file at once gives.
func TestChunkingIsInvisible(t *testing.T) {
	for _, mode := range []string{"clean", "html"} {
		b, err := os.ReadFile("../../../docs/wudict-markdown/examples/" + mode + Ext)
		if err != nil {
			t.Fatal(err)
		}
		chunkDocs["spec example "+mode] = string(b)
	}
	delete(chunkDocs, "spec examples")
	saved := chunkSize
	defer func() { chunkSize = saved }()
	readAll := func(t *testing.T, text string, size int) (dict.Meta, []dict.Entry, []string, int) {
		t.Helper()
		chunkSize = size
		r, err := read(decode([]byte(text)))
		if err != nil {
			t.Fatal(err)
		}
		var es []dict.Entry
		for {
			e, err := r.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			es = append(es, e)
		}
		w, _ := r.Warnings()
		return r.Meta(), es, w, len(r.chunks)
	}
	for name, text := range chunkDocs {
		t.Run(name, func(t *testing.T) {
			m1, e1, w1, n1 := readAll(t, text, 1<<30)
			m2, e2, w2, n2 := readAll(t, text, 1)
			if n1 != 1 {
				t.Errorf("whole-file read used %d chunks", n1)
			}
			if n2 < 2 && strings.Count(text, "\n## ") > 1 {
				t.Errorf("smallest chunks: still %d chunk", n2)
			}
			for _, e := range e1 {
				if strings.Contains(e.Body, ">later<") && !strings.Contains(e.Body, `<a href="entry://later" title="defined last">later</a>`) {
					t.Errorf("forward reference resolved wrongly: %q", e.Body)
				}
			}
			if !reflect.DeepEqual(m1, m2) || !reflect.DeepEqual(e1, e2) || !reflect.DeepEqual(w1, w2) {
				t.Errorf("chunked read differs (%d chunks):\nwhole %+v\n      %+v\n      %q\nchunk %+v\n      %+v\n      %q", n2, m1, e1, w1, m2, e2, w2)
			}
		})
	}
}

// TestSwallowedHeadings: a line that looks like an entry heading but lies
// inside an HTML block is content, as CommonMark says, and the reader says so
// - where the entry went missing, and which line opened the block. The
// reader itself stays stock: the entries are what any CommonMark parser makes.
func TestSwallowedHeadings(t *testing.T) {
	const head = "# T\nwudict: 1\n\n"
	for _, tc := range []struct {
		name, body string
		names      []string // entries read
		warns      []string // each warning holds these, in order
	}{
		// a block of type 6 runs to the next blank line
		{"div without blank line", "## a\n\n<div>x</div>\n## b\n\ny\n",
			[]string{"a"}, []string{`7: "## b" is inside the HTML block that starts on line 6`}},
		// <script> runs to </script>, however many entries that takes
		{"script", "## a\n\n<script src=\"s.js\"/>\n## b\n\nx\n\n## c\n\ny\n",
			[]string{"a"}, []string{`7: "## b" is inside the HTML block that starts on line 6`, `11: "## c"`}},
		// goldmark's own start conditions, read as they are
		{"pre/", "## a\n\n<pre/>\n## b\n\nx\n", []string{"a"}, []string{`7: "## b" is inside the HTML block that starts on line 6`}},
		{"meta after a paragraph", "## a\n\np\n<meta x=1>\n## b\n\nx\n", []string{"a"}, []string{`8: "## b" is inside the HTML block that starts on line 7`}},
		// not swallowed: nothing to say
		{"closed by a blank line", "## a\n\n<div>x</div>\n\n## b\n\ny\n", []string{"a", "b"}, nil},
		{"closed by its tag", "## a\n\n<script>\nx\n</script>\n\n## b\n\ny\n", []string{"a", "b"}, nil},
		{"in a code block", "## a\n\n```\n## not an entry\n```\n\n## b\n\ny\n", []string{"a", "b"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{1 << 30, 1} { // whole, and cut as finely as possible
				saved := chunkSize
				chunkSize = size
				r, err := read([]byte(head + tc.body))
				chunkSize = saved
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for {
					e, err := r.Next()
					if err != nil {
						break
					}
					names = append(names, e.Headwords[0])
				}
				if !reflect.DeepEqual(names, tc.names) {
					t.Errorf("chunk %d: entries %q, want %q", size, names, tc.names)
				}
				w, _ := r.Warnings()
				if len(w) != len(tc.warns) {
					t.Fatalf("chunk %d: warnings %q, want %d", size, w, len(tc.warns))
				}
				for i, want := range tc.warns {
					if !strings.Contains(w[i], want) {
						t.Errorf("chunk %d: warning %q, want it to hold %q", size, w[i], want)
					}
				}
			}
		})
	}
}
