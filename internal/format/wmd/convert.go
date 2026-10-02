// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	gast "github.com/yuin/goldmark/v2/ast"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/htmlref"
)

// The `clean` writer mode (spec R6.6-R6.7): an article's HTML reduced to what
// CommonMark writes natively, in one walk over the parsed fragment. Cleanup
// is destructive by design - anything outside the allowlist is unwrapped or
// dropped - so a conversion never has to decide between two markdown forms.
// The result is then parsed by the stock parser the reader uses, which proves
// the body cannot split the entry (R6.8).

// CleanError is a body `clean` mode cannot write (R6.8).
type CleanError struct {
	Construct, Reason string
}

func (e *CleanError) Error() string {
	return e.Construct + " cannot be written as clean markdown: " + e.Reason
}

// maxNest bounds list and quote nesting; deeper levels are unwrapped into
// their parent's flow, since every level costs every line an indent.
const maxNest = 16

type blockKind int

const (
	kPara blockKind = iota
	kList
	kOther
)

type block struct {
	md   string
	kind blockKind
	list atom.Atom // for kList: Ul or Ol, so a following list can alternate
	// For kList: it can interrupt a paragraph - its first item is not empty
	// and, if ordered, starts at 1 (CommonMark 5.2). One that cannot must
	// not follow a paragraph without a blank line.
	interrupts bool
}

// cleanBody converts an article's HTML to `clean` markdown. st is the
// dictionary's display table (R6.6, htmlref.ParseCSS over the stylesheets its
// articles link), nil when it has none.
func cleanBody(src string, st htmlref.Styles) (md string, err error) {
	defer func() {
		if p := recover(); p != nil {
			md, err = "", &CleanError{Construct: "the article", Reason: fmt.Sprintf("internal error: %v", p)}
		}
	}()
	src = validText(src)
	nodes, perr := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if perr != nil {
		return "", &CleanError{Construct: "the article", Reason: perr.Error()}
	}
	c := &converter{st: st}
	md = joinBlocks(c.flow(nodes, 0))
	if !strings.Contains(md, "\n") && strings.HasPrefix(md, "see:") {
		md = `see\:` + md[len("see:"):] // would read as a redirect (R3.6)
	}
	if md != "" {
		if err := splitsEntry(md); err != nil {
			return "", err
		}
	}
	return md, nil
}

// validText repairs a body as the reader repairs a file (R2.1): each byte of
// invalid UTF-8, and NUL, become U+FFFD. The writer never emits what the
// reader would have to repair.
func validText(s string) string {
	return strings.ReplaceAll(validUTF8(s), "\x00", "\uFFFD")
}

// validUTF8 is s with each byte of invalid UTF-8 replaced by U+FFFD, as a
// []rune conversion does.
func validUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return string([]rune(s))
}

// splitsEntry parses md as the reader will and refuses a top-level level-1 or
// level-2 heading: it would end the entry (R3.5).
func splitsEntry(md string) error {
	doc := mdParser.Parse([]byte(md))
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*gast.Heading); ok && h.Level <= 2 {
			return &CleanError{Construct: "a heading", Reason: "it would read as the start of an entry"}
		}
	}
	return nil
}

func joinBlocks(bs []block) string {
	var parts []string
	for _, b := range bs {
		parts = append(parts, b.md)
	}
	return strings.Join(parts, "\n\n")
}

type converter struct {
	st    htmlref.Styles
	holds map[*html.Node]bool // holdsBlock, memoized
}

// display is the layout n's classes are given by the dictionary's stylesheet:
// htmlref.DisplayUnset without one, or for a node that is no HTML element.
func display(st htmlref.Styles, n *html.Node) htmlref.Display {
	if len(st) == 0 || n.Type != html.ElementNode || n.Namespace != "" {
		return htmlref.DisplayUnset
	}
	class, _ := getAttr(n, "class")
	return st.Class(class)
}

// skipped elements go with their content: dropped by tag, or hidden by the
// stylesheet - what the dictionary does not show is not part of the text.
func skipped(st htmlref.Styles, n *html.Node) bool {
	return dropped(n) || display(st, n) == htmlref.DisplayNone
}

// styledBlock reports an element its tag makes inline and the stylesheet
// lays out as a block: it keeps the boundary a reader of the page sees.
func styledBlock(st htmlref.Styles, n *html.Node) bool {
	return !blockElem(n) && display(st, n) == htmlref.DisplayBlock
}

