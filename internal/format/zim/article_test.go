// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import "testing"

func TestArticleBody(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			"strips the shell, hoists the stylesheet",
			`<!DOCTYPE html><html><head><title>t</title>` +
				`<link rel="stylesheet" href="a.css"><link rel="icon" href="f.png">` +
				`</head><body class="mw">text</body></html>`,
			`<link rel="stylesheet" href="a.css">text`,
		},
		{
			"several stylesheets keep their order",
			`<head><link rel="stylesheet" href="1.css"><LINK REL="stylesheet" HREF="2.css"></head><body>x</body>`,
			`<link rel="stylesheet" href="1.css"><LINK REL="stylesheet" HREF="2.css">x`,
		},
		{"uppercase tags", `<HTML><BODY>hi</BODY></HTML>`, `hi`},
		{"attributes on body", `<body data-x=">">y</body>`, `">y`},
		// No <body> at all: a devdocs or zimit fragment must survive whole
		// rather than be emptied.
		{"no body element", `<div>fragment</div>`, `<div>fragment</div>`},
		{"unclosed body", `<html><body>tail`, `tail`},
		{"empty", ``, ``},
		{"body-like text only", `just words`, `just words`},
		// "İ" lower-cases to the 1-byte "i" and the Kelvin sign to "k": an
		// offset found in a lower-cased copy cut the original early.
		{"letters whose lower case is shorter", `<head><title>İİK</title></head><body>gövde</body>`, `gövde`},
		{"a longer tag name is not body", `<bodyx>no</bodyx><body>yes</body>`, `yes`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := articleBody(tc.in); got != tc.want {
				t.Errorf("articleBody(%q)\n = %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// A cross-reference resolves against the article it is written in, as the
// browser resolves it against the page Kiwix serves at the article's path.
func TestLinkRef(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		ref   string
		newNS bool
		want  string
	}{
		{"sibling path", "x", "cat", true, "entry://cat"},
		{"percent-encoded", "x", "odrasl%C4%83", true, "entry://odraslă"},
		{"underscore is a space", "x", "New_York", true, "entry://New York"},
		{"leading ./", "x", "./cat", true, "entry://cat"},
		{"fragment is kept", "x", "cat#Etymology", true, "entry://cat#Etymology"},
		{"fragment only", "x", "#Etymology", true, "#Etymology"},
		{"absolute", "x", "https://example.org/x", true, "https://example.org/x"},
		{"protocol relative", "x", "//example.org/x", true, "//example.org/x"},
		{"mailto", "x", "mailto:a@b.c", true, "mailto:a@b.c"},
		{"data uri", "x", "data:text/plain,x", true, "data:text/plain,x"},
		{"query is a live reference", "x", "index.php?title=x", true, "index.php?title=x"},
		{"empty", "x", "", true, ""},
		// A stored file is named from the archive's root; .html is an article.
		{"stored file", "x", "_res_/a.css", true, "_res_/a.css"},
		{"stored file from a subdirectory", "a/b", "../doc.pdf", true, "doc.pdf"},
		{"zimwriterfs article", "Main_Page.html", "Other_Page.html", true, "entry://Other Page.html"},
		// A path in a subdirectory, as devdocs and wikibooks subpages write.
		{"devdocs sibling directory", "show-authors/index", "../extended-show/index", true, "entry://extended-show/index"},
		{"subpage", "Python", "Python/%D0%A7%D1%82%D0%BE_%D0%BD%D0%BE%D0%B2%D0%BE%D0%B3%D0%BE", true, "entry://Python/Что нового"},
		{"same directory in a subdirectory", "a/b", "c", true, "entry://a/c"},
		{"above the root stops at it", "a", "../../x", true, "entry://x"},
		{"a namespace written into a 6.1 file", "x", "../C/cat", true, "entry://C/cat"},
		{"the archive root", "a/b", "../", true, "../"},
		// Pre-6.1: the namespace is the first segment of every path, and only
		// the article namespace is a lookup.
		{"old scheme article", "A/x", "../A/cat", false, "entry://cat"},
		{"old scheme image", "A/x", "../I/p.png", false, "I/p.png"},
		{"old scheme layout", "A/x", "../-/s.css", false, "-/s.css"},
		{"old scheme sibling", "A/x", "cat", false, "entry://cat"},
		{"old scheme namespace alone", "A/x", "./", false, "./"},
		{"old scheme metadata", "A/x", "../M/Title", false, "../M/Title"},
		// a bad escape is kept as a literal "%", which the link then encodes
		{"bad escape is a literal percent", "x", "a%zz", true, "entry://a%25zz"},
		{"encoded hash stays in the headword", "x", "C%23", true, "entry://C%23"},
		{"leading at is a headword, not a sub-entry", "x", "%40home", true, "entry://%40home"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkRef(tc.base, tc.ref, tc.newNS); got != tc.want {
				t.Errorf("linkRef(%q, %q, %v) = %q, want %q", tc.base, tc.ref, tc.newNS, got, tc.want)
			}
		})
	}
}

// A resource resolves to its path from the archive's root: the server maps
// that under /res/{id}/, where a relative "../" would have climbed out of the
// dictionary's folder.
func TestResourceRef(t *testing.T) {
	cases := []struct{ name, base, ref, want string }{
		{"old scheme image", "A/Bitcoin", "../I/x.jpg.webp", "I/x.jpg.webp"},
		{"old scheme layout", "A/Bitcoin", "../-/mw/site.js", "-/mw/site.js"},
		{"devdocs, two levels up", "archive/tar/index", "../../application.css", "application.css"},
		{"devdocs, one level up", "fmt/index", "../assets/index.css", "assets/index.css"},
		{"mwoffliner", "Python", "./_assets_/a.jpg", "_assets_/a.jpg"},
		{"mwoffliner subpage", "Python/Foo", "../_res_/x.css", "_res_/x.css"},
		{"percent-encoding kept", "A/x", "../I/Caf%C3%A9.jpg", "I/Caf%C3%A9.jpg"},
		{"query and fragment kept", "a/b", "c.svg?v=2#icon", "a/c.svg?v=2#icon"},
		{"root-relative is the archive root", "a/b", "/s.css", "s.css"},
		{"encoded dots are dots", "a/b/c", "%2e%2E/d.png", "a/d.png"},
		{"a raw base is escaped", "100% pure/x", "y.png", "100%25%20pure/y.png"},
		{"a colon in the first segment", "x", "./a:b.png", "./a:b.png"},
		{"fragment only", "a/b", "#map", "#map"},
		{"data uri", "a/b", "data:image/png;base64,AA", "data:image/png;base64,AA"},
		{"network path", "a/b", "//cdn.example/x.js", "//cdn.example/x.js"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceRef(tc.base, tc.ref); got != tc.want {
				t.Errorf("resourceRef(%q, %q) = %q, want %q", tc.base, tc.ref, got, tc.want)
			}
		})
	}
}

