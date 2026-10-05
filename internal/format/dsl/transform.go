// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package dsl implements ABBYY Lingvo DSL dictionaries: plain-text
// markup with no native index, so the direct backend transparently
// ingests into a text.db on first open (SPEC §1). Markup semantics are
// ported from pyglossary/plugins/dsl (lex.py, transform.py, title.py).
package dsl

import (
	"path"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/artmark"
)

// stripComments removes `{{...}}` comments. A comment may itself contain a
// single `}`, so the terminator is the first `}}` rather than "no closing
// brace at all". Escaped braces (`\{\{`) are not a comment: the backslash
// sits between them, so no `{{` exists, and an unterminated `{{` is literal
// text - a real one in an article body is a typo, not a request to delete
// the rest of the entry.
//
// A comment that stood alone on its line takes the line with it. Deleting
// only the braces leaves the line's indentation and its newline behind, and
// the lexer turns that leftover empty line into a <br/>.

func stripComments(text string) string {
	if !strings.Contains(text, "{{") {
		return text
	}
	out := make([]byte, 0, len(text))
	lineStart := 0 // index in out where the current line begins
	commented := false
	dropLine := func() bool {
		if !commented {
			return false
		}
		for _, c := range out[lineStart:] {
			switch c {
			case ' ', '\t', '\r', '\v', '\f':
			default:
				return false
			}
		}
		return true
	}
	for i := 0; i < len(text); {
		if text[i] == '{' && i+1 < len(text) && text[i+1] == '{' {
			if j := strings.Index(text[i+2:], "}}"); j >= 0 {
				i += 2 + j + len("}}")
				commented = true
				continue
			}
		}
		if text[i] == '\n' {
			if dropLine() {
				out = out[:lineStart] // the line and its newline both go
			} else {
				out = append(out, '\n')
				lineStart = len(out)
			}
			commented = false
			i++
			continue
		}
		out = append(out, text[i])
		i++
	}
	// A comment on the last line has no newline of its own to drop, so it
	// takes the one that ended the line before it; left in place that would
	// render as a trailing <br/>.
	if dropLine() {
		out = out[:lineStart]
		if lineStart > 0 && out[lineStart-1] == '\n' {
			out = out[:lineStart-1]
		}
	}
	return string(out)
}

// transformer converts one entry body from DSL markup to HTML in three
// stages over one token slice (D165): lex (this file), resolve and emit
// (balance.go). DSL tags are zones over the text, not a tree - Lingvo rejects
// a tag nested in itself, never two tags of different kinds that overlap - so
// the token stream is read as zones first and only the emitter decides how
// they nest. That is what keeps the HTML well formed whatever the source
// does: no consumer repairs it downstream (the browser repairs b/i/u only, a
// stray </span> closes whichever span is open; `clean` is a tokenizer).
type transformer struct {
	input      string
	currentKey string
	abbrev     *abbrevMap
	resFiles   []string

	toks []token

	// lexer state
	raw         string // link zone being read verbatim: "ref", "url", inlineLink, or ""
	noLinkUntil int    // a `<<` before this offset has no `>>` on its line

	// resolve and emit state (balance.go)
	zones   []zone
	stack   []int32
	fresh   []int32
	reopen  []int32
	pend    []int32
	lineEnd []int32
	endHead []int32
	out     strings.Builder
}

var transformers = sync.Pool{New: func() any { return new(transformer) }}

// transformBody renders a whole DSL entry body to HTML. currentKey replaces
// `~`. The surrounding whitespace is the file's own indentation, not content,
// so it is dropped - but only here, at the outer edge of a complete article.
func transformBody(text, currentKey string) (html string, resFiles []string, err error) {
	return transformBodyAbbrev(text, currentKey, nil)
}

// transformBodyAbbrev is transformBody with the dictionary's abbreviation
// glossary in hand, so [p] labels it knows can carry their expansion. A nil map
// is the no-companion case and produces byte-identical output.
func transformBodyAbbrev(text, currentKey string, ab *abbrevMap) (html string, resFiles []string, err error) {
	html, resFiles, err = transformFragmentAbbrev(text, currentKey, ab)
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(html), resFiles, nil
}

