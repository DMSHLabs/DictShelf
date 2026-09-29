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
// The file is parsed in CHUNKS, because a syntax tree costs about forty times
// the text it came from. A chunk ends where a heading group starts, and every
// cut is CONFIRMED by the parser rather than guessed: the window up to a
// candidate `## ` line is parsed, and the candidate stands only when that
// parse ends in a top-level ATX heading starting on it. The cut is then made
// at the start of the heading group that heading belongs to. A top-level
// heading closes every open block, so parsing onwards from it gives the same
// blocks as parsing the whole file - by induction from the file's start. A
// candidate the parser rejects (a `## ` line inside a code block or an HTML
// block) doubles the window, so the work stays linear however many such lines
// a block holds.
//
// Only link reference definitions cross a cut, since CommonMark makes them
// document-wide. When the file can hold any, they are collected first, and
// every chunk is parsed knowing all of them, the first definition of a label
// winning - exactly how a whole-file parse resolves them.

var (
	mdParser   = parser.New(parser.WithExtensions(extension.TableParser))
	mdRenderer = gmhtml.New(gmhtml.WithUnsafe(), gmhtml.WithExtensions(extension.TableHTMLRenderer))
)

// chunkSize is the text a chunk aims for; a variable so tests can make
// chunks small.
var chunkSize = 1 << 20

// maxWarnings bounds what one file keeps; the count past it is kept.
const maxWarnings = 1000

type chunk struct {
	start, end int // byte range in the text
	line       int // line number of start
	first      int // index of its first entry in Reader.entries
}

type entry struct {
	names  []string
	chunk  int
	see    string // redirect target (R3.6)
	folded bool   // a redirect whose names became aliases of its targets
}

// Reader is the ingest scan. The file is read at open for its names; each
// body is rendered when Next reaches it, from one chunk's parse at a time.
type Reader struct {
	src     *source
	meta    dict.Meta
	defs    []parser.LinkDefinition // every reference definition, file order
	chunks  []chunk
	entries []entry
	i       int
	warns   []string
	dropped int

	cur    int           // the chunk Next renders from, -1 before the first
	curSrc []byte        // its text
	bodies [][2]ast.Node // first, stop of each of its kept groups
}

// NewReader opens a `.wudict.md`, a qualifying `.md`, or their `.gz`/`.dz`.
// A file this reader does not read returns *Error.
func NewReader(path string) (*Reader, error) {
	src, err := openSource(path)
	if err != nil {
		return nil, err
	}
	r, err := readSource(src)
	if err != nil {
		src.close()
		return nil, err
	}
	r.meta.Path = path
	return r, nil
}

// read reads decoded text held in memory.
func read(text []byte) (*Reader, error) { return readSource(sourceFromBytes(text)) }

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

