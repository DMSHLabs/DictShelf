// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"slices"
	"strings"

	"github.com/wuweidict/wudict/internal/htmlref"
)

// Zones and their nesting (D165).
//
// resolve pairs every opening tag with its closing tag: a closing tag ends the
// nearest open zone of its name, wherever that zone sits, and the zones opened
// inside it stay open - DSL tags are ranges, which is what Lingvo's compiler
// accepts, and the article is complete before it is rendered, so nothing has
// to be decided before the whole of it has been read. emit then writes
// well-formed HTML for those ranges:
//
//   - A closing tag that ends a zone with others open inside it closes them
//     too, and they reopen at the next content. Reopening is lazy, so a
//     repair never creates an empty element (an empty element the author
//     wrote stays).
//   - Zones that start together, and zones waiting to reopen, nest by where
//     they end: the one that lasts longer goes outside. `[p][trn]a[/p] b[/trn]`
//     nests as trn > p and needs no split at all; a split is needed only where
//     two zones truly cross, once per crossing.
//   - A paragraph ([m]) is always outermost: opening or closing one closes the
//     inline zones, which reopen inside the next.
//
// Input whose tags already nest renders exactly as it is written.

const (
	maxSameName = 3  // open zones of one name; HTML's Noah's Ark bound (Lingvo allows 1)
	maxOpen     = 64 // open zones in all; with maxSameName, resolve stays linear
)

type zoneState uint8

const (
	zIdle   zoneState = iota // before its opening tag, or ended
	zFresh                   // opened, its markup not yet written
	zOn                      // its markup is open in the output
	zReopen                  // interrupted by a closing tag, reopens at the next content
)

type zone struct {
	def       tagDef
	name      string
	attrs     map[string]string
	open, end int32 // token indices: its opening tag, and the token it ends before
	line      int32 // source line of the opening tag
	next      int32 // the next zone ending at the same token
	state     zoneState
	closed    bool // ended by its own closing tag
	closing   bool
	ready     bool // markup computed
	openHTML  string
	closeHTML string
}

func (tr *transformer) newZone(ti int, def tagDef, line int32) int32 {
	t := &tr.toks[ti]
	x := int32(len(tr.zones))
	tr.zones = append(tr.zones, zone{def: def, name: t.s, attrs: t.attrs, open: int32(ti), end: -1, line: line, next: -1})
	t.rng = x
	return x
}

// resolve pairs the tags and fixes where every zone ends.
//
//   - A closing tag with no open zone of its name is dropped.
//   - A known tag left open ends at the end of its own line, so one typo
//     colours one line, not the rest of the article. A paragraph ends at the
//     next [m] or at the end of the article: closing it is optional (lingvo-ref
//     "Тэг [m]···[/m]").
//   - A tag nobody knows is judged by its pairing: closed, it was markup (its
//     content is kept; `[B]…[/B]` is read as [b]), left open, it was text -
//     `[1]`, `[sic]`, a Roman `[I]` - and is printed as written.
//   - A link opened inside a link ends the outer one, as an <a> start tag
//     does in HTML.
//   - Past maxSameName or maxOpen the oldest zone in the way ends, as in
//     HTML's Noah's Ark clause - never the new one, so a few stray openers
//     early in an article cannot disable a tag for the rest of it.
func (tr *transformer) resolve() {
	toks := tr.toks
	stack := tr.stack[:0]
	block := int32(-1)
	line := int32(0)
	for ti := range toks {
		t := &toks[ti]
		switch t.kind {
		case tkBreak:
			tr.lineEnd = append(tr.lineEnd, int32(ti))
			line++
		case tkOpen:
			def, _ := lookupTag(t.s)
			if def.kind == kBlock {
				if block >= 0 {
					tr.zones[block].end = int32(ti)
				}
				block = tr.newZone(ti, def, line)
				continue
			}
			if def.kind == kLink {
				for s := len(stack) - 1; s >= 0; s-- {
					if tr.zones[stack[s]].def.kind == kLink {
						stack = tr.endEarly(stack, s, ti, line)
					}
				}
			}
			if s := tr.oldestNamed(stack, t.s); s >= 0 {
				stack = tr.endEarly(stack, s, ti, line)
			}
			if len(stack) >= maxOpen {
				stack = tr.endEarly(stack, 0, ti, line)
			}
			stack = append(stack, tr.newZone(ti, def, line))
		case tkClose:
			if isMarginTag(t.s) {
				if block >= 0 {
					tr.zones[block].end = int32(ti)
					block = -1
				}
				continue
			}
			for s := len(stack) - 1; s >= 0; s-- {
				if z := &tr.zones[stack[s]]; z.name == t.s {
					z.end, z.closed = int32(ti), true
					stack = slices.Delete(stack, s, s+1)
					break
				}
			}
		}
	}
	for _, x := range stack {
		z := &tr.zones[x]
		z.end = int32(len(toks))
		if int(z.line) < len(tr.lineEnd) {
			z.end = tr.lineEnd[z.line]
		}
	}
	if block >= 0 {
		tr.zones[block].end = int32(len(toks))
	}
	tr.stack = stack[:0]
	tr.absorbBreaks()

	tr.endHead = slices.Grow(tr.endHead[:0], len(toks)+1)[:len(toks)+1]
	for i := range tr.endHead {
		tr.endHead[i] = -1
	}
	for x := range tr.zones {
		z := &tr.zones[x]
		if z.def.kind == kUnknown {
			if !z.closed {
				toks[z.open].kind = tkText
				toks[z.open].rng = -1
				continue
			}
			// Closed by its own tag, so markup. A known name in the wrong case
			// is that tag; anything else is unwrapped, its content kept.
			if d, ok := tagDefs[strings.ToLower(z.name)]; ok && (d.kind == kInline || d.kind == kLabel) {
				z.def, z.name = d, strings.ToLower(z.name)
			} else {
				toks[z.open].rng = -1
				continue
			}
		}
		z.next = tr.endHead[z.end]
		tr.endHead[z.end] = int32(x)
		if z.name == "t" {
			tr.markLegacyIPA(z)
		}
	}
}

