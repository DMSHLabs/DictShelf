// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package htmlref locates the resource references inside one dictionary
// article and rewrites them, using the browser's own HTML tokenizer.
//
// # Why a tokenizer and not a regular expression
//
// This package exists because two separate call sites - the server's /res/
// rewriter and the media packer - each grew their own regular expression over
// article HTML, and both encoded the same wrong assumption: that attribute
// values are quoted. They are frequently not. Real dictionary HTML is
// hand-written, machine-repacked or scraped, and this is ordinary in it:
//
//	<a href=plaintiff__gb_1.ogg" onclick="new Audio(this.href).play()">
//
// The opening quote is missing. Every browser resolves that by the WHATWG
// tokenizer's "attribute value (unquoted) state", which ends the value at
// whitespace and keeps the stray quote - so the value is `plaintiff__gb_1.ogg"`.
// A `href="([^"]+)"` pattern simply does not see the attribute at all, leaves
// it alone, and the reference escapes to the page origin.
//
// Regexes over markup fail on more than that, and all of it appears in real
// dictionaries: unquoted values (legal HTML), spaces around `=`, `>` inside a
// quoted value, `src=` written inside a JavaScript string, and `href=` written
// in prose or inside a comment. None of those are edge cases you can patch
// one at a time - they are the difference between matching text and parsing a
// document. So this package parses.
//
// # The state machine
//
// golang.org/x/net/html implements the WHATWG tokenizer, which is the state
// machine browsers use; this package is the thin layer over it that decides
// which tokens carry references. Every token is accounted for:
//
//	TOKEN                    | STATE      | ACTION
//	-------------------------|------------|------------------------------------
//	StartTag, SelfClosingTag | any        | inspect attributes (see below). If
//	                         |            | Drop says so, the element is
//	                         |            | omitted. If no value changed, the
//	                         |            | ORIGINAL BYTES are emitted, so the
//	                         |            | document is preserved exactly.
//	                         |            | A raw-text tag (<style>, <script>,
//	                         |            | <title>, <textarea>, …, self-closing
//	                         |            | or dropped included) sets the
//	                         |            | raw-text state, as in the tokenizer.
//	EndTag                   | any        | emitted as-is; leaves raw text.
//	<audio> … </audio>       | PlayLinks  | held back until its end, then
//	                         |            | emitted as a play link or as-is.
//	Text                     | in <style> | CSS: url(...) references rewritten.
//	Text                     | in <script>| NEVER touched. A `src="…"` inside a
//	                         |            | script is a string literal, not
//	                         |            | markup - the regexes rewrote it.
//	Text                     | otherwise  | emitted as-is. Prose that mentions
//	                         |            | href="…" is not a reference either.
//	Comment, Doctype         | any        | emitted as-is.
//	Error                    | any        | end of input, or a malformed tail
//	                         |            | the tokenizer cannot continue past.
//	                         |            | Whatever is left is emitted so the
//	                         |            | function never loses bytes.
//
// Attribute classification, per element+attribute, is NOT decided here - it is
// policy, and the two call sites want different answers. See Ref and Rewriter.
//
// # Fidelity
//
// A Rewriter whose URL function returns its input unchanged reproduces
// well-formed input byte for byte - original quoting, attribute order,
// whitespace and case all survive, because an element with no changed value is
// emitted as its original bytes rather than re-serialised.
//
// The one deliberate exception is a value Clean repairs: `href=x.ogg"` comes
// back as `href="x.ogg"`. That is not a fidelity loss but the repair itself,
// and it is applied before any policy runs, because a malformed value is a
// parsing concern rather than a caller's decision. TestRoundTrip pins both
// halves against real dictionary articles.
//
// The other exception is opt-in: with PlayLinks, an <audio> that qualifies
// becomes an <a>. One that does not keeps its bytes.
package htmlref

import (
	"strings"

	"golang.org/x/net/html"
)

// Site says where a reference was found, because the syntax differs and so
// does what a caller may legitimately do with it.
type Site int

