// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/DMSHLabs/DictShelf/internal/hilite"
)

// The phrase and near readings are painted HERE, from the rung's own phrases.
//
// internal/hilite cannot do this job for them. It is installed as the HTML
// rewriter's Text hook and sees one text node at a time, so a phrase the
// dictionary broke across markup - `<a>джазовый</a><span><a>кларнетист</a></span>`
// - is beyond it: either nothing is marked, or (with WithWordFallback) both
// words are marked as if they were two separate matches, and the reader's
// arrows stop twice on one phrase.
//
// An occurrence of these readings is a run of TOKENS, not of text nodes, and
// the model that says where it starts and ends is the one the navigation
// counts: the article's text folded into blocks (articleFindGroupNodes), the
// rung's phrases matched over it (articleFindMatches). Painting takes the same
// parse and the same matcher, so the marks a reader sees and the positions the
// arrows walk cannot disagree - and the client no longer has to re-derive
// either.
//
// The classes are the whole vocabulary of a reading's answer:
//
//	wu-hl     the FIRST fragment of an occurrence - the navigation stop
//	wu-hl-ex  the other query words of the same occurrence, painted alike
//	wu-hl-ex2 the words between a near window's terms: painted, not a match
//
// Every fragment carries data-wu-match="N", the occurrence's number in the
// article, which is also the navigation cursor's matchIndex. wu-cur is the one
// class the reader's client writes, on the occurrence being walked to.

// articleFindMarking is one answer's mark plan: the rung's phrases as the FTS5
// expression saw them, and how they group into navigation occurrences.
type articleFindMarking struct {
	phrases   []hilite.Phrase
	near      bool
	nearWords int
}

// articleFindMarkSet is one matcher run: the terms of one phrase (or of the
// whole near bag) and the rule that turns their matches into occurrences.
type articleFindMarkSet struct {
	terms []articleFindTerm
	mode  string
	bound int
}

// articleFindPaint is one range of one text node to wrap.
type articleFindPaint struct {
	start, end int // rune offsets within the node's own text
	class      string
	occurrence int
}

// markArticleFindHTML returns body with the marking's occurrences painted.
// When nothing matches, the input is returned unchanged, byte for byte.
//
// Only bodies that carry marks are re-serialized. That is a real trade: the
// article's text is preserved exactly, but its markup comes back canonical
// (quoted attribute values, spelled-out entities, closed void elements). The
// alternative - splicing the marks into the source bytes - would need a second
// text-node model that could drift from the one the matcher used, and a drift
// there paints the wrong word.
func markArticleFindHTML(body string, m articleFindMarking) string {
	sets := articleFindMarkingSets(m)
	if body == "" || len(sets) == 0 {
		return body
	}
	roots, err := articleFindParse(body)
	if err != nil || len(roots) == 0 {
		return body
	}
	// ParseFragment detaches its roots, and painting splices nodes in place, so
	// they need a parent to splice into.
	container := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, root := range roots {
		container.AppendChild(root)
	}
	paints := articleFindPlan(articleFindGroupNodes(roots, true), sets, m.near)
	if len(paints) == 0 {
		return body
	}
	articleFindApply(paints)
	var b strings.Builder
	b.Grow(len(body) + len(body)/16)
	for child := container.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&b, child); err != nil {
			return body
		}
	}
	return b.String()
}

// articleFindMarkingSets lowers a mark plan into matcher runs. Phrase: each of
// the rung's phrases is found on its own. Near: the rung's terms are one bag,
// and a window closes when the last of them has been found.
func articleFindMarkingSets(m articleFindMarking) []articleFindMarkSet {
	if m.near {
		var bag []articleFindTerm
		for _, p := range m.phrases {
			bag = append(bag, articleFindPhraseTerms(p)...)
		}
		if len(bag) == 0 {
			return nil
		}
		return []articleFindMarkSet{{terms: bag, mode: "near", bound: m.nearWords}}
	}
	sets := make([]articleFindMarkSet, 0, len(m.phrases))
	for _, p := range m.phrases {
		if terms := articleFindPhraseTerms(p); len(terms) > 0 {
			sets = append(sets, articleFindMarkSet{terms: terms, mode: "phrase"})
		}
	}
	return sets
}