// absorbBreaks gives a line that is secondary from end to end - every piece of
// its content inside a [*] zone - the line break that follows it. The UI's
// brief view hides wu-sec (lingvo-ref "Тэг [*]···[/*]": shown or hidden at the
// reader's request); a hidden line that left both of its breaks behind would
// leave a blank line in its place. With the break inside, the line goes whole:
//
//	sense<br/><span class="wu-sec">example<br/></span>next
//
// A line's preceding break stays outside, so a run of such lines each takes
// one break and the run as a whole takes all of them but the first. A break
// that a paragraph swallows (s == "") has nothing to give, and a line inside
// a [*] zone that already runs past its break needs nothing.
//
// Linear: zones are created in the order they open, and the same-name bound
// (maxSameName) keeps the set of [*] zones open at any token to three.
func (tr *transformer) absorbBreaks() {
	toks, zones := tr.toks, tr.zones
	next := slices.IndexFunc(zones, func(z zone) bool { return z.name == "*" })
	if next < 0 {
		return
	}
	var open []int32   // [*] zones covering token i, oldest first
	cover := int32(-1) // the outermost one covering the line's last content
	whole := true      // every content token of the line so far is covered
	for i := range toks {
		t := &toks[i]
		open = slices.DeleteFunc(open, func(x int32) bool { return zones[x].end <= int32(i) })
		for ; next < len(zones) && zones[next].open < int32(i); next++ {
			if z := &zones[next]; z.name == "*" && z.end > int32(i) {
				open = append(open, int32(next))
			}
		}
		if t.kind == tkBreak {
			if whole && cover >= 0 && t.s != "" && zones[cover].end <= int32(i) {
				zones[cover].end = int32(i) + 1
			}
			cover, whole = -1, true
			continue
		}
		if !whole || !tr.isContent(t) {
			continue
		}
		if len(open) == 0 {
			whole = false
			continue
		}
		cover = open[0]
	}
}

// isContent reports whether a token puts something on the page: text other
// than the line's own blanks, a literal, an element, or an unknown tag that
// nobody closed and is therefore printed as written.
func (tr *transformer) isContent(t *token) bool {
	switch t.kind {
	case tkText:
		return strings.Trim(tr.input[t.a:t.b], " \t") != ""
	case tkLit:
		return t.s != ""
	case tkHTML:
		return true
	case tkOpen:
		return t.rng >= 0 && tr.zones[t.rng].def.kind == kUnknown && !tr.zones[t.rng].closed
	}
	return false
}

// oldestNamed is the stack index of the oldest open zone named name when
// maxSameName of them are open, or -1.
func (tr *transformer) oldestNamed(stack []int32, name string) int {
	n, oldest := 0, -1
	for s, x := range stack {
		if tr.zones[x].name == name {
			if n == 0 {
				oldest = s
			}
			n++
		}
	}
	if n < maxSameName {
		return -1
	}
	return oldest
}

