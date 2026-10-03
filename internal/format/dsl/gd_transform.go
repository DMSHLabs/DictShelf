// Tree construction and crossed-tag repair follow GoldenDict's ArticleDom
// (dsl_details.cc, Konstantin Isakov and contributors, GPL-3.0-or-later).
// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/htmlref"
	"golang.org/x/text/unicode/norm"
)

type gdNode struct {
	tag          string
	attrs        map[string]string
	rawAttrs     string
	text         string
	children     []*gdNode
	mediaTag     string
	mediaPayload string
}

type gdParser struct {
	root      gdNode
	stack     []*gdNode
	key       string
	nodes     int
	pending   strings.Builder
	resources []string
}

func (p *gdParser) append(n *gdNode) {
	p.flushText()
	p.appendRaw(n)
}

func (p *gdParser) appendRaw(n *gdNode) {
	parent := &p.root
	if len(p.stack) > 0 {
		parent = p.stack[len(p.stack)-1]
	}
	parent.children = append(parent.children, n)
	p.nodes++
}

func (p *gdParser) text(s string) {
	p.pending.WriteString(s)
}

func (p *gdParser) flushText() {
	if p.pending.Len() > 0 {
		p.appendRaw(&gdNode{text: p.pending.String()})
		p.pending.Reset()
	}
}

func (p *gdParser) open(tag string, attrs map[string]string) *gdNode {
	p.flushText()
	var reopen []*gdNode
	if isMarginTag(tag) {
		for _, n := range p.stack {
			if !isMarginTag(n.tag) {
				reopen = append(reopen, n)
			}
		}
		p.stack = nil
	}
	n := &gdNode{tag: tag, attrs: attrs}
	p.append(n)
	p.stack = append(p.stack, n)
	for _, old := range reopen {
		n := &gdNode{tag: old.tag, attrs: old.attrs, rawAttrs: old.rawAttrs}
		p.append(n)
		p.stack = append(p.stack, n)
	}
	return n
}

func (p *gdParser) close(tag string) {
	p.flushText()
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].tag != tag && !(isMarginTag(tag) && isMarginTag(p.stack[i].tag)) {
			continue
		}
		reopen := append([]*gdNode(nil), p.stack[i+1:]...)
		for j := len(p.stack) - 1; j >= i; j-- {
			n := p.stack[j]
			if len(n.children) == 0 && n.tag != "br" {
				parent := &p.root
				if j > 0 {
					parent = p.stack[j-1]
				}
				parent.children = parent.children[:len(parent.children)-1]
			}
		}
		p.stack = p.stack[:i]
		for _, old := range reopen {
			p.open(old.tag, old.attrs).rawAttrs = old.rawAttrs
		}
		return
	}
}

func gdTag(text string, pos int) (tag string, attrs map[string]string, next int, ok bool) {
	// Use the existing attribute lexer, while keeping tree construction independent.
	i := pos + 1
	for i < len(text) && text[i] != ']' && text[i] != ' ' && text[i] != '\t' {
		i++
	}
	if i >= len(text) {
		return "", nil, pos, false
	}
	tag = text[pos+1 : i]
	if tag == "" || tag == "/" {
		return "", nil, pos, false
	}
	if text[i] == ']' {
		return tag, nil, i + 1, true
	}
	tr := transformer{input: text, pos: i}
	tr.skipAny(" \t")
	attrs = tr.lexAttrs()
	if tr.pos == 0 || text[tr.pos-1] != ']' {
		return "", nil, pos, false
	}
	// GoldenDict's language renderer requires "id=", not the permissive
	// "id 2" spelling accepted by the ordinary reader's attribute lexer.
	if tag == "lang" && !strings.Contains(text[i:tr.pos-1], "id=") && attrs["id"] != "" {
		attrs[attrs["id"]] = ""
		attrs["id"] = ""
	}
	return tag, attrs, tr.pos, true
}