const (
	// SiteAttr is a single-URL attribute: src, poster, href on <link>, …
	SiteAttr Site = iota
	// SiteSrcset is one candidate of a srcset list ("url 2x").
	SiteSrcset
	// SiteCSS is one url(...) inside a style="…" attribute or a <style> block.
	SiteCSS
)

// Ref is one reference, handed to Rewriter.URL for a decision.
//
// Tag and Attr are what make an element-aware policy possible, and their
// absence is why the regexes could not be correct: `href` on <link> is a
// stylesheet this dictionary bundles, while `href` on <a> is a cross-reference
// the user may click. A pattern matching `href="…"` cannot tell them apart, so
// it treated both the same and turned every cross-reference into a resource
// fetch.
type Ref struct {
	Site Site
	Tag  string // lower-case element name ("" for a bare <style> text run)
	Attr string // lower-case attribute name ("" for SiteCSS in a <style> block)
	URL  string // the reference, already through Clean
}

// Rewriter walks an article. Both hooks are optional; a zero Rewriter is an
// expensive no-op that still round-trips exactly.
type Rewriter struct {
	// URL maps one reference to its replacement. Return the input unchanged
	// to leave it alone - that is not merely allowed, it is the case that
	// preserves the original bytes.
	URL func(Ref) string

	// Drop reports whether an element should be removed entirely, by
	// lower-case tag name. wudict uses it for <base>, whose href would
	// otherwise re-root every relative reference in the article.
	Drop func(tag string) bool

	// Text maps one prose text node to its replacement, given the RAW source
	// bytes as they appear in the document - entities intact, nothing
	// unescaped. Returning the input unchanged is the case that preserves the
	// original bytes, and a nil Text is that case for free.
	//
	// It is called only for text that is prose: never inside <script>,
	// <style>, <title> or any other raw-text element (rawTextTag), and never
	// for a comment or a doctype, which are different token types entirely.
	// wudict uses it to mark full-text matches (internal/hilite), which is why
	// the hook takes raw bytes and returns raw bytes: a marker that unescaped
	// its input would have to re-escape it, and any disagreement between the
	// two would rewrite what the article says.
	Text func(raw string) string

	// PlayLinks turns a controls-less <audio> speaker fallback into a browser
	// play link. Only a closed simple body with an <img> and playable URL can
	// qualify; elements with controls, script-facing attributes, unsafe schemes,
	// nested anchors or parser-ambiguous content are preserved as written.
	PlayLinks bool
}

var fallbackTag = map[string]bool{
	"img": true, "source": true, "track": true, "br": true, "wbr": true,
	"span": true, "b": true, "i": true, "em": true, "strong": true, "small": true,
	"sub": true, "sup": true, "u": true, "s": true, "font": true, "abbr": true,
	"mark": true, "bdi": true, "bdo": true, "cite": true, "dfn": true, "q": true,
	"code": true, "kbd": true, "samp": true, "var": true, "time": true, "data": true,
}

type pendingAudio struct {
	open  string
	src   string
	body  strings.Builder
	stack []string
	img   bool
}

func (p *pendingAudio) admit(t *html.Token) bool {
	if !fallbackTag[t.Data] {
		return false
	}
	if t.Data == "img" {
		p.img = true
	}
	if !voidTag(t.Data) {
		p.stack = append(p.stack, t.Data)
	}
	return true
}

func (p *pendingAudio) finish(end string) string {
	if p.img && playable(p.src) {
		return `<a class="wu-audio" href="` + html.EscapeString(p.src) + `">` + p.body.String() + `</a>`
	}
	return p.open + p.body.String() + end
}

func (p *pendingAudio) abandon() string { return p.open + p.body.String() }

func playLinkSrc(t *html.Token) (string, bool) {
	src, seen := "", false
	for _, a := range t.Attr {
		switch k := a.Key; {
		case k == "src":
			if !seen {
				src, seen = Clean(a.Val), true
			}
		case k == "controls", k == "autoplay", k == "muted", k == "loop", k == "id",
			strings.HasPrefix(k, "on"), strings.HasPrefix(k, "data-"):
			return "", false
		}
	}
	return src, true
}