// endEarly ends the open zone stack[s] before token ti - or at the end of its
// own line, when that line is already over - and takes it off the stack.
func (tr *transformer) endEarly(stack []int32, s, ti int, line int32) []int32 {
	z := &tr.zones[stack[s]]
	z.end = int32(ti)
	if z.line < line {
		z.end = tr.lineEnd[z.line]
	}
	return slices.Delete(stack, s, s+1)
}

// zoneText is the plain text a zone covers: a label's lookup key, a link's
// target.
func (tr *transformer) zoneText(z *zone) string {
	var b strings.Builder
	for i := z.open + 1; i < z.end; i++ {
		switch t := &tr.toks[i]; t.kind {
		case tkText:
			b.WriteString(tr.input[t.a:t.b])
		case tkLit:
			b.WriteString(t.s)
		case tkHTML:
			switch t.s {
			case "&nbsp;":
				b.WriteString("\u00a0")
			case "<br/>":
				b.WriteByte(' ')
			}
		case tkBreak:
			if t.s != "" {
				b.WriteByte(' ')
			}
		}
	}
	return b.String()
}

// emit writes the HTML.
func (tr *transformer) emit() {
	for ti := 0; ; ti++ {
		tr.closeAt(ti)
		if ti == len(tr.toks) {
			return
		}
		t := &tr.toks[ti]
		switch t.kind {
		case tkOpen:
			if t.rng < 0 {
				continue
			}
			if tr.zones[t.rng].def.kind == kBlock {
				tr.suspend()
				tr.out.WriteString(tr.markup(&tr.zones[t.rng]).openHTML)
				continue
			}
			tr.zones[t.rng].state = zFresh
			tr.fresh = append(tr.fresh, t.rng)
		case tkText:
			tr.materialize(false)
			s := tr.input[t.a:t.b]
			if t.ipa {
				s = legacyIPA(s)
			}
			textEscaper.WriteString(&tr.out, s)
		case tkLit:
			tr.materialize(false)
			textEscaper.WriteString(&tr.out, t.s)
		case tkHTML:
			if t.anchor {
				// An <a> inside a link is invalid HTML and the parser ends the
				// outer link there; the links are closed around it instead.
				tr.leaveLinks()
			}
			tr.materialize(t.anchor)
			tr.out.WriteString(t.s)
		case tkBreak:
			if t.s != "" {
				tr.materialize(false)
				tr.out.WriteString(t.s)
			}
		}
	}
}

// closeAt ends every zone whose end is token ti.
func (tr *transformer) closeAt(ti int) {
	x := tr.endHead[ti]
	if x < 0 {
		return
	}
	blockEnds, fresh, n := false, false, 0
	for ; x >= 0; x = tr.zones[x].next {
		z := &tr.zones[x]
		switch {
		case z.def.kind == kBlock:
			blockEnds = true
		case z.state == zFresh && z.closed:
			z.closing, fresh = true, true
			n++
		case z.state == zFresh:
			// Ended by a repair before any content: leaves nothing.
			z.state = zIdle
			tr.fresh = slices.DeleteFunc(tr.fresh, func(y int32) bool { return y == x })
		case z.state == zOn:
			z.closing = true
			n++
		case z.state == zReopen:
			z.state = zIdle
			tr.reopen = slices.DeleteFunc(tr.reopen, func(y int32) bool { return y == x })
		}
	}
	if fresh {
		// Opened and closed by the author with nothing between: kept, as
		// written.
		tr.openPending(&tr.fresh, false)
	}
	for n > 0 {
		if z := &tr.zones[tr.stack[len(tr.stack)-1]]; z.closing {
			tr.closeTop()
			z.closing, z.state = false, zIdle
			n--
		} else {
			tr.suspendTop()
		}
	}
	if blockEnds {
		tr.suspend()
		tr.out.WriteString("</p>")
	}
}

// suspend closes every open inline zone for a paragraph boundary; they reopen
// at the next content.
func (tr *transformer) suspend() {
	for len(tr.stack) > 0 {
		tr.suspendTop()
	}
}

// leaveLinks closes the open links, and everything open inside them, before
// an element that is itself a link.
func (tr *transformer) leaveLinks() {
	low := slices.IndexFunc(tr.stack, func(x int32) bool { return tr.zones[x].def.kind == kLink })
	for low >= 0 && len(tr.stack) > low {
		tr.suspendTop()
	}
}

// closeTop closes the innermost open zone in the output and takes it off the
// stack.
func (tr *transformer) closeTop() int32 {
	x := tr.stack[len(tr.stack)-1]
	tr.stack = tr.stack[:len(tr.stack)-1]
	tr.out.WriteString(tr.zones[x].closeHTML)
	return x
}