// transformFragment is transformBody without the trim, for markup that is
// concatenated with text on either side of it.
func transformFragment(text, currentKey string) (html string, resFiles []string, err error) {
	return transformFragmentAbbrev(text, currentKey, nil)
}

func transformFragmentAbbrev(text, currentKey string, ab *abbrevMap) (html string, resFiles []string, err error) {
	tr := transformers.Get().(*transformer)
	tr.reset(stripComments(text), currentKey, ab)
	tr.lex()
	tr.resolve()
	tr.emit()
	html, resFiles = tr.out.String(), tr.resFiles
	tr.reset("", "", nil) // the pool must not pin this article's strings
	transformers.Put(tr)
	return html, resFiles, nil
}

func (tr *transformer) reset(input, currentKey string, ab *abbrevMap) {
	tr.input, tr.currentKey, tr.abbrev, tr.resFiles = input, currentKey, ab, nil
	clear(tr.toks)
	clear(tr.zones)
	tr.toks, tr.zones = tr.toks[:0], tr.zones[:0]
	tr.stack, tr.fresh, tr.reopen, tr.pend = tr.stack[:0], tr.fresh[:0], tr.reopen[:0], tr.pend[:0]
	tr.lineEnd, tr.endHead = tr.lineEnd[:0], tr.endHead[:0]
	tr.raw, tr.noLinkUntil = "", 0
	tr.out = strings.Builder{}
	tr.out.Grow(len(input) + len(input)/4)
}

// tokKind classifies one lexed token.
type tokKind uint8

const (
	tkText  tokKind = iota // input[a:b], escaped on output
	tkLit                  // s, escaped on output
	tkHTML                 // s, written as it is
	tkBreak                // a source line break; s is its markup, "" when [m] follows
	tkOpen                 // [name attrs]; input[a:b] is the whole tag, s the name
	tkClose                // [/name]; s the name
)

type token struct {
	s      string
	attrs  map[string]string
	a, b   int32
	rng    int32 // tkOpen: the zone it starts, -1 for none
	kind   tokKind
	ipa    bool // tkText inside a legacy-font [t] zone (ipa.go)
	anchor bool // tkHTML: a media element drawn as <a>, which may not sit in a link
}

// tagKind is what a tag does to the text it covers.
type tagKind uint8

const (
	kUnknown tagKind = iota
	kInline          // a zone with fixed or attribute-built markup
	kLabel           // [p]: a zone whose text selects its markup (the abbreviation)
	kLink            // [ref], [url], <<…>>: a zone whose text is also its target
	kBlock           // [m], [mN]: a paragraph; never inside an inline zone
	kVoid            // [br]: markup with no zone
	kMedia           // [s], [video]: the zone's text is a file name, replaced by an element
	kIgnore          // [preview] outside a media zone: accepted, no effect
)

type tagDef struct {
	kind        tagKind
	open, close string // fixed markup; empty open = built from the zone (zoneMarkup)
}

// tagDefs is every tag Lingvo or GoldenDict accepts in a body. Every role is
// one <span> from the internal/artmark vocabulary.
var tagDefs = map[string]tagDef{
	"b":    {kInline, "<b>", "</b>"},
	"i":    {kInline, "<i>", "</i>"},
	"u":    {kInline, "<u>", "</u>"},
	"sup":  {kInline, "<sup>", "</sup>"},
	"sub":  {kInline, "<sub>", "</sub>"},
	"*":    {kInline, `<span class="wu-sec">`, "</span>"},
	"ex":   {kInline, `<span class="wu-ex">`, "</span>"},
	"t":    {kInline, `<span class="wu-ipa">`, "</span>"},
	"'":    {kInline, `<span class="wu-acc">`, "</span>"},
	"trn":  {kInline, `<span class="wu-trn">`, "</span>"},
	"!trn": {kInline, `<span class="wu-trn-not">`, "</span>"},
	"trs":  {kInline, `<span class="wu-trs">`, "</span>"},
	"!trs": {kInline, `<span class="wu-trs-not">`, "</span>"},
	"com":  {kInline, `<span class="wu-com">`, "</span>"},
	"c":    {kInline, "", "</span>"},
	"lang": {kInline, "", "</span>"},
	"p":    {kLabel, "", ""},
	"ref":  {kLink, "", "</a>"},
	"url":  {kLink, "", "</a>"},
	// Undocumented but compiled since Lingvo x5 (lingvo-ref "Тэг [br]"): a
	// hard line break with no closing descriptor.
	"br": {kVoid, "<br/>", ""},
	"s":  {kMedia, "", ""},
	// [video] is an undocumented exact synonym of [s], accepted by the Lingvo
	// x5 compiler (lingvo-ref "Тэги мультимедиа"): same syntax, same effect.
	"video": {kMedia, "", ""},
	// [preview] is legal only INSIDE [s]/[video], where lexMedia consumes it;
	// the compiler accepts it and it has no effect (lingvo-ref "Тэг
	// [preview]···[/preview]"), so a stray one is dropped rather than printed.
	"preview": {kIgnore, "", ""},
}

