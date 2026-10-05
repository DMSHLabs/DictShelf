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

// GDOptions is retained while the UI and source descriptors still use GD names.
type GDOptions = ArticleOptions

func transformGDWithOptions(text, key string, ab *abbrevMap, options GDOptions) (string, []string, error) {
	return transformArticleWithOptions(text, key, ab, options)
}

// Only comparison tests use GoldenDict's historical rendering rules.
func transformGDReferenceWithOptions(text, key string, ab *abbrevMap, options GDOptions) (string, []string, error) {
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

// Media controls and examples are skipped; definition links and plain
// translations keep the paragraph visible. Include the block's own example
// role because cleanup may have moved the class out of its span.
func gdExampleOnly(block *html.Node) bool {
	return gdRoleOnly(block, "wu-ex", "dsl_ex")
}

func gdRoleOnly(block *html.Node, role, legacy string) bool {
	hasExample, ownText := false, false
	var visit func(*html.Node, bool)
	visit = func(n *html.Node, skipped bool) {
		example := gdClass(n, role) || gdClass(n, legacy)
		hasExample = hasExample || example
		skipped = skipped || example || gdClass(n, "wu-audio") || gdClass(n, "wu-file")
		if n.Type == html.TextNode && !skipped {
			for _, r := range n.Data {
				if !unicode.IsSpace(r) && !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
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

// Prepare presentation and folding once, while the article is indexed.
func gdPrepareExamples(root *html.Node) {
	var blocks, examples []*html.Node
	nestedBlocks := make(map[*html.Node]bool)
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "p" || gdClass(n, "wu-m") {
				blocks = append(blocks, n)
				for p := n.Parent; p != nil && p != root; p = p.Parent {
					if p.Data == "p" || gdClass(p, "wu-m") {
						nestedBlocks[p] = true
					}
				}
			}
			if gdClass(n, "wu-ex") || gdClass(n, "dsl_ex") {
				examples = append(examples, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(root)
	for _, block := range blocks {
		if gdRoleOnly(block, "wu-sec", "dsl_opt") {
			gdAddClass(block, "wu-xonly")
		}
	}
	for _, block := range blocks {
		if !gdExampleOnly(block) {
			continue
		}
		gdAddClass(block, "wu-exonly")
		if nestedBlocks[block] {
			continue
		}
		gdAddClass(block, "wu-example-block")
		var bullets []*html.Node
		var lengths []int
		stopped := false
		var scan func(*html.Node)
		scan = func(n *html.Node) {
			if stopped || n.Type == html.ElementNode && n.Data == "a" {
				return
			}
			if n.Type == html.TextNode {
				end, marker := 0, false
				for i, r := range n.Data {
					if unicode.IsSpace(r) {
						end = i + len(string(r))
						continue
					}
					if gdExampleMarker(r) {
						marker = true
						end = i + len(string(r))
						continue
					}
					break
				}
				if marker {
					bullets = append(bullets, n)
					lengths = append(lengths, end)
				}
				for _, r := range n.Data[end:] {
					if !unicode.IsSpace(r) && !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
						stopped = true
						break
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				scan(c)
			}
		}
		scan(block)
		for i, n := range bullets {
			span := &html.Node{Type: html.ElementNode, Data: "span", DataAtom: atom.Span}
			gdAddClass(span, "wu-example-bullet")
			span.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data[:lengths[i]]})
			n.Parent.InsertBefore(span, n)
			n.Data = n.Data[lengths[i]:]
			gdAddClass(block, "wu-author-bullet")
		}
	}
	for _, ex := range examples {
		nested := false
		for p := ex; p != nil && p != root; p = p.Parent {
			if gdClass(p, "wu-example-block") || p != ex && (gdClass(p, "wu-ex") || gdClass(p, "dsl_ex")) {
				nested = true
				break
			}
		}
		if !nested {
			gdAddClass(ex, "wu-inline-example")
		}
	}
}

func gdExampleMarker(r rune) bool {
	return r >= 0x25a0 && r <= 0x25ff || strings.ContainsRune("•‣⁃⁌⁍∙·♦★☆☐☑☒❥❧➔➜➤➢→⇒*+-–—", r)
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
	gdPrepareExamples(root)
	var out bytes.Buffer
	if options.Styles {
		out.WriteString(`<style>@import url("/assets/presets/gd/article-style.css?v=4");</style><div class="wu-gd" data-wu-examples="3">`)
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