// formatting elements are those the inline writer turns into markup of
// their own; any other inline element - a link without a destination too -
// only wraps its content.
func formatting(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Em, atom.I, atom.Strong, atom.B, atom.Sup, atom.Sub, atom.U, atom.Small, atom.Del, atom.S,
		atom.Strike, atom.Ins, atom.Code, atom.Kbd, atom.Samp, atom.Tt, atom.Img, atom.Audio,
		atom.Video, atom.Source, atom.Object:
		return true
	case atom.A:
		return linkHref(n) != ""
	}
	return false
}

// holdsBlock reports an element with a block - by tag or by the stylesheet -
// among its descendants, outside anything skipped.
func (c *converter) holdsBlock(n *html.Node) bool {
	if v, ok := c.holds[n]; ok {
		return v
	}
	v := false
	for k := n.FirstChild; k != nil && !v; k = k.NextSibling {
		if k.Type == html.ElementNode && !skipped(c.st, k) {
			v = blockElem(k) || styledBlock(c.st, k) || c.holdsBlock(k)
		}
	}
	if c.holds == nil {
		c.holds = map[*html.Node]bool{}
	}
	c.holds[n] = v
	return v
}

func (c *converter) newInline(m inlineMode) *inline {
	w := newInline(m)
	w.st = c.st
	return w
}

func isElem(n *html.Node, a atom.Atom) bool {
	return n.Type == html.ElementNode && n.Namespace == "" && n.DataAtom == a
}

// dropped elements go with their content.
func dropped(n *html.Node) bool {
	if n.Type == html.CommentNode || n.Type == html.DoctypeNode {
		return true
	}
	if n.Type != html.ElementNode {
		return false
	}
	if n.Namespace != "" { // svg, math: no text worth a dictionary reader
		return true
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Template, atom.Iframe, atom.Noscript, atom.Noembed, atom.Noframes,
		atom.Input, atom.Select, atom.Textarea, atom.Button, atom.Embed, atom.Head, atom.Meta, atom.Link,
		atom.Title, atom.Base, atom.Frame, atom.Frameset, atom.Canvas, atom.Map:
		return true
	case atom.Object:
		return mediaSrc(n) == ""
	}
	return false
}

// unwrapped elements are paragraph boundaries whose content joins the flow
// around them.
func unwrapBlock(n *html.Node) bool {
	switch n.DataAtom {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.Main, atom.Header, atom.Footer, atom.Aside,
		atom.Nav, atom.Figure, atom.Figcaption, atom.Address, atom.Center, atom.Hgroup, atom.Fieldset,
		atom.Legend, atom.Dl, atom.Dt, atom.Dd, atom.Form, atom.Body, atom.Html, atom.Caption, atom.Li,
		atom.Tr, atom.Td, atom.Th, atom.Thead, atom.Tbody, atom.Tfoot, atom.Summary, atom.Menu, atom.Dir,
		atom.Listing, atom.Xmp, atom.Plaintext, atom.Search, atom.Dialog, atom.Option, atom.Optgroup:
		return true
	}
	return false
}

// blockElem reports an element that starts its own markdown block.
func blockElem(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Namespace != "" {
		return false
	}
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol, atom.Blockquote, atom.Pre,
		atom.Hr, atom.Table, atom.Details:
		return true
	}
	return unwrapBlock(n)
}

// flow converts a sequence of sibling nodes into blocks.
func (c *converter) flow(nodes []*html.Node, depth int) []block {
	var out []block
	para := c.newInline(inPara)
	flush := func() {
		if md := para.done(); md != "" {
			out = append(out, block{md: md, kind: kPara})
		}
		para = c.newInline(inPara)
	}
	add := func(b block) {
		// Two lists of a kind in a row stay two only if their markers differ:
		// the second takes the other character.
		if b.kind == kList && len(out) > 0 {
			if p := out[len(out)-1]; p.kind == kList && p.list == b.list && listMark(p.md) == listMark(b.md) {
				b.md = alternate(b.md)
			}
		}
		out = append(out, b)
	}
	var walk func(nodes []*html.Node)
	walk = func(nodes []*html.Node) {
		for _, n := range nodes {
			switch {
			case skipped(c.st, n):
			case styledBlock(c.st, n) && formatting(n):
				// Its own paragraph, its markup kept.
				flush()
				para.node(n)
				flush()
			case styledBlock(c.st, n):
				flush()
				for _, b := range c.flow(children(n), depth) {
					add(b)
				}
			case !blockElem(n) && !formatting(n) && c.holdsBlock(n):
				// A wrapper around a block is laid out as the blocks and
				// the runs of text between them (CSS block-in-inline): its
				// content joins this flow.
				walk(children(n))
			case !blockElem(n):
				para.node(n)
			case (isElem(n, atom.Ul) || isElem(n, atom.Ol) || isElem(n, atom.Blockquote)) && depth >= maxNest:
				flush()
				for _, b := range c.flow(children(n), depth) {
					add(b)
				}
			case unwrapBlock(n):
				flush()
				for _, b := range c.flow(children(n), depth) {
					add(b)
				}
			default:
				flush()
				if b, ok := c.blockOf(n, depth); ok {
					add(b)
				}
			}
		}
	}
	walk(nodes)
	flush()
	return out
}