func (p *gdParser) parse(text string) error {
	text = stripComments(text)
	for i := 0; i < len(text); {
		if len(p.stack) > 256 || p.nodes > 1_000_000 {
			return fmt.Errorf("dsl GD: markup nesting or node limit exceeded")
		}
		if text[i] == '\\' {
			i++
			if i == len(text) {
				p.text("\\")
				break
			}
			if text[i] == ' ' {
				p.append(&gdNode{tag: "literal", text: "\u00a0"})
				i++
				continue
			}
			if text[i] == '\n' || text[i] == '\r' {
				p.append(&gdNode{tag: "literal", text: "\u00a0"})
				continue
			}
			r, size := utf8.DecodeRuneInString(text[i:])
			p.text(string(r))
			i += size
			continue
		}
		if strings.HasPrefix(text[i:], "[[") || strings.HasPrefix(text[i:], "]]") {
			p.text(text[i : i+1])
			i += 2
			continue
		}
		if text[i] == '[' {
			start := i
			tag, attrs, next, ok := gdTag(text, i)
			if ok {
				i = next
				if strings.HasPrefix(tag, "/") {
					p.close(tag[1:])
					continue
				}
				if tag == "br" {
					p.append(&gdNode{tag: tag})
					continue
				}
				if tag == "s" || tag == "video" {
					start := i
					tr := transformer{input: text, pos: i}
					name := tr.collectMediaName()
					i = tr.pos
					media := transformer{input: gdMediaText(name, p.key) + "[/s]"}
					media.lexTagS()
					p.resources = append(p.resources, media.resFiles...)
					payload := text[start:i]
					if end := strings.Index(payload, "[/"+tag+"]"); end >= 0 {
						payload = payload[:end]
					}
					p.append(&gdNode{tag: "media", text: media.out.String(), mediaTag: tag, mediaPayload: gdMediaText(payload, p.key)})
					continue
				}
				p.open(tag, attrs).rawAttrs = strings.TrimLeft(text[start+1+len(tag):next-1], " \t")
				continue
			}
		}
		if strings.HasPrefix(text[i:], "<<") {
			p.open("ref", nil)
			i += 2
			continue
		}
		if strings.HasPrefix(text[i:], ">>") {
			linked := false
			for _, n := range p.stack {
				if n.tag == "ref" {
					linked = true
					break
				}
			}
			if linked {
				p.close("ref")
			} else {
				p.text(">>")
			}
			i += 2
			continue
		}
		if text[i] == '~' {
			p.text(p.key)
			i++
			continue
		}
		if text[i] == '^' {
			if i+1 < len(text) && text[i+1] == '~' {
				p.text(flipCaseFirst(p.key))
				i += 2
				continue
			}
			p.text("^")
			i++
			continue
		}
		// Append runs, not individual bytes: long paragraphs stay linear.
		start := i
		i++
		for i < len(text) && !strings.ContainsRune("\\[]<>~^", rune(text[i])) {
			i++
		}
		p.text(text[start:i])
	}
	p.flushText()
	return nil
}

// ArticleDom unescapes media text without changing escaped spaces to NBSP.
func gdMediaText(text, key string) string {
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '\\' && i+1 < len(text):
			i++
			out.WriteByte(text[i])
		case strings.HasPrefix(text[i:], "[[") || strings.HasPrefix(text[i:], "]]"):
			out.WriteByte(text[i])
			i++
		case strings.HasPrefix(text[i:], "^~"):
			out.WriteString(flipCaseFirst(key))
			i++
		case text[i] == '~':
			out.WriteString(key)
		default:
			out.WriteByte(text[i])
		}
	}
	return out.String()
}

func gdPlain(n *gdNode) string {
	var b strings.Builder
	var visit func(*gdNode, bool)
	visit = func(n *gdNode, ipa bool) {
		if ipa && n.tag == "" {
			b.WriteString(gdTranscription(n.text))
		} else {
			b.WriteString(n.text)
		}
		for _, c := range n.children {
			visit(c, ipa || n.tag == "t")
		}
	}
	visit(n, false)
	return b.String()
}

