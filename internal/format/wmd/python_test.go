// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/htmlref"
)

// The pyglossary plugin (wudict_md) ports this package to Python, and its
// wumd.py must write byte for byte what this package writes, and read the same
// entries. This runs it over every input the Go tests use: the clean and html
// tables, the fuzz seeds, the spec's examples and the reader's test documents.
//
// The plugin is not part of this repository, so the check is opt-in:
// WMD_PY_PORT names the plugin's directory (the one holding conformance.py).
// Unset, or without python3 and markdown-it-py, it skips.
//
// WMD_PY_CORPUS may name a WuWeiDict markdown file whose bodies are added as
// inputs - a dump of a real dictionary is the realistic test - and
// WMD_PY_CORPUS_MAX caps how many.

type pyBody struct {
	HTML     string `json:"html"`
	Clean    string `json:"clean"`
	CleanErr bool   `json:"cleanErr"`
	Raw      string `json:"raw"`
	// The display table the body is cleaned with, class → htmlref.Display.
	Styles map[string]int `json:"styles,omitempty"`
}

type pyDoc struct {
	Text  string     `json:"text"`
	Names [][]string `json:"names"`
}

func TestPythonPort(t *testing.T) {
	port := os.Getenv("WMD_PY_PORT")
	if port == "" {
		t.Skip("WMD_PY_PORT not set: the Python port is not in this repository")
	}
	script := filepath.Join(port, "conformance.py")
	if _, err := os.Stat(script); err != nil {
		// Set but wrong is a mistake in the run, not an absent optional tool.
		t.Fatalf("WMD_PY_PORT: %v", err)
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	if exec.Command(py, "-c", "import markdown_it").Run() != nil {
		t.Skip("markdown-it-py not installed")
	}

	var htmls []string
	for _, c := range cleanCases {
		htmls = append(htmls, c.html)
	}
	for _, c := range htmlModeCases {
		htmls = append(htmls, c.html)
	}
	htmls = append(htmls, cleanSeeds...)
	htmls = append(htmls,
		`<span class="pos">v.</span> <b>1</b> to move quickly on foot <a href="bword://walk">walk</a>`,
		`<p>A language.</p><h2>History</h2><p>2000.</p>`,
		`<div class="entry"><span class="hw">run</span> <span class="pos">v.</span><ol><li><span class="def">to move fast</span> <span class="ex">he <b>ran</b> home</span></li><li>to manage</li></ol></div>`,
		`<table><tr><td>a</td></tr></table> stray <td>cell</td> <tr>row</tr>`,
		`<p>one<p>two<div>three</div>four</p>five`,
		`<ul><li>a<li>b<ul><li>c</ul></ul><dl><dt>t<dd>d<dt>u</dl>`,
		`<a href="entry://%D0%B2%D0%BE%D0%B4%D0%B0">вода</a> <a href="x:@sub">s</a> <a href="D:word">w</a>`,
		`<xmp><b>raw</b></xmp><textarea>t</textarea><noscript>n</noscript><svg><text>s</text></svg>after`,
		"<pre>\n\na\n\n</pre><div>x\n\n\ny</div>",
		`<a href="bword://x" title='a"b' data-x=1>x</a><img srcset="bword://y 1x, z.png 2x" style="background:url( 'q.png' )">`,
	)
	if p := os.Getenv("WMD_PY_CORPUS"); p != "" {
		r, err := NewReader(p)
		if err != nil {
			t.Fatal(err)
		}
		limit, _ := strconv.Atoi(os.Getenv("WMD_PY_CORPUS_MAX"))
		for n := 0; limit <= 0 || n < limit; n++ {
			e, err := r.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			htmls = append(htmls, e.Body)
		}
		r.Close()
	}
	var cases struct {
		Bodies []pyBody `json:"bodies"`
		Docs   []pyDoc  `json:"docs"`
	}
	add := func(h string, st htmlref.Styles) {
		b := pyBody{HTML: h, Raw: htmlBody(h)}
		md, err := cleanBody(h, st)
		b.Clean, b.CleanErr = md, err != nil
		for class, d := range st {
			if b.Styles == nil {
				b.Styles = map[string]int{}
			}
			b.Styles[class] = int(d)
		}
		cases.Bodies = append(cases.Bodies, b)
	}
	for _, h := range htmls {
		add(h, nil)
	}
	// Every body again with a display table, for the stylesheet rules.
	for _, c := range styledCases {
		htmls = append(htmls, c.html)
	}
	for _, h := range htmls {
		add(h, styledStyles)
	}

	var texts []string
	for _, mode := range []string{"clean", "html"} {
		b, err := os.ReadFile("../../../docs/wudict-markdown/examples/" + mode + Ext)
		if err != nil {
			t.Fatal(err)
		}
		texts = append(texts, string(b))
	}
	for _, text := range chunkDocs {
		if text != "" {
			texts = append(texts, text)
		}
	}
	for _, text := range texts {
		r, err := read(decode([]byte(text)))
		if err != nil {
			t.Fatal(err)
		}
		d := pyDoc{Text: text}
		for {
			e, err := r.Next()
			if err != nil {
				break
			}
			d.Names = append(d.Names, e.Headwords)
		}
		cases.Docs = append(cases.Docs, d)
	}

	in := filepath.Join(t.TempDir(), "cases.json")
	b, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, b, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(py, script, in).CombinedOutput()
	if err != nil {
		t.Fatalf("the Python port differs:\n%s", out)
	}
	t.Log(strings.TrimSpace(string(out)))
}