func firstAttr(t *html.Token, name string) string {
	for _, a := range t.Attr {
		if a.Key == name {
			return Clean(a.Val)
		}
	}
	return ""
}

func firstDirectSource(body string) string {
	z := html.NewTokenizer(strings.NewReader(body))
	depth := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if depth == 0 && t.Data == "source" {
				return firstAttr(&t, "src")
			}
			if !voidTag(t.Data) {
				depth++
			}
		case html.EndTagToken:
			if depth > 0 {
				depth--
			}
		}
	}
}

func playable(u string) bool {
	if u == "" || u[0] == '#' || u[0] == '?' {
		return false
	}
	i := strings.IndexAny(u, ":/?#")
	if i < 0 || u[i] != ':' {
		return true
	}
	switch strings.ToLower(u[:i]) {
	case "http", "https":
		return true
	case "data":
		return strings.HasPrefix(strings.ToLower(u[i+1:]), "audio/")
	}
	return false
}

func endTagName(raw []byte) []byte {
	if len(raw) < 2 {
		return nil
	}
	raw = raw[2:]
	for i, c := range raw {
		switch c {
		case ' ', '\t', '\n', '\f', '\r', '/', '>':
			return raw[:i]
		}
	}
	return raw
}

func tagIs(name []byte, lower string) bool {
	if len(name) != len(lower) {
		return false
	}
	for i, c := range name {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

// rawTextTag reports whether an element's content is raw text rather than
// markup. The tokenizer already switches to those states; this mirror lets us
// tell a <style> text run (CSS, rewritable) from a <script> one (never).
func rawTextTag(name string) bool {
	switch name {
	case "script", "style", "title", "textarea", "iframe", "noscript", "noembed", "noframes", "xmp", "plaintext":
		return true
	}
	return false
}

// Rewrite returns doc with every reference passed through URL.
func (rw Rewriter) Rewrite(doc string) string {
	if doc == "" {
		return doc
	}
	z := html.NewTokenizer(strings.NewReader(doc))
	var b strings.Builder
	b.Grow(len(doc) + len(doc)/8)
	out := &b
	var pa *pendingAudio
	inAnchor := false
	raw := "" // the raw-text element we are inside, "" when in markup
	for {
		switch z.Next() {
		case html.ErrorToken:
			// io.EOF, or markup the tokenizer cannot continue past. Emit
			// whatever it did not consume: a rewriter must never silently
			// truncate an article.
			tail := z.Raw()
			if pa != nil {
				if len(tail) == 0 {
					b.WriteString(pa.finish(""))
				} else {
					b.WriteString(pa.abandon())
				}
			}
			b.Write(tail)
			return b.String()

		case html.StartTagToken, html.SelfClosingTagToken:
			// Raw() is only valid until the next Next(); Token() copies.
			t := z.Token()
			if raw == "" && rawTextTag(t.Data) {
				raw = t.Data
			}
			if rw.Drop != nil && rw.Drop(t.Data) {
				continue
			}
			changed := rw.rewriteAttrs(&t)
			if pa != nil {
				if pa.admit(&t) {
					writeTag(&pa.body, &t, changed, z)
					continue
				}
				b.WriteString(pa.abandon())
				pa, out = nil, &b
			}
			if rw.PlayLinks {
				switch t.Data {
				case "a":
					inAnchor = true
				case "audio":
					if src, ok := playLinkSrc(&t); ok && src != "" && !inAnchor {
						pa = &pendingAudio{src: src}
						var open strings.Builder
						writeTag(&open, &t, changed, z)
						pa.open, out = open.String(), &pa.body
						continue
					}
				}
			}
			writeTag(out, &t, changed, z)

		case html.EndTagToken:
			if raw != "" {
				raw = ""
				out.Write(z.Raw())
				continue
			}
			if rw.PlayLinks {
				name := endTagName(z.Raw())
				if pa != nil {
					n := len(pa.stack)
					switch {
					case n > 0 && tagIs(name, pa.stack[n-1]):
						pa.stack = pa.stack[:n-1]
						pa.body.Write(z.Raw())
						continue
					case n == 0 && tagIs(name, "audio"):
						if pa.src == "" {
							pa.src = firstDirectSource(pa.body.String())
						}
						b.WriteString(pa.finish(string(z.Raw())))
						pa, out = nil, &b
						continue
					}
					b.WriteString(pa.abandon())
					pa, out = nil, &b
				}
				if tagIs(name, "a") {
					inAnchor = false
				}
			}
			out.Write(z.Raw())

		case html.TextToken:
			// Only <style> holds rewritable text. <script> explicitly does
			// not, and neither does prose - but prose is exactly what Text
			// wants, so the two hooks are mutually exclusive by construction:
			// raw == "" IS the definition of prose here.
			switch {
			case raw == "style" && rw.URL != nil:
				out.WriteString(rewriteCSS(string(z.Raw()), func(u string) string {
					return rw.URL(Ref{Site: SiteCSS, Tag: "style", URL: u})
				}))
			case raw == "" && rw.Text != nil:
				// Raw() is only valid until the next Next(); the hook is
				// called and its result written before that happens.
				out.WriteString(rw.Text(string(z.Raw())))
			default:
				out.Write(z.Raw())
			}

		default: // Comment, Doctype
			out.Write(z.Raw())
		}
	}
}

func writeTag(w *strings.Builder, t *html.Token, changed bool, z *html.Tokenizer) {
	if changed {
		w.WriteString(t.String())
	} else {
		w.Write(z.Raw())
	}
}

// rewriteAttrs applies URL to every reference-bearing attribute of t,
// reporting whether anything changed.
func (rw Rewriter) rewriteAttrs(t *html.Token) bool {
	if rw.URL == nil {
		return false
	}
	changed := false
	for i := range t.Attr {
		a := &t.Attr[i]
		name := strings.ToLower(a.Key)
		var out string
		switch {
		case name == "style":
			out = rewriteCSS(a.Val, func(u string) string {
				return rw.URL(Ref{Site: SiteCSS, Tag: t.Data, Attr: name, URL: Clean(u)})
			})
		case isSrcset(name):
			out = rewriteSrcset(a.Val, func(u string) string {
				return rw.URL(Ref{Site: SiteSrcset, Tag: t.Data, Attr: name, URL: u})
			})
		case isURLAttr(name):
			out = rw.URL(Ref{Site: SiteAttr, Tag: t.Data, Attr: name, URL: Clean(a.Val)})
		default:
			continue
		}
		if out != a.Val {
			a.Val = out
			changed = true
		}
	}
	return changed
}

// urlAttr is every attribute whose value is a single URL. The `data-`/`xlink:`
// forms matter: repacked dictionaries habitually stash the real reference in
// data-src or data-original and swap it in with their own script.
var urlAttr = map[string]bool{
	"src": true, "href": true, "data": true, "poster": true,
	"background": true, "longdesc": true, "usemap": true,
}

// isURLAttr accepts a bare name or a prefixed one - data-src, xlink:href,
// data-original-src. The separator is required, so "metadata" is not "data"
// and "srcdoc" is not "src".
func isURLAttr(name string) bool {
	if urlAttr[name] {
		return true
	}
	if i := strings.LastIndexAny(name, "-:"); i >= 0 {
		return urlAttr[name[i+1:]]
	}
	return false
}

func isSrcset(name string) bool {
	return name == "srcset" || name == "imagesrcset" ||
		strings.HasSuffix(name, "-srcset") || strings.HasSuffix(name, ":srcset")
}

// Refs returns every reference in doc, in document order, with duplicates
// kept - deduplication is the caller's policy.
func (rw Rewriter) Refs(doc string) []Ref {
	var out []Ref
	inner := rw.URL
	Rewriter{
		Drop: rw.Drop,
		URL: func(r Ref) string {
			out = append(out, r)
			if inner != nil {
				return inner(r)
			}
			return r.URL
		},
	}.Rewrite(doc)
	return out
}