func isH2(n ast.Node) bool {
	h, ok := n.(*ast.Heading)
	return ok && h.Level == 2
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

// parse reads text[lo:hi] and parses it, knowing every reference definition
// of the file.
func (r *Reader) parse(lo, hi int) (ast.Node, parser.Context, []byte, error) {
	text, err := r.src.read(lo, hi)
	if err != nil {
		return nil, nil, nil, err
	}
	pc := parser.NewContext()
	for _, d := range r.defs {
		pc.AddLinkDefinition(d)
	}
	return mdParser.Parse(text, parser.WithContext(pc)), pc, text, nil
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

// nextChunk finds where the chunk starting at start ends (see the package
// comment). It returns the end; the parse it was confirmed with, which may run
// past the end into the next chunk's first heading group; the text of that
// parse; and the node the chunk stops at in it, nil for the last chunk.
func (r *Reader) nextChunk(start int) (end int, doc ast.Node, pc parser.Context, text []byte, stop ast.Node, err error) {
	from := start + chunkSize
	for k := r.candidateFrom(max(from, start+1)); k < len(r.src.cands); {
		c := r.src.cands[k]
		if doc, pc, text, err = r.parse(start, c.end); err != nil {
			return 0, nil, nil, nil, nil, err
		}
		last := doc.LastChild()
		if h, ok := last.(*ast.Heading); ok && h.Level == 2 && h.HeadingKind == ast.HeadingKindATX && last.Pos() == c.at-start {
			g := groupStart(last)
			cut := start + bytes.LastIndexByte(text[:max(g.Pos(), 0)], '\n') + 1
			if g != doc.FirstChild() && cut > start {
				return cut, doc, pc, text, g, nil
			}
		}
		// Not a cut: inside a block, or the second heading of a group. The
		// window doubles, so the work stays linear.
		k = r.candidateFrom(max(c.at+1, start+2*(c.at-start)))
	}
	if doc, pc, text, err = r.parse(start, r.src.size); err != nil {
		return 0, nil, nil, nil, nil, err
	}
	return r.src.size, doc, pc, text, nil, nil
}

// readSource applies §3 to a decoded text.
func readSource(src *source) (r *Reader, err error) {
	line1, rest := cutLine(src.head)
	line2, _ := cutLine(rest)
	if !titleLine(line1) {
		return nil, formatErr("line 1 must be `# ` and the dictionary title")
	}
	if err := checkVersion(string(line2)); err != nil {
		return nil, err
	}
	defer func() {
		// The parser is third-party code on hostile input: a panic costs the
		// file, never the process.
		if p := recover(); p != nil {
			r, err = nil, formatErr("the file could not be parsed: %v", p)
		}
	}()

	r = &Reader{src: src, cur: -1}
	// A reference definition always holds "]:"; a file without one has none,
	// and its chunks are scanned from the parse that cut them.
	withDefs := src.defs
	line := 1
	for start := 0; start < src.size; {
		end, doc, pc, text, stop, err := r.nextChunk(start)
		if err != nil {
			return nil, err
		}
		r.chunks = append(r.chunks, chunk{start: start, end: end, line: line, first: len(r.entries)})
		line = lineAt(text, end-start, line)
		if withDefs {
			for _, d := range pc.LinkDefinitions() {
				// Detached from its node: the stock link parser reads a
				// definition through its node's positions, which refer to
				// the chunk it was parsed from; without one it reads the
				// bytes, through the same decoder.
				r.defs = append(r.defs, parser.NewLinkDefinition(
					bytes.Clone(d.Label()), bytes.Clone(d.Destination()), bytes.Clone(d.Title())))
			}
		} else if err := r.scan(len(r.chunks)-1, doc, stop, text); err != nil {
			return nil, err
		}
		start = end
	}
	if withDefs {
		// The cuts stand, since definitions change no block; the names are
		// read again, with every definition known.
		for i := range r.chunks {
			r.chunks[i].first = len(r.entries)
			doc, _, text, err := r.parse(r.chunks[i].start, r.chunks[i].end)
			if err != nil {
				return nil, err
			}
			if err := r.scan(i, doc, nil, text); err != nil {
				return nil, err
			}
		}
	}
	r.foldRedirects()
	return r, nil
}

// group is one heading group and its body, in one chunk's parse.
type group struct {
	names       []string
	at          ast.Node // the group's first heading
	first, stop ast.Node // body: first up to, not including, stop
	blocks      int      // body blocks, reference definitions not counted
	only        ast.Node // the body's block when blocks == 1
}

func (g group) kept() bool { return g.blocks > 0 && len(g.names) > 0 }

// groups walks the top-level blocks from n up to stop (R3.5).
func groups(n, stop ast.Node, src []byte) []group {
	var out []group
	for n != nil && n != stop {
		g := group{at: n}
		seen := map[string]bool{}
		for n != nil && n != stop && (isH2(n) || isRefDef(n)) {
			if isH2(n) {
				if t := headingText(n, src); t != "" && !seen[t] {
					seen[t] = true
					g.names = append(g.names, t)
				}
			}
			n = n.NextSibling()
		}
		g.first = n
		for n != nil && n != stop && !isH2(n) {
			if !isRefDef(n) {
				g.blocks++
				g.only = n
			}
			n = n.NextSibling()
		}
		g.stop = n
		out = append(out, g)
	}
	return out
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

// scan reads one chunk's header (the first chunk only) and heading groups;
// src is the text doc was parsed from.
func (r *Reader) scan(ci int, doc, stop ast.Node, src []byte) error {
	c := r.chunks[ci]
	if ci == 0 {
		if err := r.header(doc, stop, src); err != nil {
			return err
		}
	}
	for _, g := range groups(entriesFrom(doc, ci == 0), stop, src) {
		switch {
		case g.blocks == 0:
			r.warn(lineAt(src, g.at.Pos(), c.line), "heading group without a body; skipped")
			continue
		case len(g.names) == 0:
			r.warn(lineAt(src, g.at.Pos(), c.line), "entry without a headword; skipped")
			continue
		}
		e := entry{names: g.names, chunk: ci}
		if g.blocks == 1 {
			if _, ok := g.only.(*ast.Paragraph); ok {
				if ls := sourceLines(g.only, src); len(ls) == 1 {
					if t, ok := strings.CutPrefix(ls[0], "see:"); ok && t != "" && (t[0] == ' ' || t[0] == '\t') && trimWS(t) != "" {
						e.see = trimWS(t)
					}
				}
			}
		}
		r.entries = append(r.entries, e)
	}
	return nil
}

// header reads the title, the header fields and the description (R3.1-R3.3).
func (r *Reader) header(doc, stop ast.Node, src []byte) error {
	line1, _ := cutLine(src)
	title, ok := doc.FirstChild().(*ast.Heading)
	if !ok || title.Level != 1 {
		return formatErr("line 1 must be `# ` and the dictionary title")
	}
	name := headingText(title, src)
	if name == "" {
		return formatErr("the title on line 1 is empty")
	}
	head := title.NextSibling()
	if _, ok := head.(*ast.Paragraph); !ok || head.Pos() != len(line1)+1 {
		return formatErr("line 2 must be `wudict: 1`, directly under the title")
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
	for n != nil && n != stop && !isH2(n) {
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

// render renders the top-level blocks first up to stop with the stock
// renderer. A renderer panic keeps the body as escaped source text.
func (r *Reader) render(src []byte, first, stop ast.Node) (out string) {
	if first == nil || first == stop {
		return ""
	}
	defer func() {
		if p := recover(); p != nil {
			end := len(src)
			if stop != nil && stop.Pos() >= 0 {
				end = stop.Pos()
			}
			start := min(max(first.Pos(), 0), end)
			r.warn(0, "a body could not be rendered (%v); kept as text", p)
			out = "<pre>" + html.EscapeString(string(src[start:end])) + "</pre>"
		}
	}()
	var b bytes.Buffer
	for n := first; n != nil && n != stop; n = n.NextSibling() {
		if err := mdRenderer.Render(&b, src, n); err != nil {
			panic(err)
		}
	}
	return b.String()
}

// loadChunk parses chunk ci again for its bodies; one chunk's text and tree
// are held at a time.
func (r *Reader) loadChunk(ci int) error {
	c := r.chunks[ci]
	doc, _, text, err := r.parse(c.start, c.end)
	if err != nil {
		return err
	}
	// A fresh slice: reusing the old one's array would keep the previous
	// chunk's tree reachable through its tail.
	r.cur, r.curSrc, r.bodies = ci, text, nil
	for _, g := range groups(entriesFrom(doc, ci == 0), nil, r.curSrc) {
		if g.kept() {
			r.bodies = append(r.bodies, [2]ast.Node{g.first, g.stop})
		}
	}
	return nil
}

func (r *Reader) Meta() dict.Meta { return r.meta }

// Next yields the articles in file order, redirects already folded into them
// (foldRedirects).
func (r *Reader) Next() (e dict.Entry, err error) {
	defer func() {
		// A chunk is parsed again here; the parser is third-party code on
		// hostile input, and a panic costs the file, never the process.
		if p := recover(); p != nil {
			e, err = dict.Entry{}, formatErr("the file could not be parsed: %v", p)
		}
	}()
	for r.i < len(r.entries) {
		i := r.i
		e := &r.entries[i]
		r.i++
		switch {
		case e.folded:
			continue
		case e.see != "":
			return dict.Entry{Headwords: e.names, Body: lookupLink(e.see), Kind: dict.BodyHTML}, nil
		}
		if e.chunk != r.cur {
			if err := r.loadChunk(e.chunk); err != nil {
				return dict.Entry{}, err
			}
		}
		k := i - r.chunks[e.chunk].first
		if k >= len(r.bodies) {
			return dict.Entry{}, fmt.Errorf("wudict markdown: chunk %d reads differently on its second parse", e.chunk)
		}
		body := r.render(r.curSrc, r.bodies[k][0], r.bodies[k][1])
		return dict.Entry{Headwords: e.names, Body: body, Kind: dict.BodyHTML}, nil
	}
	r.bodies, r.curSrc = nil, nil
	return dict.Entry{}, io.EOF
}

// Warnings returns the W-entry diagnostics, and how many were dropped past
// the cap.
func (r *Reader) Warnings() ([]string, int) { return r.warns, r.dropped }

// Close releases the text (a temporary file, if one was made) and reports
// the warnings: each in verbose mode, a count otherwise.
func (r *Reader) Close() error {
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