// suspendTop closes the innermost open zone, which is not over: it reopens at
// the next content.
func (tr *transformer) suspendTop() {
	x := tr.closeTop()
	tr.zones[x].state = zReopen
	tr.reopen = append(tr.reopen, x)
}

// materialize opens every zone waiting for content. With noLinks it stops at
// the first link in nesting order: what would open inside that link waits
// with it.
func (tr *transformer) materialize(noLinks bool) {
	if len(tr.fresh)+len(tr.reopen) == 0 {
		return
	}
	tr.pend = append(append(tr.pend[:0], tr.reopen...), tr.fresh...)
	tr.reopen, tr.fresh = tr.reopen[:0], tr.fresh[:0]
	tr.openPending(&tr.pend, noLinks)
}

// openPending opens the zones in *p, outermost first: the zone that ends
// later goes outside, and among zones ending together the one opened first.
// Zones left behind by noLinks go back to waiting.
func (tr *transformer) openPending(p *[]int32, noLinks bool) {
	pend := *p
	slices.SortFunc(pend, func(a, b int32) int {
		za, zb := &tr.zones[a], &tr.zones[b]
		if za.end != zb.end {
			return int(zb.end - za.end)
		}
		return int(za.open - zb.open)
	})
	for k, x := range pend {
		z := &tr.zones[x]
		if noLinks && z.def.kind == kLink {
			for _, y := range pend[k:] {
				tr.zones[y].state = zReopen
				tr.reopen = append(tr.reopen, y)
			}
			break
		}
		tr.out.WriteString(tr.markup(z).openHTML)
		z.state = zOn
		tr.stack = append(tr.stack, x)
	}
	*p = pend[:0]
}

// markup computes a zone's opening and closing markup once; a zone that is
// split carries the same markup on every piece.
func (tr *transformer) markup(z *zone) *zone {
	if z.ready {
		return z
	}
	z.ready = true
	z.openHTML, z.closeHTML = z.def.open, z.def.close
	switch {
	case z.def.kind == kBlock:
		z.openHTML = marginOpen(z.name)
	case z.name == "c":
		z.openHTML = colourOpen(z.attrs)
	case z.name == "lang":
		z.openHTML = `<span class="wu-lang"` + langAttrs(z.attrs) + `>`
	case z.def.kind == kLabel:
		// When the dictionary's abbreviation companion knows this label, the
		// whole coloured run is wrapped in an <abbr> carrying the expansion:
		// the browser draws its own tooltip, no client code is involved, and
		// both the element and the title= survive `-format clean`. The label
		// itself is a role (wu-p, internal/artmark) and carries no colour of
		// its own.
		z.openHTML, z.closeHTML = `<span class="wu-p">`, "</span>"
		if exp, ok := tr.abbrev.lookup(strings.Join(strings.Fields(tr.zoneText(z)), " ")); ok {
			z.openHTML += `<abbr class="wu-abbr" title=` + quoteAttr(exp) + `>`
			z.closeHTML = "</abbr></span>"
		}
	case z.def.kind == kLink:
		// A target is a lookup key, and keys have their whitespace collapsed
		// (title.go); the text around a tag or a split link leaves blanks at
		// its edges and doubled between words (GoldenDict normalizeHeadword).
		target := z.attrs["target"]
		if target == "" {
			target = tr.zoneText(z)
		}
		z.openHTML = linkMarkup(z.name, collapseSpace(target), z.attrs["dict"])
	}
	return z
}

// linkMarkup is the markup of [ref], <<…>> and [url].
func linkMarkup(tag, target, dict string) string {
	if tag == "url" {
		if !strings.Contains(target, "://") {
			target = "http://" + target
		}
		return "<a href=" + quoteAttr(target) + ">"
	}
	href := quoteAttr(htmlref.EntryHref(target))
	// dict="..." names ANOTHER dictionary by its #NAME, and the link is meant
	// to land there rather than in this one (lingvo-ref "Тэг [ref]···[/ref]").
	// Lingvo draws that name as the hover tooltip, so it rides in `title` -
	// which is also the only attribute besides href that the article sanitiser
	// keeps on an <a> (internal/server/articleformat.go); data-dict is the
	// machine-readable copy the live UI resolves to a dictionary id, and a
	// name that resolves to nothing degrades to an unscoped search.
	if d := strings.TrimSpace(dict); d != "" {
		return `<a class="wu-xref" data-dict=` + quoteAttr(d) + ` title=` + quoteAttr(d) + " href=" + href + ">"
	}
	return "<a href=" + href + ">"
}
