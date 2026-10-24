// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/util"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/sysmem"
)

// Phase 1 (see read.go): the structure of the file, from block parses.

// pchunk is what phase 1 reads off one chunk's block parse.
type pchunk struct {
	start, end int
	ok         bool // the cut that ends it stands (always, for the last chunk)
	lines      int  // line ends in [start, end)
	head       bool // it is the first chunk and lines 1-2 have the shape R3.1-R3.2 need
	groups     []pgroup
	swallowed  []swallow
	defs       []parser.LinkDefinition // its own, detached from the parse
}

// pgroup is a heading group as phase 1 found it.
type pgroup struct {
	at     int // the start of its first heading's line
	line   int // that line's number, counted from the chunk's first line as 0
	heads  []heading
	blocks int    // body blocks, reference definitions not counted
	see    string // redirect target (R3.6)
}

// heading is an entry heading: its text, which is its content as goldmark
// cut it unless that holds inline syntax; then its whole line, parsed again
// once every definition is known.
type heading struct {
	line []byte // only while the text is still to be read
	text string
}

// inlineSyntax holds every byte that starts inline syntax in goldmark's
// CommonMark parsers (code spans, links and images, autolinks and raw HTML,
// emphasis) or that its text decoder reads (escapes, entities).
const inlineSyntax = "`[]!<*_\\&"

// swallow is a line that looks like an entry heading inside an HTML block.
type swallow struct {
	line, block int // its line and the block's first, counted from the chunk's first line as 0
	text        string
}

// The weights phase 1 and 2 charge the preparation budget, in peak Go heap
// per byte of text: a block-only tree, and a full tree with its rendering,
// each at the collector's twice-the-live-heap goal.
const (
	chunkWeight = 13
	unitWeight  = 46
)

// specCuts chooses the cuts ahead: for every chunkSize of text, the first
// candidate line at or past it that does not continue a heading group - one
// the line before, or only blank lines before, is another candidate. The
// parse confirms each; a wrong choice costs a repair, never the result.
func (r *Reader) specCuts() []int {
	cands := r.src.cands
	var cuts []int
	if cutEvery {
		for k := range cands {
			if k > 0 || cands[k].at > 0 {
				cuts = append(cuts, k)
			}
		}
		return cuts
	}
	for k := r.candidateFrom(chunkSize); k < len(cands); {
		for k < len(cands) && k > 0 && r.continuesGroup(k) {
			k++
		}
		if k >= len(cands) {
			break
		}
		cuts = append(cuts, k)
		k = r.candidateFrom(cands[k].at + chunkSize)
	}
	return cuts
}

// scanFault, when set, fails the read of a chunk starting at start: for the
// tests of what a chunk read from a wrong start may do.
var scanFault func(start int) error

// cutEvery makes every candidate line a cut chosen ahead, most of which the
// parse then rejects: for the tests of the repair.
var cutEvery = false

// continuesGroup reports a candidate with only blank lines (at most a few)
// between it and the candidate before it.
func (r *Reader) continuesGroup(k int) bool {
	lo, hi := r.src.cands[k-1].end, r.src.cands[k].at
	if lo >= hi {
		return true
	}
	if hi-lo > 256 {
		return false
	}
	b, err := r.src.read(lo, hi)
	return err == nil && len(bytes.Trim(b, " \t\n")) == 0
}

// scanChunk reads the chunk from start, ended by the cut at candidate
// confirm, or by the end of the text when confirm is -1.
func (r *Reader) scanChunk(start, confirm int) (c pchunk, err error) {
	defer func() {
		// The parser is third-party code on hostile input: a panic costs the
		// file, never the process.
		if p := recover(); p != nil {
			c, err = pchunk{}, formatErr("the file could not be parsed: %v", p)
		}
	}()
	if scanFault != nil {
		if err := scanFault(start); err != nil {
			return pchunk{}, err
		}
	}
	hi := r.src.size
	if confirm >= 0 {
		hi = r.src.cands[confirm].end
	}
	text, err := r.src.read(start, hi)
	if err != nil {
		return pchunk{}, err
	}
	pc := parser.NewContext()
	doc := blockParser.Parse(text, parser.WithContext(pc))
	c = pchunk{start: start, end: hi}
	var stop ast.Node
	if confirm >= 0 {
		at := r.src.cands[confirm].at
		last := doc.LastChild()
		if !isH2(last) || last.Pos() != at-start || groupStart(last) != last || last == doc.FirstChild() {
			return c, nil // not a cut: inside a block, or not the first heading of its group
		}
		c.end, stop = at, last
	}
	c.ok = true
	c.lines = bytes.Count(text[:c.end-start], []byte{'\n'})
	from := doc.FirstChild()
	if start == 0 {
		from = nil // the header's shape is wrong: the barrier says how
		if _, err := headBlocks(doc, text); err == nil {
			c.head, from = true, entriesFrom(doc, true)
		}
	}
	c.swallowed = r.swallowedIn(start, doc, stop, text)
	c.groups = scanGroups(from, stop, text, start)
	for _, d := range pc.LinkDefinitions() {
		// Detached from its node: the stock link parser reads a definition
		// through its node's positions, which refer to the chunk it was parsed
		// from; without one it reads the bytes, through the same decoder.
		c.defs = append(c.defs, parser.NewLinkDefinition(
			bytes.Clone(d.Label()), bytes.Clone(d.Destination()), bytes.Clone(d.Title())))
	}
	return c, nil
}

