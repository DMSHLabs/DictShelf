// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/wuweidict/wudict/internal/htmlref"
)

// The `html` writer mode (spec R6.5): an article's HTML kept verbatim as one
// CommonMark HTML block. Three things change, and nothing else:
//
//   - lookup links take their canonical spelling (writers emit only entry://);
//   - the body is wrapped in <div> unless it already is a single element that
//     opens an HTML block of CommonMark's type 6;
//   - no line is left blank, since a blank line ends an HTML block and the
//     rest would be read as markdown. Inside pre, textarea and listing a blank
//     line is text, so its line end becomes &#10;; everywhere else it goes,
//     including inside script and style, where no entity would be decoded.

// type6 are the tags that open a CommonMark type-6 HTML block.
var type6 = map[atom.Atom]bool{
	atom.Address: true, atom.Article: true, atom.Aside: true, atom.Base: true, atom.Basefont: true,
	atom.Blockquote: true, atom.Body: true, atom.Caption: true, atom.Center: true, atom.Col: true,
	atom.Colgroup: true, atom.Dd: true, atom.Details: true, atom.Dialog: true, atom.Dir: true, atom.Div: true,
	atom.Dl: true, atom.Dt: true, atom.Fieldset: true, atom.Figcaption: true, atom.Figure: true,
	atom.Footer: true, atom.Form: true, atom.Frame: true, atom.Frameset: true, atom.H1: true, atom.H2: true,
	atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true, atom.Head: true, atom.Header: true,
	atom.Hr: true, atom.Html: true, atom.Iframe: true, atom.Legend: true, atom.Li: true, atom.Link: true,
	atom.Main: true, atom.Menu: true, atom.Menuitem: true, atom.Nav: true, atom.Noframes: true, atom.Ol: true,
	atom.Optgroup: true, atom.Option: true, atom.P: true, atom.Param: true, atom.Search: true,
	atom.Section: true, atom.Summary: true, atom.Table: true, atom.Tbody: true, atom.Td: true,
	atom.Tfoot: true, atom.Th: true, atom.Thead: true, atom.Title: true, atom.Tr: true, atom.Track: true,
	atom.Ul: true,
}

// htmlBody converts an article's HTML to its `html` mode block.
func htmlBody(src string) string {
	s := strings.Trim(htmlref.CanonLinks(validText(src)), " \t\r\n\f")
	if s == "" {
		return ""
	}
	s = noBlankLines(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
	if !wrapped(s) {
		s = "<div>\n" + s + "\n</div>"
	}
	return s
}

// wrapped reports a body that is a single element opening a type-6 HTML
// block: its first line starts that block, and no wrapper is needed.
func wrapped(s string) bool {
	nodes, err := html.ParseFragment(strings.NewReader(s), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return false
	}
	var only *html.Node
	for _, n := range nodes {
		switch {
		case n.Type == html.TextNode && strings.Trim(n.Data, " \t\n\f") == "":
		case only == nil && n.Type == html.ElementNode && n.Namespace == "" && type6[n.DataAtom]:
			only = n
		default:
			return false
		}
	}
	if only == nil {
		return false
	}
	// The parser may have moved or implied the element: it must be the text
	// the body starts with.
	tag := "<" + only.Data
	if len(s) <= len(tag) || !strings.EqualFold(s[:len(tag)], tag) {
		return false
	}
	// What follows the name must open the block in the reader (goldmark): a
	// space, `>` or `/>`; after a tab or a line end it reads a paragraph.
	switch rest := s[len(tag):]; {
	case rest[0] == ' ', rest[0] == '>', strings.HasPrefix(rest, "/>"):
		return true
	}
	return false
}

// noBlankLines removes every blank line, turning one inside pre, textarea or
// listing text into &#10; so that text is unchanged.
func noBlankLines(s string) string {
	if !hasBlankLine(s) {
		return s
	}
	// Text inside pre, textarea and listing, as byte ranges of s.
	var keep [][2]int
	z := html.NewTokenizer(strings.NewReader(s))
	off, depth := 0, 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		raw := len(z.Raw())
		switch tt {
		case html.StartTagToken, html.EndTagToken:
			name, _ := z.TagName()
			switch atom.Lookup(name) {
			case atom.Pre, atom.Textarea, atom.Listing:
				if tt == html.StartTagToken {
					depth++
				} else if depth > 0 {
					depth--
				}
			}
		case html.TextToken:
			if depth > 0 {
				keep = append(keep, [2]int{off, off + raw})
			}
		}
		off += raw
	}
	inText := func(at int) bool {
		for _, k := range keep {
			if at >= k[0] && at < k[1] {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	b.Grow(len(s))
	for at := 0; at < len(s); {
		end := strings.IndexByte(s[at:], '\n')
		line := s[at:]
		if end >= 0 {
			line = s[at : at+end]
		}
		next := at + len(line) + 1
		if strings.Trim(line, " \t") != "" {
			b.WriteString(line)
			if end >= 0 {
				b.WriteByte('\n')
			}
		} else if inText(at) && end >= 0 {
			// Its line end becomes an entity: the next line joins this one.
			b.WriteString(line)
			b.WriteString("&#10;")
		}
		at = next
	}
	return strings.TrimRight(b.String(), "\n")
}

func hasBlankLine(s string) bool {
	for _, l := range strings.Split(s, "\n") {
		if strings.Trim(l, " \t") == "" {
			return true
		}
	}
	return false
}