func children(n *html.Node) []*html.Node {
	var out []*html.Node
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		out = append(out, k)
	}
	return out
}

func (c *converter) blockOf(n *html.Node, depth int) (block, bool) {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		w := c.newInline(inHeading)
		w.nodes(children(n))
		t := w.done()
		if t == "" {
			return block{}, false
		}
		// R6.2: a trailing run of `#` after WS would be read as a closing sequence.
		if j := len(strings.TrimRight(t, "#")); j < len(t) && (j == 0 || t[j-1] == ' ') {
			t = t[:j] + `\` + t[j:]
		}
		level := max(int(n.Data[1]-'0'), 3)
		return block{md: strings.Repeat("#", level) + " " + t, kind: kOther}, true
	case atom.Hr:
		return block{md: "***", kind: kOther}, true
	case atom.Pre:
		return block{md: codeBlock(n), kind: kOther}, true
	case atom.Blockquote:
		inner := joinBlocks(c.flow(children(n), depth+1))
		if inner == "" {
			return block{}, false
		}
		return block{md: prefixLines(inner, "> ", ">"), kind: kOther}, true
	case atom.Ul, atom.Ol:
		md, interrupts := c.list(n, depth+1)
		if md == "" {
			return block{}, false
		}
		return block{md: md, kind: kList, list: n.DataAtom, interrupts: interrupts}, true
	case atom.Table:
		md := c.table(n)
		if md == "" {
			return block{}, false
		}
		return block{md: md, kind: kOther}, true
	case atom.Details:
		return block{md: c.details(n, depth), kind: kOther}, true
	}
	return block{}, false
}

// prefixLines puts p before every line, or empty on an empty line.
func prefixLines(s, p, empty string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = empty
		} else {
			lines[i] = p + l
		}
	}
	return strings.Join(lines, "\n")
}

func codeBlock(pre *html.Node) string {
	text := textContent(pre)
	text = strings.TrimSuffix(text, "\n")
	info := ""
	for k := pre.FirstChild; k != nil; k = k.NextSibling {
		if isElem(k, atom.Code) {
			for _, a := range k.Attr {
				if a.Key == "class" {
					for _, cl := range strings.Fields(a.Val) {
						if l, ok := strings.CutPrefix(cl, "language-"); ok && l != "" && !strings.ContainsAny(l, "`~") {
							info = l
						}
					}
				}
			}
		}
	}
	fence := strings.Repeat("`", max(3, longestRun(text, '`')+1))
	var lines []string
	if text != "" {
		for _, l := range strings.Split(text, "\n") {
			lines = append(lines, strings.TrimRight(l, "\r"))
		}
	}
	return fence + info + "\n" + strings.Join(append(lines, fence), "\n")
}

func longestRun(s string, c byte) int {
	best, cur := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			if n.DataAtom == atom.Br && n.Namespace == "" {
				b.WriteByte('\n')
				return
			}
			if dropped(n) {
				return
			}
			for k := n.FirstChild; k != nil; k = k.NextSibling {
				walk(k)
			}
		}
	}
	walk(n)
	return b.String()
}

