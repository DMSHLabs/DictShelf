// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"bytes"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// GDOptions keeps the reference parser separate from optional HTML cleanup.
// The zero value disables cleanup and bundled presentation for oracle tests.
type GDOptions struct {
	Enhance bool
	Styles  bool
}

func transformGDWithOptions(text, key string, ab *abbrevMap, options GDOptions) (string, []string, error) {
	body, media, err := transformGDBody(text, key, ab)
	if err != nil || (!options.Enhance && !options.Styles) {
		return body, media, err
	}
	body, err = prepareGDHTML(body, options)
	return body, media, err
}

func gdClass(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == name {
					return true
				}
			}
		}
	}
	return false
}

func gdAddClass(n *html.Node, name string) {
	if gdClass(n, name) {
		return
	}
	for i := range n.Attr {
		if n.Attr[i].Key == "class" {
			n.Attr[i].Val += " " + name
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: "class", Val: name})
}

func gdText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var out strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out.WriteString(gdText(c))
	}
	return out.String()
}

func gdBlank(n *html.Node) bool {
	return n != nil && n.Type == html.TextNode && strings.TrimSpace(n.Data) == ""
}

func gdBlock(n *html.Node) bool {
	return n != nil && (n.Data == "p" || n.Data == "div") && n.Type == html.ElementNode
}

func gdPure(n *html.Node) bool {
	if n.Parent == nil {
		return false
	}
	for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
		if c != n && !gdBlank(c) {
			return false
		}
	}
	return true
}

func gdUnwrap(n *html.Node) {
	parent := n.Parent
	for n.FirstChild != nil {
		c := n.FirstChild
		n.RemoveChild(c)
		parent.InsertBefore(c, n)
	}
	parent.RemoveChild(n)
}

// Enhancer's processDslDef drops empty paragraph separators before blocks/end
// and collapses pure wrapper chains. Unlike the JS, we never discard attributes
// or the semantic formatting carried by native b/i/p tags.
func gdCleanup(root *html.Node) {
	var unwrapGlyphs func(*html.Node)
	unwrapGlyphs = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			unwrapGlyphs(c)
			if c.Type == html.ElementNode && c.Data == "span" && len(c.Attr) == 1 && c.Attr[0].Key == "class" && c.Attr[0].Val == "gde" {
				gdUnwrap(c)
			}
			c = next
		}
	}
	unwrapGlyphs(root)
	var paragraphs []*html.Node
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "p" {
			paragraphs = append(paragraphs, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(root)
	for _, n := range paragraphs {
		if n.Parent == nil || n.FirstChild != nil && (!gdBlank(n.FirstChild) || n.FirstChild.NextSibling != nil) {
			continue
		}
		candidate := n
		for p := candidate.Parent; p != nil && p != root && !gdBlock(p); p = candidate.Parent {
			// Text emptiness must not discard an image, audio or another void node.
			if !gdPure(candidate) || strings.TrimSpace(gdText(p)) != "" {
				break
			}
			candidate = p
		}
		next := candidate.NextSibling
		for gdBlank(next) {
			next = next.NextSibling
		}
		if next == nil || gdBlock(next) {
			candidate.Parent.RemoveChild(candidate)
		}
	}
	var merge func(*html.Node)
	merge = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			merge(c)
			if c.Parent != nil && n != root && gdPure(c) && gdMergeable(c, n) {
				for _, a := range c.Attr {
					if a.Key == "class" {
						for _, name := range strings.Fields(a.Val) {
							gdAddClass(n, name)
						}
					} else {
						n.Attr = append(n.Attr, a)
					}
				}
				gdUnwrap(c)
			}
			c = next
		}
	}
	merge(root)
}

func gdMergeable(child, parent *html.Node) bool {
	if child.Type != html.ElementNode || parent.Type != html.ElementNode ||
		(child.Data != "span" && child.Data != "p" && child.Data != "div") {
		return false
	}
	if gdBlock(child) && !gdBlock(parent) {
		return false
	}
	// Different colours/margins/language/title/link metadata must remain nested.
	for _, a := range child.Attr {
		if a.Key != "class" {
			for _, b := range parent.Attr {
				if b.Key == a.Key {
					return false
				}
			}
		}
	}
	for _, role := range []string{"wu-c", "wu-m", "wu-lang", "wu-p", "wu-unknown"} {
		if gdClass(child, role) && gdClass(parent, role) {
			return false
		}
	}
	return true
}

// Same text rule as examples.js. Links and examples are skipped; plain
// translations keep the paragraph visible. Include the block's own example
// role because cleanup may have moved the class out of its span.
func gdExampleOnly(block *html.Node) bool {
	hasExample, ownText := false, false
	var visit func(*html.Node, bool)
	visit = func(n *html.Node, skipped bool) {
		example := gdClass(n, "wu-ex") || gdClass(n, "dsl_ex")
		hasExample = hasExample || example
		skipped = skipped || example || n.Type == html.ElementNode && n.Data == "a"
		if n.Type == html.TextNode && !skipped {
			for _, r := range n.Data {
				if !unicode.IsSpace(r) && !strings.ContainsRune("▪•·-–—", r) {
					ownText = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, skipped)
		}
	}
	visit(block, false)
	return hasExample && !ownText
}

func prepareGDHTML(body string, options GDOptions) (string, error) {
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(body), root)
	if err != nil {
		return "", err
	}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	if options.Enhance {
		gdCleanup(root)
	}
	var mark func(*html.Node)
	mark = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "p" || gdClass(n, "wu-m")) && gdExampleOnly(n) {
			gdAddClass(n, "wu-xonly")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			mark(c)
		}
	}
	mark(root)
	var out bytes.Buffer
	if options.Styles {
		out.WriteString(`<style>@import url("/assets/presets/gd/article-style.css?v=4");</style><div class="wu-gd">`)
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&out, c); err != nil {
			return "", err
		}
	}
	if options.Styles {
		out.WriteString("</div>")
	}
	return out.String(), nil
}