func transformGDBody(text, key string, ab *abbrevMap) (string, []string, error) {
	p := gdParser{key: norm.NFC.String(key)}
	if err := p.parse(norm.NFC.String(text)); err != nil {
		return "", nil, err
	}
	gdTrimMarginWhitespace(&p.root)
	var render func(*gdNode, bool) string
	var children func(*gdNode, bool) string
	children = func(n *gdNode, ipa bool) string {
		var b strings.Builder
		for i, c := range n.children {
			if c.tag == "" {
				v := c.text
				if (i == 0 && (n.tag == "" || isMarginTag(n.tag))) || (i > 0 && isMarginTag(n.children[i-1].tag)) {
					v = strings.TrimLeft(v, " \t\r\n")
				}
				if (i == len(n.children)-1 && (n.tag == "" || isMarginTag(n.tag))) || (i+1 < len(n.children) && isMarginTag(n.children[i+1].tag)) {
					v = strings.TrimRight(v, " \t\r\n")
				}
				if ipa {
					v = gdTranscription(v)
				}
				v = strings.ReplaceAll(v, "\r", "")
				lines := strings.Split(v, "\n")
				for j := 1; j < len(lines); j++ {
					lines[j] = strings.TrimLeft(lines[j], " \t")
				}
				v = strings.Join(lines, "\n")
				v = strings.ReplaceAll(escape(v), "\n", "<br/>")
				b.WriteString(v)
			} else {
				b.WriteString(render(c, ipa))
			}
		}
		return b.String()
	}
	render = func(n *gdNode, ipa bool) string {
		if n.tag == "literal" {
			return escape(n.text)
		}
		if n.tag == "media" {
			return n.text
		}
		if n.tag == "br" {
			return "<br/>"
		}
		if n.tag == "ref" || n.tag == "url" {
			label := children(n, ipa)
			target := n.attrs["target"]
			if target == "" {
				target = strings.TrimSpace(gdPlain(n))
			}
			if n.tag == "ref" {
				target = normalizeGDLink(target)
				extra := ""
				if d := n.attrs["dict"]; d != "" {
					extra = ` class="wu-xref" data-dict=` + quoteAttr(d) + ` title=` + quoteAttr(d)
				}
				return "<a" + extra + " href=" + quoteAttr(htmlref.EntryHref(target)) + ">" + label + "</a>"
			}
			target = strings.Trim(target, " \f\n\r\t\v")
			if !strings.Contains(target, ":") {
				target = "http://" + target
			}
			return "<a href=" + quoteAttr(target) + ">" + label + "</a>"
		}
		if n.tag == "p" {
			label := children(n, ipa)
			var v string
			var ok bool
			if ab != nil {
				v, ok = ab.exact[gdPlain(n)]
			}
			if ok {
				if utf8.RuneCountInString(v) < 70 {
					v = strings.NewReplacer(" ", "\u00a0", "\t", "\u00a0", "-", "\u2011").Replace(v)
				}
				label = `<abbr class="wu-abbr" title=` + quoteAttr(v) + `>` + label + `</abbr>`
			}
			return `<span class="wu-p">` + label + `</span>`
		}
		content := children(n, ipa || n.tag == "t")
		if n.tag == "lang" {
			attrs := map[string]string{}
			if i := strings.Index(n.rawAttrs, "id="); i >= 0 {
				if _, err := strconv.Atoi(strings.TrimSpace(n.rawAttrs[i+3:])); err == nil {
					attrs["id"] = strings.TrimSpace(n.rawAttrs[i+3:])
				}
			} else if i := strings.Index(n.rawAttrs, `name="`); i >= 0 {
				value := n.rawAttrs[i+6:]
				if end := strings.IndexByte(value, '"'); end >= 0 {
					attrs["name"] = value[:end]
				}
			}
			return `<span class="wu-lang"` + langAttrs(attrs) + `>` + content + `</span>`
		}
		unknown := func() string {
			marker := "[" + n.tag
			if n.rawAttrs != "" {
				marker += " " + n.rawAttrs
			}
			return `<span class="wu-unknown">` + escape(marker+"]") + content + "</span>"
		}
		if n.tag == "trs" || n.tag == "!trn" || n.tag == "preview" {
			return unknown()
		}
		if n.tag == "u" && (strings.HasPrefix(content, " ") || strings.HasPrefix(content, "\t")) {
			return " <u>" + content + "</u>"
		}
		if isMarginTag(n.tag) && content == "" {
			return ""
		}
		tr := transformer{}
		_ = tr.processTag(n.tag, n.attrs)
		opener := tr.out.String()
		tr.out.Reset()
		tr.closeTag(n.tag)
		closer := tr.out.String()
		if opener == "" && closer == "" {
			return unknown()
		}
		if content == "" {
			return ""
		}
		return opener + content + closer
	}
	return children(&p.root, false), p.resources, nil
}