// list writes ul/ol (R6.6): tight unless an item holds more than a paragraph
// and its sub-lists.
func (c *converter) list(n *html.Node, depth int) (string, bool) {
	// Each item's nodes first - an li's children, and any stray content,
	// which joins the item before it - then each item converted in one flow,
	// so two lists in a row inside it alternate their markers too.
	var groups [][]*html.Node
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		switch {
		case skipped(c.st, k), k.Type == html.TextNode && strings.TrimSpace(k.Data) == "":
		case isElem(k, atom.Li):
			groups = append(groups, children(k))
		case len(groups) == 0:
			groups = append(groups, []*html.Node{k})
		default:
			groups[len(groups)-1] = append(groups[len(groups)-1], k)
		}
	}
	// An empty item is dropped: it holds nothing, and a bare marker is where
	// parsers disagree (goldmark and CommonMark read `- 1.` + blank + text
	// differently).
	var items [][]block
	for _, g := range groups {
		if bs := c.flow(g, depth); len(bs) > 0 {
			items = append(items, bs)
		}
	}
	if len(items) == 0 {
		return "", false
	}
	// Tight when every item is one block, or a paragraph and its sub-lists.
	tight := true
	for _, it := range items {
		if len(it) < 2 {
			continue
		}
		for _, b := range it[1:] {
			if b.kind != kList || it[0].kind != kPara || !b.interrupts {
				tight = false
			}
		}
	}
	start := 1
	if isElem(n, atom.Ol) {
		for _, a := range n.Attr {
			if a.Key == "start" {
				if v, err := strconv.Atoi(strings.TrimSpace(a.Val)); err == nil && v >= 0 && v <= 999999999 {
					start = v
				}
			}
		}
	}
	sep := "\n"
	if !tight {
		sep = "\n\n"
	}
	out := make([]string, len(items))
	for i, it := range items {
		marker := "-"
		if isElem(n, atom.Ol) {
			marker = strconv.Itoa(start+i) + "."
		}
		var parts []string
		for _, b := range it {
			parts = append(parts, b.md)
		}
		body := strings.Join(parts, sep)
		if body == "" {
			out[i] = marker
			continue
		}
		pad := strings.Repeat(" ", len(marker)+1)
		lines := strings.Split(body, "\n")
		for j := 1; j < len(lines); j++ {
			if lines[j] != "" {
				lines[j] = pad + lines[j]
			}
		}
		out[i] = marker + " " + strings.Join(lines, "\n")
	}
	return strings.Join(out, sep), len(items[0]) > 0 && (isElem(n, atom.Ul) || start == 1)
}

// listMark is the marker character of a list's first item: `-`, `*`, `.`
// or `)`.
func listMark(md string) byte {
	j := 0
	for j < len(md) && md[j] >= '0' && md[j] <= '9' {
		j++
	}
	if j < len(md) {
		return md[j]
	}
	return 0
}

// alternate switches a list's markers to the other character (`-` and `*`,
// `.` and `)`), so it does not join the list right before it.
func alternate(md string) string {
	from := listMark(md)
	to := map[byte]byte{'-': '*', '*': '-', '.': ')', ')': '.'}[from]
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		if l == "" || l[0] == ' ' {
			continue // blank, or inside an item
		}
		j := 0
		for j < len(l) && l[j] >= '0' && l[j] <= '9' {
			j++
		}
		if j < len(l) && l[j] == from {
			lines[i] = l[:j] + string(to) + l[j+1:]
		}
	}
	return strings.Join(lines, "\n")
}

func (c *converter) details(n *html.Node, depth int) string {
	var summary string
	var rest []*html.Node
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		if isElem(k, atom.Summary) && summary == "" {
			summary = strings.Join(strings.Fields(textContent(k)), " ")
			continue
		}
		rest = append(rest, k)
	}
	var b strings.Builder
	b.WriteString("<details>\n")
	if summary != "" {
		b.WriteString("<summary>" + html.EscapeString(summary) + "</summary>\n")
	}
	if inner := joinBlocks(c.flow(rest, depth)); inner != "" {
		b.WriteString("\n" + inner + "\n\n")
	}
	b.WriteString("</details>")
	return b.String()
}

// table writes a GFM table: the first row is the header; block content in a
// cell is flattened with <br>.
func (c *converter) table(t *html.Node) string {
	var rows [][]*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			switch {
			case skipped(c.st, k):
			case isElem(k, atom.Tr):
				var cells []*html.Node
				for cell := k.FirstChild; cell != nil; cell = cell.NextSibling {
					if isElem(cell, atom.Td) || isElem(cell, atom.Th) {
						cells = append(cells, cell)
					}
				}
				rows = append(rows, cells)
			case isElem(k, atom.Thead), isElem(k, atom.Tbody), isElem(k, atom.Tfoot):
				walk(k)
			}
		}
	}
	walk(t)
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return ""
	}
	line := func(r []*html.Node) string {
		var b strings.Builder
		b.WriteString("|")
		for i := 0; i < cols; i++ {
			md := ""
			if i < len(r) && !skipped(c.st, r[i]) {
				w := c.newInline(inCell)
				w.nodes(children(r[i]))
				md = w.done()
			}
			if md == "" {
				b.WriteString(" |")
			} else {
				b.WriteString(" " + md + " |")
			}
		}
		return b.String()
	}
	out := []string{line(rows[0])}
	delim := "|"
	for i := 0; i < cols; i++ {
		a := ""
		if i < len(rows[0]) {
			a = cellAlign(rows[0][i])
		}
		delim += " " + map[string]string{"": "---", "left": ":--", "center": ":-:", "right": "--:"}[a] + " |"
	}
	out = append(out, delim)
	for _, r := range rows[1:] {
		out = append(out, line(r))
	}
	return strings.Join(out, "\n")
}