// inlineLink names the <<…>> zone. It holds a blank, which no tag name read
// by lexTag can, so no [tag] spelling opens or closes it.
const inlineLink = "<< >>"

// lookupTag resolves a tag name. [mN] shifts the left margin by N; a bare [m]
// is a shift of zero (lingvo-ref "Тэг [m]···[/m]"), and [/m2] closes the same
// paragraph as [/m].
func lookupTag(name string) (tagDef, bool) {
	if isMarginTag(name) {
		return tagDef{kind: kBlock, close: "</p>"}, true
	}
	if name == inlineLink {
		return tagDef{kind: kLink, close: "</a>"}, true
	}
	d, ok := tagDefs[name]
	return d, ok
}

// isMarginTag reports whether tag is [m] or [m] followed only by digits.
func isMarginTag(tag string) bool {
	if tag == "" || tag[0] != 'm' {
		return false
	}
	for i := 1; i < len(tag); i++ {
		if tag[i] < '0' || tag[i] > '9' {
			return false
		}
	}
	return true
}

func (tr *transformer) push(t token) { tr.toks = append(tr.toks, t) }

func (tr *transformer) text(a, b int) {
	tr.push(token{kind: tkText, a: int32(a), b: int32(b), rng: -1})
}

func (tr *transformer) lit(s string)  { tr.push(token{kind: tkLit, s: s, rng: -1}) }
func (tr *transformer) html(s string) { tr.push(token{kind: tkHTML, s: s, rng: -1}) }