// Crossed inline tags may wrap the whitespace between two margin blocks.
// Trim through those wrappers, but leave explicit breaks and escaped spaces.
func gdTrimMarginWhitespace(n *gdNode) {
	var trimEdge func(*gdNode, bool)
	trimEdge = func(n *gdNode, left bool) {
		if n.tag == "" && len(n.children) == 0 {
			if left {
				n.text = strings.TrimLeft(n.text, " \t\r\n")
			} else {
				n.text = strings.TrimRight(n.text, " \t\r\n")
			}
			return
		}
		if n.tag == "literal" || n.tag == "br" || n.tag == "media" {
			return
		}
		for i := 0; i < len(n.children); i++ {
			j := i
			if !left {
				j = len(n.children) - 1 - i
			}
			c := n.children[j]
			trimEdge(c, left)
			if c.tag != "" || c.text != "" {
				break
			}
		}
	}
	for i, c := range n.children {
		gdTrimMarginWhitespace(c)
		if (i == 0 && isMarginTag(n.tag)) || (i > 0 && isMarginTag(n.children[i-1].tag)) {
			trimEdge(c, true)
		}
		if (i == len(n.children)-1 && isMarginTag(n.tag)) || (i+1 < len(n.children) && isMarginTag(n.children[i+1].tag)) {
			trimEdge(c, false)
		}
	}
}

// GoldenDict's normalizeHeadword collapses ASCII spaces, not arbitrary Unicode whitespace.
func normalizeGDLink(target string) string {
	return strings.Join(strings.FieldsFunc(target, func(r rune) bool { return r == ' ' }), " ")
}

// Legacy Lingvo transcription glyphs, as decoded by GoldenDict inside [t].
var gdIPAGlyphs = map[rune]string{
	0x2021: "æ", 0x407: "r", 0xB0: "k", 0x20AC: "ɔ", 0x404: "z", 0x40F: "ʃ",
	0xAB: "t", 0xAC: "d", 0x2020: "ə", 0x490: "m", 0xA7: "f", 0xAE: "l",
	0xB1: "g", 0x45E: "e", 0xAD: "n", 0xA9: "s", 0xA6: "w", 0x2026: "ʌ",
	0x452: "v", 0x408: "p", 0x40C: "u", 0x406: "h", 0xB5: "a", 0x491: "ɛ",
	0x40A: "ŋ", 0x2030: "ð", 0x456: "j", 0xA4: "b", 0x409: "ʒ", 0x40E: "i",
	0x40B: "Ө", 0xB6: "ʊ", 0x2018: "ɑ", 0x457: "ɥ", 0x458: "œ", 0x405: "œ̃",
	0x441: "ɲ", 0x442: "ɔ̃", 0x443: "ø", 0x445: "ɛ̃", 0x446: "ç", 0x44C: "ɑ̃",
	0x44D: "ɪ", 0x44F: "ɒ", '0': "β", '1': "ẽ", '2': "ɜ", '3': "ĩ", '4': "õ",
	'6': "ʎ", '7': "ɣ", '8': "ǝ", ':': "ː", '\'': "ˈ", 0x455: "ǐ", 0xB7: "ã",
	0xA0: "ʧ", 0x402: "i:", 0x403: "ɑ:", 0x428: "a", 0x453: "u:",
	0x201A: "ɔ", 0x201E: "ə", 0x2039: "dʒ",
}

func gdTranscription(s string) string {
	var b strings.Builder
	for _, r := range s {
		if v, ok := gdIPAGlyphs[r]; ok {
			b.WriteString(v)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