func cellAlign(c *html.Node) string {
	for _, a := range c.Attr {
		v := strings.ToLower(strings.TrimSpace(a.Val))
		switch a.Key {
		case "align":
			if v == "left" || v == "center" || v == "right" {
				return v
			}
		case "style":
			for _, decl := range strings.Split(v, ";") {
				if k, val, ok := strings.Cut(decl, ":"); ok && strings.TrimSpace(k) == "text-align" {
					if val = strings.TrimSpace(val); val == "left" || val == "center" || val == "right" {
						return val
					}
				}
			}
		}
	}
	return ""
}

// mediaSrc is the file an audio, video, source or pronunciation object
// plays, or "".
func mediaSrc(n *html.Node) string {
	attr := func(n *html.Node, k string) string {
		for _, a := range n.Attr {
			if a.Key == k {
				return strings.TrimSpace(a.Val)
			}
		}
		return ""
	}
	switch n.DataAtom {
	case atom.Audio, atom.Video, atom.Source:
		if s := attr(n, "src"); s != "" {
			return s
		}
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			if isElem(k, atom.Source) {
				if s := attr(k, "src"); s != "" {
					return s
				}
			}
		}
	case atom.Object:
		if t := strings.ToLower(attr(n, "type")); strings.HasPrefix(t, "audio/") || strings.HasPrefix(t, "video/") {
			return attr(n, "data")
		}
	}
	return ""
}

// Inline writing (R6.7).

type inlineMode int

const (
	inPara inlineMode = iota
	inHeading
	inCell
)

type inline struct {
	st        htmlref.Styles
	mode      inlineMode
	b         strings.Builder
	lineStart bool  // at the start of a line: block syntax is possible here
	starts    []int // offsets of the lines that start with text
	space     bool  // a pending space
	brk       bool  // a pending hard break
	lead      bool  // space before the first content: the parent writes it
}

func newInline(m inlineMode) *inline { return &inline{mode: m, lineStart: true} }

// sub is a writer for the content of an element inside this one's text.
func (w *inline) sub() *inline {
	s := newInline(w.mode)
	s.st, s.lineStart = w.st, false
	return s
}