// lex is the character loop. A construct that fails to parse - a `[` with no
// `]` on its line, a `<<` with no `>>` on its line - yields its first
// character as text, and lexing resumes right after it: a broken construct
// can never swallow the markup that follows it.
func (tr *transformer) lex() {
	in := tr.input
	i, run := 0, 0 // run: start of the pending stretch of plain text
	flush := func() {
		if i > run {
			tr.text(run, i)
		}
	}
	for i < len(in) {
		c := in[i]
		if tr.raw != "" {
			// A link's text is its target, so it is read verbatim: `^`, `]` and
			// `<` are themselves there, and `\` only escapes. `~` and `^~` are
			// the headword, as everywhere: GoldenDict expands them in the whole
			// article before parsing it. Tags still open and close inside it.
			switch {
			case c == '~':
				flush()
				tr.lit(tr.currentKey)
				i++
				run = i
			case c == '^' && i+1 < len(in) && in[i+1] == '~':
				flush()
				tr.lit(flipCaseFirst(tr.currentKey))
				i += 2
				run = i
			case c == '\\':
				flush()
				if i+1 < len(in) && in[i+1] == '\n' {
					// The blank-line idiom's backslash: the line break that
					// follows is markup, never link text.
					i++
				} else if i+1 < len(in) {
					n := 1 + runeLen(in[i+1:])
					tr.text(i+1, i+n)
					i += n
				} else {
					i++
				}
				run = i
			case c == '[':
				flush()
				i = tr.lexTag(i)
				run = i
			case c == '\n':
				// A link does not continue onto the next line; the line break
				// is read again below, in the normal mode.
				flush()
				tr.raw = ""
				run = i
			case c == '>' && tr.raw == inlineLink && i+1 < len(in) && in[i+1] == '>':
				flush()
				tr.push(token{kind: tkClose, s: inlineLink, rng: -1})
				tr.raw = ""
				i += 2
				run = i
			default:
				i++
			}
			continue
		}
		switch c {
		case '\\':
			flush()
			switch {
			case i+1 >= len(in):
				tr.text(i, i+1) // a trailing backslash is itself
				i++
			case in[i+1] == '\n':
				// `\` with the line break right behind it is the escaped space
				// of the blank-line idiom (lingvo-ref "Тело статьи": an empty
				// line between paragraphs is written as a body line holding
				// one escaped space). Editors strip the trailing space, so the
				// backslash is usually all that is left of it. The newline is
				// NOT consumed: the break it produces is the other half of the
				// blank line.
				tr.html("&nbsp;")
				i++
			case in[i+1] == ' ':
				tr.html("&nbsp;")
				i += 2
			case (in[i+1] == '<' || in[i+1] == '>') && i+2 < len(in) && in[i+2] == in[i+1]:
				tr.text(i+1, i+3)
				i += 3
			default:
				n := runeLen(in[i+1:])
				tr.text(i+1, i+1+n)
				i += 1 + n
			}
			run = i
		case '[':
			flush()
			i = tr.lexTag(i)
			run = i
		case ']':
			// "]]" is an escaped literal "]", the mirror of the "[[" that
			// lexTag folds (lingvo-ref "Удвоение квадратных скобок"); a lone
			// one is text (pyglossary parity).
			if i+1 < len(in) && in[i+1] == ']' {
				flush()
				tr.text(i, i+1)
				i += 2
				run = i
			} else {
				i++
			}
		case '~':
			flush()
			tr.lit(tr.currentKey)
			i++
			run = i
		case '^':
			// "^" inverts the case of the character that follows it
			// (lingvo-ref "Команда ^"). Its one real use is "^~": a dictionary
			// whose headwords are capitalised mirrors them into running text
			// lower-cased. "^" before markup or at end of input has nothing to
			// act on and disappears, which is what the compiler does with it.
			flush()
			i++
			switch {
			case i >= len(in):
			case in[i] == '~':
				tr.lit(flipCaseFirst(tr.currentKey))
				i++
			case in[i] == '[', in[i] == '\\':
			default:
				r, size := utf8.DecodeRuneInString(in[i:])
				tr.lit(flipCaseFirst(string(r)))
				i += size
			}
			run = i
		case '\n':
			// Leading whitespace of a continuation line is the file's
			// indentation. The break is markup unless [m] follows, whose <p>
			// already breaks the line.
			flush()
			i++
			for i < len(in) && (in[i] == ' ' || in[i] == '\t') {
				i++
			}
			// a: where the next line's content starts; a paragraph opening
			// there blanks s (lexTag).
			tr.push(token{kind: tkBreak, s: "<br/>", a: int32(i), rng: -1})
			run = i
		case '<':
			if i+1 < len(in) && in[i+1] == '<' && tr.linkCloses(i+2) {
				flush()
				tr.push(token{kind: tkOpen, s: inlineLink, a: int32(i), b: int32(i + 2), rng: -1})
				tr.raw = inlineLink
				i += 2
				run = i
			} else {
				i++
			}
		default:
			i++
		}
	}
	flush()
}

// linkCloses reports whether a `<<` whose text starts at i has its `>>` on the
// same line. A miss is remembered up to the line's end, so a line full of
// `<<` is scanned once, not once per `<<`.
func (tr *transformer) linkCloses(i int) bool {
	if i < tr.noLinkUntil {
		return false
	}
	in := tr.input
	for i < len(in) {
		switch in[i] {
		case '\\':
			if i+1 < len(in) && in[i+1] != '\n' {
				i += 2
				continue
			}
		case '\n':
			tr.noLinkUntil = i
			return false
		case '>':
			if i+1 < len(in) && in[i+1] == '>' {
				return true
			}
		}
		i++
	}
	tr.noLinkUntil = len(in)
	return false
}

// runeLen is the byte length of the rune s starts with, at least 1, so an
// escaped character is one token even when it is not ASCII.
func runeLen(s string) int {
	_, n := utf8.DecodeRuneInString(s)
	return max(n, 1)
}