// scanGroups reads the heading groups from n up to stop (R3.5), as spans
// cuts them - the grouping phase 2 checks its parse against.
func scanGroups(n, stop ast.Node, text []byte, start int) []pgroup {
	var out []pgroup
	off, line := 0, 0 // a line start and its line number, advanced group by group
	for _, sp := range spans(n, stop) {
		ls := lineStart(text, sp.at.Pos())
		line += bytes.Count(text[off:ls], []byte{'\n'})
		off = ls
		g := pgroup{at: start + ls, line: line}
		for h := sp.at; h != sp.first; h = h.NextSibling() {
			if isH2(h) {
				g.heads = append(g.heads, readHeading(h, text))
			}
		}
		var only ast.Node
		for b := sp.first; b != sp.stop; b = b.NextSibling() {
			if !isRefDef(b) {
				g.blocks++
				only = b
			}
		}
		if g.blocks == 1 {
			g.see = seeTarget(only, text)
		}
		out = append(out, g)
	}
	return out
}

// readHeading keeps what reading a heading's text needs.
func readHeading(n ast.Node, text []byte) heading {
	src := n.(ast.BlockNode).Source()
	if len(src) == 0 {
		return heading{} // an empty heading: no text
	}
	content := src[0].Bytes(text)
	if bytes.ContainsAny(content, inlineSyntax) {
		ls := lineStart(text, n.Pos())
		le := len(text)
		if i := bytes.IndexByte(text[ls:], '\n'); i >= 0 {
			le = ls + i
		}
		return heading{line: bytes.Clone(text[ls:le])}
	}
	return heading{text: trimWS(string(content))}
}

// swallowedIn finds the lines that look like entry headings - `##` and WS at
// column 0, the lines the chunk cutter collects - but lie inside an HTML
// block. Markdown makes such a line part of the block, so the entry it meant
// to start is silently not there: the block opened earlier and has not ended,
// because a type 6-7 block runs to the next blank line and a <pre>, <script>,
// <style> or <textarea> block to its closing tag. The reader stays a stock
// CommonMark reader (D154); what it adds is saying so, on the line where an
// entry went missing and the line that took it.
func (r *Reader) swallowedIn(start int, doc, stop ast.Node, text []byte) []swallow {
	var out []swallow
	for n := doc.FirstChild(); n != nil && n != stop; n = n.NextSibling() {
		hb, ok := n.(*ast.HTMLBlock)
		if !ok {
			continue
		}
		lo, hi := blockSpan(hb)
		if hi <= lo {
			continue
		}
		// A candidate is a whole line, so one that starts after the block's
		// first byte and before its last is one of its lines.
		for k := r.candidateFrom(start + lo + 1); k < len(r.src.cands) && r.src.cands[k].at < start+hi; k++ {
			at := r.src.cands[k].at - start
			line, _ := cutLine(text[at:])
			out = append(out, swallow{line: lineAt(text, at, 0), block: lineAt(text, lo, 0), text: clip(string(line))})
		}
	}
	return out
}

// future is a chunk being read, claimed by a worker or by the validator.
type future struct {
	claim  atomic.Int32 // claimNone, claimWorker or claimHere
	done   chan struct{}
	c      pchunk
	err    error
	weight int64
}

const (
	claimNone   = iota
	claimWorker // a worker reads it
	claimHere   // the validator reads it
)

// readSource applies §3 to a decoded text: phase 1, then what phase 2 needs.
// path is the source the preparation is for, whose urgency chooses the lane.
func readSource(src *source, path string) (r *Reader, err error) {
	line1, rest := cutLine(src.head)
	line2, _ := cutLine(rest)
	if !titleLine(line1) {
		return nil, formatErr("line 1 must be `# ` and the dictionary title")
	}
	if err := checkVersion(string(line2)); err != nil {
		return nil, err
	}
	defer func() {
		if p := recover(); p != nil {
			r, err = nil, formatErr("the file could not be parsed: %v", p)
		}
	}()
	r = &Reader{src: src, path: path, cur: -1, defs: map[string]parser.LinkDefinition{}}
	chunks, err := r.index()
	if err != nil {
		return nil, err
	}
	if err := r.assemble(chunks); err != nil {
		return nil, err
	}
	return r, nil
}

