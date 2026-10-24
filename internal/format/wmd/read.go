// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	gmhtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/htmlref"
	"github.com/wuweidict/wudict/internal/lang"
	"github.com/wuweidict/wudict/internal/logx"
)

// The reader (spec §3). A stock CommonMark parser with the GFM table
// extension reads the file, and the dictionary is read off the top-level
// blocks of what it builds. Nothing here parses markdown syntax; the only
// line-level reading is lines 1-2 (R3.1), and the header and `see:` lines,
// which the spec defines as literal text.
//
// The file is read in two phases (D182), each parsing every byte once.
//
// Phase 1, inside NewReader, reads the structure: the heading groups, their
// `see:` lines, the swallowed-heading warnings, the reference definitions.
// It parses in CHUNKS, because a syntax tree costs about forty times the text
// it came from, with goldmark's block parsers alone: CommonMark builds the
// block structure before any inline parsing and independently of it and of
// which definitions exist, so the top-level blocks are the full parser's, at a
// fraction of the cost. A chunk ends where a heading group starts, and every
// cut is CONFIRMED by the parser rather than guessed: a chunk's window runs to
// the end of the next chunk's first line, and the cut stands only when that
// parse ends in a top-level ATX heading starting on it that begins its group.
// A top-level heading closes every open block, so parsing onwards from it
// gives the same blocks as parsing the whole file - by induction from the
// file's start. The cuts are chosen ahead (every chunkSize, at a `## ` line
// that does not continue a group), so the chunks are parsed in parallel and
// confirmed in order; a cut the parser rejects (a `## ` line inside a code
// block or an HTML block) is replaced by doubling the window over the cuts
// chosen ahead, so the work stays linear however many such lines a block
// holds. Once every definition is known, heading text is read: most headings
// hold no inline syntax and are their text; the rest are parsed again, their
// lines alone. Then the title, the header and the description.
//
// Phase 2, in Next, renders the bodies, a UNIT of whole heading groups at a
// time - any group start is a valid cut once phase 1 has confirmed the
// structure - on worker goroutines while a person waits on the preparation
// (dict.Urgent), inline otherwise, and always in file order.
//
// Only link reference definitions cross a cut, since CommonMark makes them
// document-wide: every inline parse is given the definitions its text can
// reference, the first definition of a label winning - exactly how a
// whole-file parse resolves them.

var (
	mdParser = parser.New(parser.WithExtensions(extension.TableParser))
	// blockParser is mdParser without its inline parsers: the same block
	// parsers at their stock priorities, the stock definition and table
	// paragraph transformers.
	blockParser = parser.New(parser.WithDefaultParsers(false),
		parser.WithBlockParsers(
			util.Prioritized(parser.NewSetextHeadingParser(), 100),
			util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewATXHeadingParser(), 600),
			util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			util.Prioritized(parser.NewHTMLBlockParser(), 900),
			util.Prioritized(parser.NewParagraphParser(), 1000),
		),
		parser.WithParagraphTransformers(util.Prioritized(parser.LinkReferenceParagraphTransformer, 100)),
		parser.WithExtensions(extension.TableParser))
	mdRenderer = gmhtml.New(gmhtml.WithUnsafe(), gmhtml.WithExtensions(extension.TableHTMLRenderer))
)

// chunkSize is the text a phase-1 chunk aims for, unitSize a phase-2 unit's,
// workersMax the most goroutines either phase runs; variables so tests can
// make them small.
var (
	chunkSize  = 1 << 20
	unitSize   = 256 << 10
	workersMax = 8
)

// maxWarnings bounds what one file keeps; the count past it is kept.
const maxWarnings = 1000

type chunk struct {
	start, end int // byte range in the text
	line       int // line number of start
}

type entry struct {
	names  []string
	unit   int    // the unit its body is rendered in
	slot   int    // its place among that unit's rendered bodies
	see    string // redirect target (R3.6)
	folded bool   // a redirect whose names became aliases of its targets
}