// lexTag reads the tag at in[i] == '[' and returns where lexing resumes. A
// tag must end on its own line and contain no second `[` (Lingvo: "Repeated
// symbol [ is not allowed"); whitespace after the `[` is accepted only before
// a name this package knows (GoldenDict reads `[ b]` as [b]). Anything else
// is a literal `[`.
func (tr *transformer) lexTag(i int) int {
	in := tr.input
	j := i + 1
	if j < len(in) && in[j] == '[' { // "[[" is a literal '['
		tr.text(i, i+1)
		return i + 2
	}
	k := j
	for k < len(in) && (in[k] == ' ' || in[k] == '\t') {
		k++
	}
	n := k
	for n < len(in) && in[n] != ']' && in[n] != ' ' && in[n] != '\t' && in[n] != '\n' && in[n] != '[' {
		n++
	}
	if n >= len(in) || in[n] == '\n' || in[n] == '[' {
		return tr.bracket(i)
	}
	name := in[k:n]
	base := strings.TrimPrefix(name, "/")
	closing := len(base) < len(name)
	if base == "" {
		return tr.bracket(i)
	}
	def, known := lookupTag(base)
	if k > j && !known {
		return tr.bracket(i)
	}
	var attrs map[string]string
	end := n + 1
	if in[n] != ']' {
		var ok bool
		if attrs, end, ok = scanAttrs(in, n); !ok {
			return tr.bracket(i)
		}
	}
	switch {
	case closing:
		switch def.kind {
		case kVoid, kMedia, kIgnore: // no zone to close
		default:
			tr.push(token{kind: tkClose, s: base, rng: -1})
			if base == tr.raw {
				tr.raw = ""
			}
		}
	case def.kind == kVoid:
		tr.html(def.open)
	case def.kind == kIgnore:
	case def.kind == kMedia:
		return tr.lexMedia(end)
	default:
		if n := len(tr.toks); def.kind == kBlock && n > 0 && tr.toks[n-1].kind == tkBreak && tr.toks[n-1].a == int32(i) {
			tr.toks[n-1].s = "" // the paragraph breaks the line
		}
		tr.push(token{kind: tkOpen, s: base, attrs: attrs, a: int32(i), b: int32(end), rng: -1})
		if def.kind == kLink {
			tr.raw = base // a link ends the one it opens in (resolve)
		}
	}
	return end
}

// bracket is a `[` that opens no tag: text, unless it is the `[/` of a
// closing tag that lost its name to the next tag (`[/[m1]`), which is dropped.
func (tr *transformer) bracket(i int) int {
	if in := tr.input; i+2 < len(in) && in[i+1] == '/' && in[i+2] == '[' {
		return i + 2
	}
	tr.text(i, i+1)
	return i + 1
}

// scanAttrs parses the attributes of a tag from in[i] (the blank after the
// name) through the closing ']'. Values may be quoted ('…' or "…") or bare,
// and `\` escapes inside them; a bare attribute with no `=` is recorded with
// an empty value - which is how `[c red]` names its colour. It fails, and the
// tag with it, at a line break, an unescaped `[`, or the end of input.
func scanAttrs(in string, i int) (map[string]string, int, bool) {
	attrs := map[string]string{}
	name := -1 // start of the attribute name being read
	flushName := func() {
		if name >= 0 {
			attrs[in[name:i]] = ""
			name = -1
		}
	}
	for i < len(in) {
		switch c := in[i]; c {
		case '\n', '[':
			return nil, 0, false
		case ']':
			flushName()
			return attrs, i + 1, true
		case ' ', '\t':
			flushName()
			i++
		case '=':
			key := ""
			if name >= 0 {
				key = in[name:i]
				name = -1
			}
			i++
			for i < len(in) && (in[i] == ' ' || in[i] == '\t') {
				i++
			}
			v, next, ok := scanAttrValue(in, i)
			if !ok {
				return nil, 0, false
			}
			attrs[key] = v
			i = next
		default:
			if name < 0 {
				name = i
			}
			i++
		}
	}
	return nil, 0, false
}