// index reads the chunks the cuts chosen ahead make, confirmed in order, an
// unconfirmed one replaced by its repair. While the preparation is urgent a
// dispatcher hands the chunks ahead to workers, as far as the budget allows;
// a chunk none took, the validator reads itself. The lane is read at every
// chunk, both ways.
func (r *Reader) index() ([]pchunk, error) {
	cuts := r.specCuts()
	n := len(cuts) + 1
	startOf := func(k int) int {
		if k == 0 {
			return 0
		}
		return r.src.cands[cuts[k-1]].at
	}
	confirmOf := func(k int) int {
		if k < len(cuts) {
			return cuts[k]
		}
		return -1
	}
	scan := func(k int) (pchunk, error) { return r.scanChunk(startOf(k), confirmOf(k)) }
	futures := make([]future, n)
	var (
		acct        sysmem.Account
		jobs        chan int
		stop        chan struct{}
		released    = make(chan struct{}, 1)
		dispatching atomic.Bool
		workers     sync.WaitGroup
		dispatchers sync.WaitGroup
	)
	defer acct.Close()
	defer func() {
		if jobs != nil {
			close(stop)
			dispatchers.Wait()
			close(jobs)
			workers.Wait()
		}
	}()
	// dispatch starts the workers once, and a dispatcher from chunk from on
	// unless one runs.
	dispatch := func(from int) {
		if jobs == nil {
			jobs, stop = make(chan int, 2*workersMax), make(chan struct{})
			for range min(runtime.GOMAXPROCS(0), workersMax) {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for k := range jobs {
						f := &futures[k]
						f.c, f.err = scan(k)
						close(f.done)
					}
				}()
			}
		}
		if !dispatching.CompareAndSwap(false, true) {
			return
		}
		dispatchers.Add(1)
		go func() {
			defer dispatchers.Done()
			defer dispatching.Store(false)
			for k := from; k < n; k++ {
				if !urgentNow(r) {
					return
				}
				end := r.src.size
				if c := confirmOf(k); c >= 0 {
					end = r.src.cands[c].end
				}
				w := chunkWeight * int64(end-startOf(k))
				room := true
				for !acct.TryAcquire(w) {
					if acct.Held() == 0 {
						room = false // no room for one chunk ahead: the validator reads it
						break
					}
					select {
					case <-released:
					case <-stop:
						return
					}
				}
				if !room {
					continue
				}
				f := &futures[k]
				if !f.claim.CompareAndSwap(claimNone, claimWorker) {
					acct.Release(w) // the validator got there first
					continue
				}
				f.weight = w
				select {
				case jobs <- k:
				case <-stop:
					return
				}
			}
		}()
	}
	// take is chunk k, from its worker or read here; give returns a worker's
	// draw on the budget.
	give := func(f *future) {
		acct.Release(f.weight)
		select {
		case released <- struct{}{}:
		default:
		}
	}
	take := func(k int) (pchunk, error) {
		f := &futures[k]
		if f.claim.CompareAndSwap(claimNone, claimHere) {
			return scan(k)
		}
		<-f.done
		give(f)
		return f.c, f.err
	}
	// discard drops chunk k, read from a start that turned out wrong: what it
	// found, a parse error included, is not the file's.
	discard := func(k int) {
		if f := &futures[k]; !f.claim.CompareAndSwap(claimNone, claimHere) {
			<-f.done
			give(f)
		}
	}
	for k := range futures {
		futures[k].done = make(chan struct{})
	}
	var chunks []pchunk
	for k := 0; k < n; {
		if k+1 < n && urgentNow(r) {
			dispatch(k + 1)
		}
		c, err := take(k)
		if err != nil {
			return nil, err
		}
		next := k + 1
		if !c.ok {
			// The cut that ends chunk k does not stand: double the window over
			// the cuts chosen ahead until one does, or the text ends.
			for m := 2; ; m *= 2 {
				if k+m >= n {
					next = n
					c, err = r.scanChunk(startOf(k), -1)
				} else {
					next = k + m
					c, err = r.scanChunk(startOf(k), confirmOf(k+m-1))
				}
				if err != nil {
					return nil, err
				}
				if c.ok {
					break
				}
			}
			for j := k + 1; j < next; j++ {
				discard(j)
			}
		}
		chunks = append(chunks, c)
		k = next
	}
	return chunks, nil
}