// done returns the markdown, pending space and a trailing break dropped, with
// the line-start escapes of R6.7 in place. They are decided on each finished
// line, not per text node: whether `1.` starts a list depends on what follows
// it, which may come from the next node.
func (w *inline) done() string {
	s := w.b.String()
	for i := len(w.starts) - 1; i >= 0; i-- {
		if at := lineStartEscape(s, w.starts[i]); at >= 0 {
			s = s[:at] + `\` + s[at:]
		}
	}
	return s
}

// lineStartEscape returns where a line starting at off needs a backslash to
// stay text, or -1: before `#`, `+`, `-`, `=` or `:`, and before the delimiter of
// a 1-9 digit run that CommonMark would read as an ordered list marker.
func lineStartEscape(s string, off int) int {
	if off >= len(s) {
		return -1
	}
	switch s[off] {
	case '#', '+', '-', '=', ':': // `:` or `-` could start a table delimiter row
		return off
	}
	j := off
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if n := j - off; n >= 1 && n <= 9 && j < len(s) && (s[j] == '.' || s[j] == ')') {
		if k := j + 1; k == len(s) || s[k] == ' ' || s[k] == '\t' || s[k] == '\n' {
			return j
		}
	}
	return -1
}

func (w *inline) last() byte {
	s := w.b.String()
	if s == "" {
		return 0
	}
	return s[len(s)-1]
}

// spaced records a space. One before any content is also the writer's lead:
// an element's markup starts at its first visible character, and the space
// goes before it, in the parent (hoist).
func (w *inline) spaced() {
	w.space = true
	if w.b.Len() == 0 {
		w.lead = true
	}
}

// hoist moves the space at either edge of an element's content, written by
// sub, outside the element's markup: before must be called before the markup
// is written, and after it.
func (w *inline) hoist(sub *inline, before bool) {
	if before && sub.lead || !before && sub.space {
		w.spaced()
	}
}

// pending writes the space or hard break waiting before visible content.
func (w *inline) pending() {
	if w.b.Len() == 0 {
		w.space, w.brk = false, false
		return
	}
	switch {
	case w.brk:
		switch w.mode {
		case inPara:
			// A text backslash right before the break would make `\\\`
			// + LF, which goldmark does not read as CommonMark says (a
			// `\` and a break): the break is then a tag, read alike by all.
			if w.last() == '\\' {
				w.b.WriteString("<br>\n")
			} else {
				w.b.WriteString("\\\n")
			}
			w.lineStart = true
		case inCell:
			w.b.WriteString("<br>")
		default:
			w.b.WriteByte(' ')
		}
	case w.space:
		w.b.WriteByte(' ')
	}
	w.space, w.brk = false, false
}

// syntax writes markup: never escaped, never a line start.
func (w *inline) syntax(s string) {
	w.pending()
	w.b.WriteString(s)
	w.lineStart = false
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' }

func (w *inline) text(s string) {
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if isSpace(r) {
			w.spaced()
			i += n
			continue
		}
		w.pending()
		if w.lineStart && w.mode == inPara {
			w.starts = append(w.starts, w.b.Len())
		}
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<', '>', '&', '~', '|':
			w.b.WriteByte('\\')
		}
		w.b.WriteRune(r)
		w.lineStart = false
		i += n
	}
}

func (w *inline) nodes(ns []*html.Node) {
	for _, n := range ns {
		w.node(n)
	}
}

// hasDelimiter reports an unescaped `*` in md: nested emphasis whose
// delimiters an enclosing `*` pair could bind to instead.
func hasDelimiter(md string) bool {
	for i := 0; i < len(md); i++ {
		switch md[i] {
		case '\\':
			i++ // the escaped character
		case '*':
			return true
		}
	}
	return false
}

// markedRune reports whether r lets a `*` delimiter bind whatever is around
// it: a letter, digit or mark - not space, punctuation or symbol.
func markedRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

func (w *inline) node(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		w.text(n.Data)
		return
	case html.ElementNode:
	default:
		return
	}
	if skipped(w.st, n) {
		return
	}
	if n.Namespace != "" {
		return
	}
	if styledBlock(w.st, n) {
		// Inside a paragraph, heading or cell a block is set off by line
		// breaks, as a tag-made block is below.
		if w.b.Len() > 0 {
			w.brk = true
		}
		defer func() { w.brk = true }()
	} else if class, _ := getAttr(n, "class"); w.st.Gap(class) {
		// Set apart by the stylesheet: by a space in the text.
		w.spaced()
		defer func() { w.space = true }()
	}
	switch n.DataAtom {
	case atom.Br:
		if w.b.Len() > 0 {
			w.brk = true
		}
	case atom.Em, atom.I, atom.Strong, atom.B:
		d := "*"
		if n.DataAtom == atom.Strong || n.DataAtom == atom.B {
			d = "**"
		}
		sub := w.sub()
		sub.nodes(children(n))
		inner := sub.done()
		w.hoist(sub, true)
		defer w.hoist(sub, false)
		if inner == "" {
			return
		}
		first, _ := utf8.DecodeRuneInString(inner)
		lastR, _ := utf8.DecodeLastRuneInString(inner)
		tag := "em"
		if d == "**" {
			tag = "strong"
		}
		w.pending()
		if markedRune(first) && markedRune(lastR) && w.last() != '*' && !hasDelimiter(inner) {
			w.syntax(d + inner + d)
		} else {
			w.syntax("<" + tag + ">" + inner + "</" + tag + ">")
		}
	case atom.Sup, atom.Sub, atom.U, atom.Small, atom.Del, atom.S, atom.Strike, atom.Ins, atom.Kbd:
		tag := n.Data
		if n.DataAtom == atom.S || n.DataAtom == atom.Strike {
			tag = "del"
		}
		sub := w.sub()
		sub.nodes(children(n))
		w.hoist(sub, true)
		if inner := sub.done(); inner != "" {
			w.syntax("<" + tag + ">" + inner + "</" + tag + ">")
		}
		w.hoist(sub, false)
	case atom.Code, atom.Samp, atom.Tt:
		t := strings.Join(strings.FieldsFunc(textContent(n), isSpace), " ")
		if t == "" {
			return
		}
		w.pending()
		if w.last() == '`' {
			// Right after a backtick the span's run would join it and
			// read differently: the tag form, its text escaped, instead.
			w.syntax("<code>")
			w.text(t)
			w.syntax("</code>")
			return
		}
		if w.mode == inCell {
			t = strings.ReplaceAll(t, "|", `\|`)
		}
		f := strings.Repeat("`", shortestAbsent(t))
		if strings.HasPrefix(t, "`") || strings.HasSuffix(t, "`") {
			t = " " + t + " "
		}
		w.syntax(f + t + f)
	case atom.A:
		w.link(n)
	case atom.Img:
		w.image(n)
	case atom.Audio, atom.Video, atom.Source, atom.Object:
		if s := mediaSrc(n); s != "" {
			w.bangGuard()
			w.syntax("[▶](" + w.cellSafe(dest(linkTarget(s)), "") + ")")
		}
	default:
		// Anything else inline is unwrapped: span, font, abbr - and a block
		// element nested inside one of them, or inside a heading or a cell,
		// which keeps its boundaries as line breaks.
		if blockElem(n) && w.b.Len() > 0 {
			w.brk = true
		}
		w.nodes(children(n))
		if blockElem(n) {
			w.brk = true
		}
	}
}

// bangGuard escapes a text `!` right before a link's `[`: `![` is an image.
func (w *inline) bangGuard() {
	if w.space || w.brk || w.b.Len() == 0 {
		return
	}
	s := w.b.String()
	if s[len(s)-1] == '!' && (len(s) < 2 || s[len(s)-2] != '\\') {
		w.b.Reset()
		w.b.WriteString(s[:len(s)-1] + `\!`)
	}
}

func shortestAbsent(t string) int {
	runs := map[int]bool{}
	cur := 0
	for i := 0; i <= len(t); i++ {
		if i < len(t) && t[i] == '`' {
			cur++
			continue
		}
		if cur > 0 {
			runs[cur] = true
		}
		cur = 0
	}
	n := 1
	for runs[n] {
		n++
	}
	return n
}

func getAttr(n *html.Node, k string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == k {
			return a.Val, true
		}
	}
	return "", false
}