// articleFindPhraseTerms lowers one Phrase into matchable terms. The text is
// tokenized the way FTS5 tokenized the quoted phrase, so a word carrying
// punctuation contributes the tokens the index actually matched, and the
// phrase's trailing star lands on the last of them.
func articleFindPhraseTerms(p hilite.Phrase) []articleFindTerm {
	var terms []articleFindTerm
	for _, term := range p {
		terms = append(terms, articleFindTerms([]rune(term.Text), articleFindFoldIndex, false)...)
	}
	if n := len(terms); n > 0 && p[len(p)-1].Prefix {
		terms[n-1].prefix = true
	}
	return terms
}

// articleFindPlan finds the occurrences of every set in every text block and
// turns them into per-node paint ranges, numbered in document order - the
// order the navigation walks. It stops at hilite.MaxMarks occurrences and never
// splits one: the number is put on all of an occurrence's fragments or on none.
func articleFindPlan(groups []articleFindTextGroup, sets []articleFindMarkSet, near bool) map[*html.Node][]articleFindPaint {
	paints := map[*html.Node][]articleFindPaint{}
	occurrence := 0
	for gi := range groups {
		group := &groups[gi]
		tokens := articleFindTokens(group.text, articleFindFoldIndex)
		if len(tokens) == 0 {
			continue
		}
		var matches []articleFindMatch
		for _, set := range sets {
			matches = append(matches, articleFindMatches(tokens, set.terms, set.mode, set.bound)...)
		}
		if len(matches) > 1 {
			// Sets are matched one after another; within a block the numbering
			// must still follow the page.
			sort.SliceStable(matches, func(i, j int) bool {
				return matches[i].fragments[0].start < matches[j].fragments[0].start
			})
		}
		for _, match := range matches {
			if occurrence >= hilite.MaxMarks {
				return paints
			}
			for ri, run := range articleFindRuns(group, match.fragments, !near) {
				class := "wu-hl"
				if ri > 0 {
					class = "wu-hl-ex"
					if near {
						articleFindAddBridge(group, paints, run.gapFrom, run.from, occurrence)
					}
				}
				articleFindAddPaint(group, paints, run.from, run.to, class, occurrence)
			}
			occurrence++
		}
	}
	return paints
}

// articleFindRun is one painted span of an occurrence: a fragment, or several
// fragments the dictionary left standing together.
type articleFindRun struct {
	from, to int
	// gapFrom is where the text before this run starts, when the run needed a
	// near bridge painted behind it.
	gapFrom int
}

// articleFindRuns groups an occurrence's fragments into painted runs.
//
// A phrase whose words all stand inside ONE text node is one run and therefore
// one mark - that is what "the words adjacent and in this order" looks like.
// Markup inside the span breaks the run: the first fragment stays the stop and
// the following ones carry the continuation class. A near window never merges
// - its label promises the query's words apart - and the words between two
// fragments are painted as the bridge.
func articleFindRuns(group *articleFindTextGroup, fragments []articleFindToken, merge bool) []articleFindRun {
	if merge && len(fragments) > 1 &&
		articleFindSameSegment(group, fragments[0].start, fragments[len(fragments)-1].end-1) {
		return []articleFindRun{{from: fragments[0].start, to: fragments[len(fragments)-1].end, gapFrom: fragments[0].start}}
	}
	runs := make([]articleFindRun, 0, len(fragments))
	for i, fragment := range fragments {
		run := articleFindRun{from: fragment.start, to: fragment.end, gapFrom: fragment.start}
		if i > 0 {
			run.gapFrom = fragments[i-1].end
		}
		runs = append(runs, run)
	}
	return runs
}

// articleFindSameSegment reports whether two group offsets are read from the
// same text node - the test for "these words stand together in the article".
func articleFindSameSegment(group *articleFindTextGroup, a, b int) bool {
	if a > b {
		a, b = b, a
	}
	for _, segment := range group.segments {
		if a >= segment.start && a < segment.end && b >= segment.start && b < segment.end {
			return true
		}
	}
	return false
}

// articleFindAddBridge paints the words between two fragments of one near
// window. A gap that holds no word is not painted: the reading is about the
// words standing between the query's words, not about the punctuation.
func articleFindAddBridge(group *articleFindTextGroup, paints map[*html.Node][]articleFindPaint, from, to, occurrence int) {
	if from >= to || !articleFindHasWordRune(group.text[from:to]) {
		return
	}
	articleFindAddPaint(group, paints, from, to, "wu-hl-ex2", occurrence)
}