func TestRemoveDots(t *testing.T) {
	cases := map[string]string{
		"a/b/../c":  "a/c",
		"../x":      "x",
		"a/./b":     "a/b",
		"a/b/..":    "a/",
		"./":        "",
		"a//b":      "a//b",
		"a/b/../..": "",
		"...":       "...",
		"a/.b/c":    "a/.b/c",
	}
	for in, want := range cases {
		if got := removeDots(in); got != want {
			t.Errorf("removeDots(%q) = %q, want %q", in, got, want)
		}
	}
}

// The whole article: every kind of reference site goes through resolution.
func TestRewriteRefs(t *testing.T) {
	in := `<link rel="stylesheet" href="../-/style.css"><img src="../I/a.png" srcset="../I/a2.png 2x">` +
		`<p style="background:url(../I/bg.png)">x</p><a href="Other_Page">o</a><a href="../I/f.pdf">f</a>`
	want := `<link rel="stylesheet" href="-/style.css"><img src="I/a.png" srcset="I/a2.png 2x">` +
		`<p style="background:url(I/bg.png)">x</p><a href="entry://Other Page">o</a><a href="I/f.pdf">f</a>`
	if got := rewriteRefs(in, "A/Bitcoin", false); got != want {
		t.Errorf("rewriteRefs\n = %s\nwant %s", got, want)
	}
}

func TestPathVariants(t *testing.T) {
	cases := []struct {
		word string
		want []string
	}{
		{"cat", []string{"cat", "Cat"}},
		{"new york", []string{"new york", "new_york", "New york", "New_york", "New_York"}},
		{"Cat", []string{"Cat"}},
		{"ăsta", []string{"ăsta", "Ăsta"}},
	}
	for _, tc := range cases {
		got := pathVariants(tc.word)
		if len(got) != len(tc.want) {
			t.Fatalf("pathVariants(%q) = %v, want %v", tc.word, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("pathVariants(%q) = %v, want %v", tc.word, got, tc.want)
			}
		}
	}
}

func TestTrimRelative(t *testing.T) {
	cases := map[string]string{
		"./a":       "a",
		"../a":      "a",
		"../../a/b": "a/b",
		"/a":        "a",
		"a":         "a",
		"":          "",
		"...":       "...",
		"././":      "",
	}
	for in, want := range cases {
		if got := trimRelative(in); got != want {
			t.Errorf("trimRelative(%q) = %q, want %q", in, got, want)
		}
	}
}