// Destinations. A stock renderer percent-encodes what it writes into an href
// (R5.3): spaces, quotes and every non-ASCII byte. So that a file reads the
// same after a round trip, and stays readable, every destination is written
// in one canonical form:
//
//   - a lookup link as `entry://` + the decoded target encoded by
//     htmlref.EncTarget, so `[вода](entry://вода)`, not `%D0%B2…`; an MDict
//     sub-entry keeps its raw `@` (`entry:@sub`);
//   - a link's relative href that names no file is a headword, written as
//     the lookup link it stands for (crossRef);
//   - any other relative path - a resource - decoded, with only `%`, `#`,
//     `?` and controls encoded (R5.4 decodes it again);
//   - any other URL encoded the way the renderer would, so it has nothing
//     left to change.

// linkTarget is the canonical form of an href or src (see above), its
// `sound://` or `file://` pseudo-scheme dropped (R6.6).
func linkTarget(v string) string {
	v = strings.Trim(v, " \t\n\r\f")
	if rest, ok := lookupRest(v); ok {
		return entryRef(rest)
	}
	for _, p := range []string{"sound://", "file://"} {
		if len(v) >= len(p) && strings.EqualFold(v[:len(p)], p) {
			v = v[len(p):]
			break
		}
	}
	if schemeRef(v) || strings.HasPrefix(v, "//") {
		return encURL(v)
	}
	path, suffix := v, ""
	if i := strings.IndexAny(v, "?#"); i >= 0 {
		path, suffix = v[:i], v[i:]
	}
	return encSome(dec(path), "%#?") + encURL(suffix)
}

// entryRef is the canonical lookup link for the text after a lookup scheme:
// the target, and a fragment if there is one.
func entryRef(rest string) string {
	target, frag, hasFrag := strings.Cut(rest, "#")
	var out string
	if len(target) > 1 && target[0] == '@' {
		out = "entry:@" + htmlref.EncTarget(dec(target[1:]))
	} else {
		out = "entry://" + htmlref.EncTarget(trimWS(dec(target)))
	}
	if hasFrag {
		out += "#" + encSome(dec(frag), "%")
	}
	return out
}

// crossRef reports a link href that is a cross-reference without a lookup
// scheme (R5.4, R6.9): relative, not rooted (`/…`, a page of the host), not
// only a fragment or a query, not under `res/` or `assets/`, and naming no
// file - the rule the clients follow links by (index.html, frame.js) and the
// server reads them by (dict.IsAssetName). It is written as the lookup link
// it stands for.
func crossRef(v string) (string, bool) {
	v = strings.Trim(v, " \t\n\r\f")
	if v == "" || v[0] == '#' || v[0] == '?' || v[0] == '/' || schemeRef(v) || dict.IsAssetName(v) ||
		hasPrefixFold(v, "res/") || hasPrefixFold(v, "assets/") {
		return "", false
	}
	if target, _, _ := strings.Cut(v, "#"); trimWS(dec(target)) == "" {
		return "", false
	}
	return entryRef(v), true
}