// Reader is the ingest scan. NewReader reads the file's structure, names and
// definitions; Next renders each body from its unit's parse.
type Reader struct {
	src     *source
	path    string // the source the preparation is for: its urgency chooses the lane
	meta    dict.Meta
	defs    map[string]parser.LinkDefinition // every reference definition, the first of each normalized label
	chunks  []chunk
	units   []unit
	entries []entry
	i       int
	warns   []string
	dropped int

	cur    int      // the unit whose bodies are held, -1 before the first
	bodies []string // its rendered bodies, by slot
	rend   *renderer
	ended  bool // the terminal Next released what phase 2 held
}

// NewReader opens a `.wudict.md`, a qualifying `.md`, or their `.gz`/`.dz`.
// A file this reader does not read returns *Error.
func NewReader(path string) (*Reader, error) {
	src, err := openSource(path)
	if err != nil {
		return nil, err
	}
	r, err := readSource(src, path)
	if err != nil {
		src.close()
		return nil, err
	}
	r.meta.Path = path
	return r, nil
}

// read reads decoded text held in memory.
func read(text []byte) (*Reader, error) { return readSource(sourceFromBytes(text), "") }

func cutLine(b []byte) (line, rest []byte) {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[:i], b[i+1:]
	}
	return b, nil
}

func trimWS(s string) string { return strings.Trim(s, " \t") }

// titleLine is R3.1's line 1: `#`, WS, a non-blank title.
func titleLine(l []byte) bool {
	return len(l) > 2 && l[0] == '#' && (l[1] == ' ' || l[1] == '\t') && trimWS(string(l[1:])) != ""
}

// versionLine reports a line that is, or plainly means to be, the `wudict`
// field: the key ignoring case, optional WS, then `:`.
func versionLine(l string) bool {
	const k = "wudict"
	if len(l) < len(k) || !strings.EqualFold(l[:len(k)], k) {
		return false
	}
	rest := strings.TrimLeft(l[len(k):], " \t")
	return rest != "" && rest[0] == ':'
}

// parseField reads `key: value` (R3.2): key [a-z][a-z0-9-]*, `:`, WS, value.
func parseField(l string) (key, val string, ok bool) {
	i := 0
	for i < len(l) && (l[i] >= 'a' && l[i] <= 'z' || i > 0 && (l[i] >= '0' && l[i] <= '9' || l[i] == '-')) {
		i++
	}
	if i == 0 || i+1 >= len(l) || l[i] != ':' || l[i+1] != ' ' && l[i+1] != '\t' {
		return "", "", false
	}
	if val = trimWS(l[i+1:]); val == "" {
		return "", "", false
	}
	return l[:i], val, true
}

// checkVersion is R3.1's line 2.
func checkVersion(line string) error {
	if !versionLine(line) {
		return formatErr("line 2 must be `wudict: 1`, the version of the format")
	}
	key, v, ok := parseField(line)
	if !ok || key != "wudict" {
		return versionErr("line 2 must be `wudict: 1`")
	}
	major, minor, dot := strings.Cut(v, ".")
	if !digits(major) || dot && !digits(minor) {
		return versionErr("line 2 must be `wudict: 1`, not %q", v)
	}
	if strings.TrimLeft(major, "0") != "1" {
		return versionErr("version %s is not supported; this reader reads `wudict: 1`", v)
	}
	return nil
}

func digits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// isH2 reports a heading that starts an entry (R3.5): `## `. A setext one is
// content: a `---` under a line of text is common in hand-written markdown.
func isH2(n ast.Node) bool {
	h, ok := n.(*ast.Heading)
	return ok && h.Level == 2 && h.HeadingKind == ast.HeadingKindATX
}

func isRefDef(n ast.Node) bool { return n.Kind() == ast.KindLinkReferenceDefinition }