// articleFindAddPaint records one group-text range on every text node it
// crosses: a token can span markup, so a fragment is painted in as many nodes
// as it lives in.
func articleFindAddPaint(group *articleFindTextGroup, paints map[*html.Node][]articleFindPaint, from, to int, class string, occurrence int) {
	if to <= from {
		return
	}
	for _, segment := range group.segments {
		if segment.end <= from || segment.start >= to {
			continue
		}
		start, end, ok := articleFindSegmentRunes(group, segment, max(from, segment.start), min(to, segment.end))
		if !ok {
			continue
		}
		paints[segment.node] = append(paints[segment.node], articleFindPaint{start: start, end: end, class: class, occurrence: occurrence})
	}
}

// articleFindSegmentRunes maps a range of the group's folded text back to rune
// offsets in the node's own text. Appending normalises whitespace and drops a
// repeated space, so the two counts can differ; walking the node under the same
// rule reproduces the mapping. A dropped space inside the range is painted too
// - it is a space, so nothing about the page changes.
func articleFindSegmentRunes(group *articleFindTextGroup, segment articleFindTextSegment, from, to int) (int, int, bool) {
	at, first, last := segment.start, -1, -1
	prevSpace := segment.start > 0 && group.text[segment.start-1] == ' '
	for i, r := range []rune(segment.node.Data) {
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		if at >= from && at < to {
			if first < 0 {
				first = i
			}
			last = i
		}
		at++
	}
	if first < 0 {
		return 0, 0, false
	}
	return first, last + 1, true
}

func articleFindHasWordRune(text []rune) bool {
	for _, r := range text {
		if articleFindWordRune(r) {
			return true
		}
	}
	return false
}

// articleFindApply rewrites each painted text node in place: the node is split
// around every painted range, and each range is wrapped in the element its
// class asks for. The vocabulary of artmark: a match is a <mark>, the words
// between a near window's terms are a <span> that is painted but is not one.
func articleFindApply(paints map[*html.Node][]articleFindPaint) {
	for node, list := range paints {
		parent := node.Parent
		if parent == nil {
			continue
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].start < list[j].start })
		runes := []rune(node.Data)
		last := 0
		var out []*html.Node
		for _, paint := range list {
			if paint.start < last || paint.end > len(runes) || paint.end <= paint.start {
				continue
			}
			if paint.start > last {
				out = append(out, articleFindTextNode(string(runes[last:paint.start])))
			}
			tag, dataAtom := "mark", atom.Mark
			if paint.class == "wu-hl-ex2" {
				tag, dataAtom = "span", atom.Span
			}
			element := &html.Node{Type: html.ElementNode, Data: tag, DataAtom: dataAtom, Attr: []html.Attribute{
				{Key: "class", Val: paint.class},
				{Key: "data-wu-match", Val: strconv.Itoa(paint.occurrence)},
			}}
			element.AppendChild(articleFindTextNode(string(runes[paint.start:paint.end])))
			out = append(out, element)
			last = paint.end
		}
		if last < len(runes) {
			out = append(out, articleFindTextNode(string(runes[last:])))
		}
		for _, n := range out {
			parent.InsertBefore(n, node)
		}
		parent.RemoveChild(node)
	}
}

func articleFindTextNode(text string) *html.Node {
	return &html.Node{Type: html.TextNode, Data: text}
}

// countArticleFindHeads counts the navigation stops in a marked body: one
// wu-hl head per occurrence. It is a tokenizer scan rather than a substring
// match, so a class list carrying more than one name, or attributes in another
// order, cannot silently zero the count - the number the arrows use is read
// from the same markup the reader sees.
//
// The second value says the count MAY BE a lower bound: the marker stops at
// hilite.MaxMarks per article, so a body holding exactly that many heads may
// have been cut there. The count is still what the arrows walk; the CALLER must
// not report it as exact (the session keeps its total marked provisional).
func countArticleFindHeads(body string) (int, bool) {
	z := html.NewTokenizer(strings.NewReader(body))
	count := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return count, count >= hilite.MaxMarks
		case html.StartTagToken:
		default:
			continue
		}
		name, more := z.TagName()
		if string(name) != "mark" {
			continue
		}
		for more {
			key, value, next := z.TagAttr()
			if string(key) == "class" {
				for _, class := range strings.Fields(string(value)) {
					if class == "wu-hl" {
						count++
						break
					}
				}
			}
			more = next
		}
	}
}