func scanAttrValue(in string, i int) (string, int, bool) {
	quote := byte(0)
	if i < len(in) && (in[i] == '\'' || in[i] == '"') {
		quote = in[i]
		i++
	}
	var v strings.Builder
	for i < len(in) {
		c := in[i]
		switch {
		case c == '\\':
			if i+1 >= len(in) || in[i+1] == '\n' {
				return "", 0, false
			}
			v.WriteByte(in[i+1])
			i += 2
		case c == ']':
			return v.String(), i, true
		case c == '\n', c == '[':
			return "", 0, false
		case quote != 0 && c == quote:
			return v.String(), i + 1, true
		case quote == 0 && (c == ' ' || c == '\t'):
			return v.String(), i, true
		default:
			v.WriteByte(c)
			i++
		}
	}
	return "", 0, false
}

// flipCaseFirst inverts the case of the first rune of s, the "перевёртыш"
// operation behind the "^" command.
func flipCaseFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	f := unicode.ToLower(r)
	if unicode.IsLower(r) {
		f = unicode.ToUpper(r)
	}
	return string(f) + s[size:]
}

var (
	textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

func escape(s string) string { return textEscaper.Replace(s) }

// quoteAttr renders a value as a complete double-quoted attribute. Every
// attribute built from dictionary content must go through this and not
// escape(): a `"` inside a colour name or a media file name would otherwise
// close the attribute and start a new one.
func quoteAttr(s string) string { return `"` + attrEscaper.Replace(s) + `"` }

// colourOpen is the [c] markup. The author's colour is a PARAMETER, not a
// decision: it rides as --wd-c and internal/artmark's rule turns it into a
// colour, so a reader who sets .wu-c{color:...} beats it without !important.
// A bare [c] is green (lingvo-ref), which is the rule's fallback and so costs
// no attribute at all.
func colourOpen(attrs map[string]string) string {
	color := ""
	for k, v := range attrs {
		if v == "" {
			color = k
			break
		}
	}
	if !artmark.IsColor(color) {
		// A name or a #hex, or nothing. Dictionary content reaching a style
		// attribute is an injection site, and a value like
		// `red;position:fixed` would be exactly that.
		return `<span class="wu-c">`
	}
	return `<span class="wu-c" style=` + quoteAttr("--wd-c:"+color) + `>`
}

// marginOpen is the [mN] markup. isMarginTag has already guaranteed the tail
// is digits, so nothing but a number can reach the custom property.
func marginOpen(tag string) string {
	if n := tag[1:]; n != "" && n != "0" {
		return `<p class="wu-m" style=` + quoteAttr("--wd-m:"+n) + `>`
	}
	return `<p class="wu-m">`
}

// mediaKind is what a browser can do with one [s] payload.
type mediaKind int

const (
	mediaFile mediaKind = iota // the default: hand it over as a link
	mediaAudio
	mediaImage
	mediaVideo
)

// mediaExt classifies the media zone by extension. Lingvo's [s] supports
// images, sound AND video, and an author may
// put anything else in the .files folder besides - a PDF plate, a document.
// Only what a browser can actually render or decode is named here; everything
// else is deliberately absent and becomes a file link.
//
// The formats Lingvo names but a browser cannot handle are the reason this is
// an allowlist rather than a "video/ prefix" test: .avi, .wmv, .flv, .mkv and
// .mpg are Lingvo's and GoldenDict's own video formats, and an inline <video>
// pointed at one is a permanently broken player. As a link they reach the
// system player instead, which is exactly what Lingvo does with them. Same
// for its .pcx, .dcx, .wmf and .emf images, which no browser draws.
var mediaExt = map[string]mediaKind{
	// Sound. Lingvo documents .wav alone; the rest arrive from GoldenDict-era
	// dictionaries, and .spx is transcoded to WAV when served (D18).
	"wav": mediaAudio, "mp3": mediaAudio, "ogg": mediaAudio,
	"spx": mediaAudio, "m4a": mediaAudio,
	// Images.
	"bmp": mediaImage, "gif": mediaImage, "ico": mediaImage,
	"jpeg": mediaImage, "jpg": mediaImage, "png": mediaImage,
	"svg": mediaImage, "tif": mediaImage, "tiff": mediaImage,
	"webp": mediaImage, "avif": mediaImage,
	// Video.
	"mp4": mediaVideo, "webm": mediaVideo, "ogv": mediaVideo,
	"mov": mediaVideo, "m4v": mediaVideo, "3gp": mediaVideo,
}

// lexMedia reads the file name filling a media zone ([s] or [video]) from
// in[i] and emits one element for it, its name recorded for the resource set.
// Only a bare name with an extension is legal there - no path, no nested
// tags, no spaces around it (lingvo-ref) - so the scan stops at the first `[`
// or line break, and the one tag the compiler does accept inside is skipped:
// [preview] was rejected during x5's development and has no effect, but a
// dictionary written against that compiler still contains it, and reading it
// as part of the name would ask the container for a file called
// "[preview]x.avi". The [/s] that follows closes no zone and is dropped.
func (tr *transformer) lexMedia(i int) int {
	in := tr.input
	var b strings.Builder
	for i < len(in) {
		if strings.HasPrefix(in[i:], "[preview]") {
			i += len("[preview]")
			continue
		}
		if strings.HasPrefix(in[i:], "[/preview]") {
			i += len("[/preview]")
			continue
		}
		if in[i] == '[' || in[i] == '\n' {
			break
		}
		b.WriteByte(in[i])
		i++
	}
	if fname := strings.TrimSpace(b.String()); fname != "" {
		tr.media(fname)
	}
	return i
}

// media emits the element for one media file. Every kind renders something.
// Emitting nothing for an unrecognised extension would lose the file
// silently: the article would show a gap where the author put a video or a
// PDF, and no part of the pipeline downstream can recover a reference that
// was never written.
func (tr *transformer) media(fname string) {
	t := token{kind: tkHTML, rng: -1}
	switch mediaExt[strings.TrimPrefix(strings.ToLower(path.Ext(fname)), ".")] {
	case mediaAudio:
		// A link, not GoldenDict's `<object type="audio/x-wav">`. That spelling
		// is a plugin-era embedding vector: `clean` has to drop it as unsafe
		// and rescue the URL back out (server/articleformat.go audioObject),
		// and both renderers need a handler that exists for this one element.
		// An anchor needs none of it: the
		// server's rewriter already recognises a media href on <a> and points
		// it at /res/, both renderers already play such a link, `clean` keeps
		// it as an ordinary link, and no inline handler is emitted (the
		// sanitiser strips every on* attribute, by design).
		//
		// [s] carries no link text of its own, so the glyph is the affordance.
		t.s, t.anchor = `<a class="wu-audio" href=`+quoteAttr(fname)+`>&#128266;</a>`, true
	case mediaImage:
		t.s = `<img align="top" src=` + quoteAttr(fname) + ` alt=` + quoteAttr(fname) + ` />`
	case mediaVideo:
		// preload="none" is what makes this affordable: a card may carry
		// several videos of tens of megabytes each, and nothing is fetched
		// until the reader presses play. src is a fetch site, so the article
		// rewriter points it at /res/{dict}/ with no special case, and `clean`
		// already keeps <video src|controls> (server/articleformat.go).
		t.s = `<video class="wu-video" controls preload="none" src=` + quoteAttr(fname) + `></video>`
	default:
		// Anything else - a PDF, a document, one of Lingvo's own formats no
		// browser handles. The `file://` pseudo-scheme is the author saying
		// "this names MY file": the rewriter honours it (server/rewrite.go
		// isResourceRef) and rewrites the link to /res/ whatever the
		// extension, so a dictionary's own container is served in full without
		// widening dict.IsAssetName - which is also the allowlist for files
		// lying LOOSE beside an .mdx, a boundary this must not touch.
		//
		// The file name is the link text because it is all there is: [s] has no
		// text of its own, and a bare glyph would not say what it opens.
		t.s, t.anchor = `<a class="wu-file" href=`+quoteAttr("file://"+fname)+
			`>&#128196; `+escape(fname)+`</a>`, true
	}
	tr.push(t)
	tr.resFiles = append(tr.resFiles, fname)
}