// headingText is R3.4: the text content of the parsed inline content.
func headingText(n ast.Node, src []byte) string {
	var b strings.Builder
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch c := c.(type) {
			case *ast.Text:
				b.WriteString(c.Value.Value(src))
				if c.SoftLineBreak() || c.HardLineBreak() {
					b.WriteByte(' ')
				}
			case *ast.CodeSpan:
				b.WriteString(c.Value.Value(src))
			case *ast.AutoLink:
				b.WriteString(c.Label.Value(src))
			case *ast.RawHTML:
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return trimWS(b.String())
}

// sourceLines returns a block's source lines without their line ends.
func sourceLines(n ast.Node, src []byte) []string {
	bl, ok := n.(ast.BlockNode)
	if !ok {
		return nil
	}
	var out []string
	for _, s := range bl.Source() {
		out = append(out, strings.TrimRight(string(s.Bytes(src)), "\n"))
	}
	return out
}

// warn records a W-entry; line is 0 when there is no position.
func (r *Reader) warn(line int, format string, args ...any) {
	if len(r.warns) >= maxWarnings {
		r.dropped++
		return
	}
	msg := fmt.Sprintf(format, args...)
	if line > 0 {
		msg = fmt.Sprintf("%d: %s", line, msg)
	}
	r.warns = append(r.warns, "W-entry: "+msg)
}

// lineAt is the line number of offset off of a text whose first byte is on
// line first.
func lineAt(text []byte, off, first int) int {
	return first + bytes.Count(text[:min(max(off, 0), len(text))], []byte{'\n'})
}

// lineStart is the offset of the start of the line holding off.
func lineStart(text []byte, off int) int {
	return bytes.LastIndexByte(text[:min(max(off, 0), len(text))], '\n') + 1
}

// candidateFrom is the index of the first candidate line starting at or
// after off, or len(cands).
func (r *Reader) candidateFrom(off int) int {
	return sort.Search(len(r.src.cands), func(i int) bool { return r.src.cands[i].at >= off })
}

// groupStart is the first heading of the group n ends: headings with nothing
// but reference definitions between them.
func groupStart(n ast.Node) ast.Node {
	g := n
	for p := n.PreviousSibling(); p != nil && (isH2(p) || isRefDef(p)); p = p.PreviousSibling() {
		if isH2(p) {
			g = p
		}
	}
	return g
}

// entriesFrom is where a chunk's heading groups begin: after the title, the
// header and the description in the first chunk.
func entriesFrom(doc ast.Node, first bool) ast.Node {
	n := doc.FirstChild()
	if first {
		n = n.NextSibling().NextSibling()
		for n != nil && !isH2(n) {
			n = n.NextSibling()
		}
	}
	return n
}

// span is one heading group and its body, in a parse: from its first heading
// at; the body from first up to, not including, stop.
type span struct{ at, first, stop ast.Node }

// spans walks the top-level blocks from n up to stop (R3.5): a run of
// headings and reference definitions, then everything up to the next heading.
func spans(n, stop ast.Node) []span {
	var out []span
	for n != nil && n != stop {
		s := span{at: n}
		for n != nil && n != stop && (isH2(n) || isRefDef(n)) {
			n = n.NextSibling()
		}
		s.first = n
		for n != nil && n != stop && !isH2(n) {
			n = n.NextSibling()
		}
		s.stop = n
		out = append(out, s)
	}
	return out
}

// seeTarget is R3.6's redirect target of a body of one block, or "".
func seeTarget(only ast.Node, src []byte) string {
	if _, ok := only.(*ast.Paragraph); !ok {
		return ""
	}
	ls := sourceLines(only, src)
	if len(ls) != 1 {
		return ""
	}
	if t, ok := strings.CutPrefix(ls[0], "see:"); ok && t != "" && (t[0] == ' ' || t[0] == '\t') && trimWS(t) != "" {
		return trimWS(t)
	}
	return ""
}

// blockSpan is the byte range an HTML block covers in its chunk's text: the
// union of its source lines and the raw lines it holds for rendering.
func blockSpan(hb *ast.HTMLBlock) (lo, hi int) {
	lo = -1
	for _, segs := range [][]text.Segment{hb.Source(), hb.Value.Segments()} {
		for _, sg := range segs {
			if lo < 0 || sg.Start < lo {
				lo = sg.Start
			}
			hi = max(hi, sg.Stop)
		}
	}
	return max(lo, 0), hi
}

// clip shortens a line quoted in a warning.
func clip(s string) string {
	s = strings.TrimRight(s, "\r")
	if len(s) > 60 {
		s = strings.ToValidUTF8(s[:60], "") + "…"
	}
	return s
}

// headBlocks checks the block shape R3.1-R3.2 give lines 1-2 - the title an
// ATX level-1 heading, the header paragraph right under it - and returns the
// header paragraph.
func headBlocks(doc ast.Node, src []byte) (ast.Node, error) {
	line1, _ := cutLine(src)
	title, ok := doc.FirstChild().(*ast.Heading)
	if !ok || title.Level != 1 {
		return nil, formatErr("line 1 must be `# ` and the dictionary title")
	}
	head := title.NextSibling()
	if h, ok := head.(*ast.Heading); ok && h.HeadingKind == ast.HeadingKindSetext {
		return nil, formatErr("the header must end at a blank line: the `---` or `===` line under it makes it a heading")
	}
	if _, ok := head.(*ast.Paragraph); !ok || head.Pos() != len(line1)+1 {
		return nil, formatErr("line 2 must be `wudict: 1`, directly under the title")
	}
	return head, nil
}

// header reads the title, the header fields and the description (R3.1-R3.3)
// from a full parse of the text before the first heading group.
func (r *Reader) header(doc ast.Node, src []byte) error {
	title, ok := doc.FirstChild().(*ast.Heading)
	if !ok || title.Level != 1 {
		return formatErr("line 1 must be `# ` and the dictionary title")
	}
	name := headingText(title, src)
	if name == "" {
		return formatErr("the title on line 1 is empty")
	}
	head, err := headBlocks(doc, src)
	if err != nil {
		return err
	}
	r.meta = dict.Meta{Name: name, Format: Format}
	fromSeen, toSeen := false, false
	for i, l := range sourceLines(head, src) {
		key, val, ok := parseField(l)
		if !ok {
			r.warn(i+2, "header line %d is not `key: value`; it and the rest of the header are ignored", i+2)
			break
		}
		switch {
		case key == "wudict":
		case key == "from" && !fromSeen:
			fromSeen, r.meta.IndexLang = true, primaryLang(val)
		case key == "to" && !toSeen:
			toSeen, r.meta.ContentsLang = true, primaryLang(val)
		case key == "from" || key == "to":
		default:
			r.meta.Header = append(r.meta.Header, dict.Field{Name: key, Value: val})
		}
	}
	n := head.NextSibling()
	desc := n
	for n != nil && !isH2(n) {
		n = n.NextSibling()
	}
	r.meta.Description = r.render(src, desc, n)
	return nil
}

// foldRedirects applies R3.6 before anything is yielded. A redirect's names
// become aliases of EVERY article whose headword or alias equals its target
// (exactly, else ignoring case), after that article's own names and in file
// order - the folded form the writer produces (R6.4). The store's own link
// resolution files a redirect under one entry only, so it is not used.
//
// A redirect that reaches no article keeps its names as an article holding a
// lookup link to the target, rather than disappearing from the index. Chains
// are not followed: a target that names only another redirect is dangling.
func (r *Reader) foldRedirects() {
	exact := map[string][]int{}
	folded := map[string][]int{}
	for i, e := range r.entries {
		if e.see == "" {
			for _, n := range e.names {
				exact[n] = append(exact[n], i)
				folded[strings.ToLower(n)] = append(folded[strings.ToLower(n)], i)
			}
		}
	}
	has := map[int]map[string]bool{} // names of an article that received folds
	for i := range r.entries {
		e := &r.entries[i]
		if e.see == "" {
			continue
		}
		targets := exact[e.see]
		if len(targets) == 0 {
			targets = folded[strings.ToLower(e.see)]
		}
		if len(targets) == 0 {
			continue // dangling: yielded as a lookup link
		}
		e.folded = true
		for _, t := range targets {
			a := &r.entries[t]
			set := has[t]
			if set == nil {
				set = make(map[string]bool, len(a.names))
				for _, n := range a.names {
					set[n] = true
				}
				has[t] = set
			}
			for _, n := range e.names {
				if !set[n] {
					set[n] = true
					a.names = append(a.names, n)
				}
			}
		}
	}
	for _, e := range r.entries {
		if !e.folded {
			r.meta.EntryCount++
		}
	}
}

// lookupLink is the article of a dangling redirect: a link to its target.
func lookupLink(target string) string {
	return `<p><a href="` + html.EscapeString(htmlref.EntryHref(target)) + `">` + html.EscapeString(target) + "</a></p>\n"
}

// primaryLang maps a BCP 47 tag to the ISO 639-1 code of its primary subtag.
func primaryLang(tag string) string {
	p, _, _ := strings.Cut(tag, "-")
	return lang.Normalize(p)
}

// renderBody renders the top-level blocks first up to stop with the stock
// renderer. A renderer panic keeps the body as escaped source text, and the
// warning that says so is returned.
func renderBody(src []byte, first, stop ast.Node) (out, warning string) {
	if first == nil || first == stop {
		return "", ""
	}
	defer func() {
		if p := recover(); p != nil {
			end := len(src)
			if stop != nil && stop.Pos() >= 0 {
				end = stop.Pos()
			}
			start := min(max(first.Pos(), 0), end)
			warning = fmt.Sprintf("a body could not be rendered (%v); kept as text", p)
			out = "<pre>" + html.EscapeString(string(src[start:end])) + "</pre>"
		}
	}()
	var b bytes.Buffer
	for n := first; n != nil && n != stop; n = n.NextSibling() {
		if err := mdRenderer.Render(&b, src, n); err != nil {
			panic(err)
		}
	}
	return b.String(), ""
}

// render is renderBody, its warning recorded.
func (r *Reader) render(src []byte, first, stop ast.Node) string {
	out, w := renderBody(src, first, stop)
	if w != "" {
		r.warn(0, "%s", w)
	}
	return out
}

func (r *Reader) Meta() dict.Meta { return r.meta }

// Next yields the articles in file order, redirects already folded into them
// (foldRedirects). After the last, or an error, what phase 2 held is let go.
func (r *Reader) Next() (dict.Entry, error) {
	for r.i < len(r.entries) {
		e := &r.entries[r.i]
		r.i++
		switch {
		case e.folded:
			continue
		case e.see != "":
			return dict.Entry{Headwords: e.names, Body: lookupLink(e.see), Kind: dict.BodyHTML}, nil
		}
		if err := r.want(e.unit); err != nil {
			r.i = len(r.entries)
			r.end()
			return dict.Entry{}, err
		}
		return dict.Entry{Headwords: e.names, Body: r.bodies[e.slot], Kind: dict.BodyHTML}, nil
	}
	r.end()
	return dict.Entry{}, io.EOF
}

// Warnings returns the W-entry diagnostics, and how many were dropped past
// the cap.
func (r *Reader) Warnings() ([]string, int) { return r.warns, r.dropped }

// Close releases the text (a temporary file, if one was made) and reports
// the warnings: each in verbose mode, a count otherwise.
func (r *Reader) Close() error {
	r.end()
	var err error
	if r.src != nil {
		err = r.src.close()
		r.src = nil
	}
	if n := len(r.warns) + r.dropped; n > 0 {
		base := filepath.Base(r.meta.Path)
		for _, w := range r.warns {
			logx.V("%s:%s", base, w)
		}
		logx.Warn("%s: %d entry warning(s); run with -v to list them", base, n)
		r.warns, r.dropped = nil, 0
	}
	return err
}