// lookupRest returns the text after a lookup scheme (R5.2): `entry:` or
// `bword:` with or without `//`, `d:`, `x:`, schemes matched ignoring case.
func lookupRest(href string) (string, bool) {
	for _, p := range [...]string{"entry:", "bword:", "d:", "x:"} {
		if len(href) >= len(p) && strings.EqualFold(href[:len(p)], p) {
			rest := href[len(p):]
			if p == "entry:" || p == "bword:" {
				rest = strings.TrimPrefix(rest, "//")
			}
			return rest, true
		}
	}
	return "", false
}

func schemeRef(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '.' || c == '-'):
		case i > 0 && c == ':':
			return true
		default:
			return false
		}
	}
	return false
}

// dec percent-decodes s when every `%` starts `%HH` and the result is valid
// UTF-8; otherwise s is returned unchanged (R5.2: all or nothing).
func dec(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b = append(b, s[i])
			continue
		}
		if i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2]) {
			return s
		}
		b = append(b, unhex(s[i+1])<<4|unhex(s[i+2]))
		i += 2
	}
	if !utf8.Valid(b) {
		return s
	}
	return string(b)
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

const upperHex = "0123456789ABCDEF"

// encSome percent-encodes the characters of set, and controls.
func encSome(s, set string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c == 0x7F || strings.IndexByte(set, c) >= 0 {
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&15])
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// urlKept are the ASCII characters the stock renderer writes into an href
// unchanged; `&` it writes as an entity, which reads back as `&`. Measured on
// goldmark v2.1.5 util.URLEscape.
const urlKept = "!#$%&'()*+,-./0123456789:;=?@ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz~"

// encURL encodes what the renderer would encode, so it changes nothing more.
func encURL(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x80 && strings.IndexByte(urlKept, c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&15])
		}
	}
	return b.String()
}

// dest writes a canonical destination as markdown: in angle brackets when it
// holds a space or a control character; `&` escaped so no entity is decoded.
func dest(v string) string {
	angle := v == ""
	for i := 0; i < len(v); i++ {
		if v[i] <= ' ' || v[i] == 0x7F {
			angle = true
		}
	}
	if angle {
		return "<" + strings.NewReplacer(`\`, `\\`, "<", `\<`, ">", `\>`, "&", `\&`, "\n", "%0A", "\r", "%0D").Replace(v) + ">"
	}
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`, "<", `\<`, "&", `\&`).Replace(v)
}

func title(n *html.Node) string {
	t, ok := getAttr(n, "title")
	if t = strings.Join(strings.FieldsFunc(t, isSpace), " "); !ok || t == "" {
		return ""
	}
	return ` "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(t) + `"`
}

// linkHref is the canonical destination of a link, or "" when it has none.
func linkHref(n *html.Node) string {
	href, _ := getAttr(n, "href")
	if ref, ok := crossRef(href); ok {
		return ref
	}
	return linkTarget(href)
}

func (w *inline) link(n *html.Node) {
	href := linkHref(n)
	if href == "" {
		w.nodes(children(n)) // an anchor without a destination is a wrapper
		return
	}
	sub := w.sub()
	sub.nodes(children(n))
	text := sub.done()
	w.hoist(sub, true)
	w.bangGuard()
	w.syntax("[" + text + "](" + w.cellSafe(dest(href), title(n)) + ")")
	w.hoist(sub, false)
}

// cellSafe keeps a destination and title from ending a table cell: GFM splits
// a row at every unescaped "|", link syntax included.
func (w *inline) cellSafe(d, t string) string {
	if w.mode != inCell {
		return d + t
	}
	return strings.ReplaceAll(d, "|", "%7C") + strings.ReplaceAll(t, "|", `\|`)
}

func (w *inline) image(n *html.Node) {
	src, _ := getAttr(n, "src")
	src = linkTarget(src)
	alt, _ := getAttr(n, "alt")
	alt = strings.Join(strings.FieldsFunc(alt, isSpace), " ")
	alt = strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(alt)
	if w.mode == inCell {
		alt = strings.ReplaceAll(alt, "|", `\|`)
	}
	if src == "" {
		if alt != "" {
			w.text(alt)
		}
		return
	}
	w.bangGuard()
	w.syntax("![" + alt + "](" + w.cellSafe(dest(src), title(n)) + ")")
}