// assemble is the barrier between the phases: every definition is known, so
// the headings, the title and the description are read; the warnings and the
// entries are laid out in file order, the redirects folded, the bodies
// assigned to phase 2's units.
func (r *Reader) assemble(chunks []pchunk) error {
	line := 1
	for _, c := range chunks {
		r.chunks = append(r.chunks, chunk{start: c.start, end: c.end, line: line})
		line += c.lines
		for _, d := range c.defs {
			// The first of a label in file order wins: within a chunk the
			// context kept it, across chunks the earlier chunk set it.
			if l := util.ToLinkReference(d.Label()); r.defs[l] == nil {
				r.defs[l] = d
			}
		}
	}
	if err := r.readHeadings(chunks); err != nil {
		return err
	}

	// The title, header and description: the text before the first heading
	// group, parsed whole. A first chunk whose lines 1-2 have the wrong shape
	// fails here, header naming what is wrong in R3.1-R3.3's order.
	end := r.src.size
	for _, c := range chunks {
		if len(c.groups) > 0 {
			end = c.groups[0].at
			break
		}
	}
	if !chunks[0].head {
		end = chunks[0].end
	}
	text, err := r.src.read(0, end)
	if err != nil {
		return err
	}
	pc := parser.NewContext()
	prefill(pc, r.defs, text)
	if err := r.header(mdParser.Parse(text, parser.WithContext(pc)), text); err != nil {
		return err
	}
	if !chunks[0].head {
		return formatErr("line 2 must be `wudict: 1`, directly under the title")
	}

	// The warnings in the order the file read whole gives them, wherever the
	// cuts fell: the header's (above), the swallowed lines', the groups'.
	for ci, c := range chunks {
		base := r.chunks[ci].line
		for _, s := range c.swallowed {
			r.warn(base+s.line, "%q is inside the HTML block that starts on line %d, so it is not an entry; "+
				"end that block before it (a blank line, or its closing tag)", s.text, base+s.block)
		}
	}
	type placed struct{ at, entry int }
	var all []placed
	for ci, c := range chunks {
		base := r.chunks[ci].line
		for _, g := range c.groups {
			p := placed{at: g.at, entry: -1}
			var names []string
			for _, h := range g.heads {
				if h.text != "" && !slices.Contains(names, h.text) {
					names = append(names, h.text)
				}
			}
			switch {
			case g.blocks == 0:
				r.warn(base+g.line, "heading group without a body; skipped")
			case len(names) == 0:
				r.warn(base+g.line, "entry without a headword; skipped")
			default:
				p.entry = len(r.entries)
				r.entries = append(r.entries, entry{names: names, see: g.see, unit: -1})
			}
			all = append(all, p)
		}
	}
	r.foldRedirects()

	// Phase 2's units: whole groups, cut at a group start once a unit holds
	// unitSize of text.
	for _, p := range all {
		if len(r.units) == 0 || p.at-r.units[len(r.units)-1].start >= unitSize {
			r.units = append(r.units, unit{start: p.at})
		}
		u := &r.units[len(r.units)-1]
		u.ats = append(u.ats, p.at)
		if p.entry >= 0 && r.entries[p.entry].see == "" {
			e := &r.entries[p.entry]
			e.unit, e.slot = len(r.units)-1, len(u.render)
			u.render = append(u.render, len(u.ats)-1)
		}
	}
	for i := range r.units {
		if i+1 < len(r.units) {
			r.units[i].end = r.units[i+1].start
		} else {
			r.units[i].end = r.src.size
		}
	}
	return nil
}

// readHeadings reads the text of the headings that hold inline syntax: their
// lines, a batch at a time, parsed as a document of headings alone - each
// stays the ATX heading it was - knowing the definitions they can reference.
func (r *Reader) readHeadings(chunks []pchunk) error {
	var pending []*heading
	size := 0
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		var doc bytes.Buffer
		for _, h := range pending {
			doc.Write(h.line)
			doc.WriteByte('\n')
		}
		src := doc.Bytes()
		pc := parser.NewContext()
		prefill(pc, r.defs, src)
		n := mdParser.Parse(src, parser.WithContext(pc)).FirstChild()
		for _, h := range pending {
			if !isH2(n) {
				return fmt.Errorf("wudict markdown: the heading %q reads differently alone", clip(string(h.line)))
			}
			h.text, h.line = headingText(n, src), nil
			n = n.NextSibling()
		}
		if n != nil {
			return fmt.Errorf("wudict markdown: the headings read differently alone")
		}
		pending, size = pending[:0], 0
		return nil
	}
	for ci := range chunks {
		for gi := range chunks[ci].groups {
			hs := chunks[ci].groups[gi].heads
			for hi := range hs {
				if hs[hi].line == nil {
					continue
				}
				pending = append(pending, &hs[hi])
				if size += len(hs[hi].line) + 1; size >= 1<<20 {
					if err := flush(); err != nil {
						return err
					}
				}
			}
		}
	}
	return flush()
}

// urgentNow says whether a person waits on the preparation of the reader's
// file now; a variable so the tests can switch lanes mid-run.
var urgentNow = func(r *Reader) bool { return dict.Urgent(r.path) }
