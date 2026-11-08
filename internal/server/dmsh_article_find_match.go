// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"strings"
	"unicode"
	"unicode/utf16"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

type articleFindMatchOptions struct {
	query     string
	mode      string
	matchCase bool
	examples  bool
	nearWords int
	variants  []string
}

type articleFindOccurrence struct {
	Group int `json:"group"`
	Start int `json:"start"`
	End   int `json:"end"`
}

type articleFindTextGroup struct {
	text    []rune
	endsUTF []int
	// segments say which text nodes the group's runes came from, in document
	// order. A token can span two of them - the group's text is a concatenation
	// - so painting a range walks the segments it covers.
	segments []articleFindTextSegment
}

// articleFindTextSegment is one text node's contribution to its group.
type articleFindTextSegment struct {
	node       *html.Node
	start, end int // rune offsets within the group's text
}

var articleFindBlockTags = map[string]bool{
	"p": true, "div": true, "li": true, "dt": true, "dd": true, "td": true, "th": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true,
}

var articleFindSkipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "button": true,
	"input": true, "select": true, "textarea": true, "audio": true, "video": true,
	"svg": true, "math": true,
}

var articleFindExampleClasses = map[string]bool{
	"wu-xonly": true, "wu-sec": true, "dsl_opt": true, "wu-exonly": true,
	"wu-ex": true, "dsl_ex": true,
}

// matchArticleFindHTML returns matches in the text streams that the article
// reader presents. Offsets are UTF-16 positions within a block, matching DOM
// Range offsets used by WebView2 and Android WebView.
func matchArticleFindHTML(source string, options articleFindMatchOptions) []articleFindOccurrence {
	groups := articleFindHTMLGroups(source, options.examples)
	query := articleFindNormalizeQuery(options.query)
	if query == "" {
		return nil
	}
	var out []articleFindOccurrence
	for groupIndex, group := range groups {
		for _, span := range matchArticleFindText(group.text, options, query) {
			if span[0] < 0 || span[1] <= span[0] || span[1] > len(group.endsUTF) {
				continue
			}
			start := 0
			if span[0] > 0 {
				start = group.endsUTF[span[0]-1]
			}
			out = append(out, articleFindOccurrence{Group: groupIndex, Start: start, End: group.endsUTF[span[1]-1]})
		}
	}
	return out
}

func articleFindHTMLGroups(source string, includeExamples bool) []articleFindTextGroup {
	nodes, err := articleFindParse(source)
	if err != nil {
		return nil
	}
	return articleFindGroupNodes(nodes, includeExamples)
}

// articleFindParse parses an article body as a fragment. The matchers and the
// painter read the same tree from here, so a painted occurrence is found
// exactly where the occurrence list says it is.
func articleFindParse(source string) ([]*html.Node, error) {
	container := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	return html.ParseFragment(strings.NewReader(source), container)
}

func articleFindGroupNodes(roots []*html.Node, includeExamples bool) []articleFindTextGroup {
	groups := make([]articleFindTextGroup, 0, 8)
	var current articleFindTextGroup
	flush := func() {
		if len(current.text) > 0 {
			groups = append(groups, current)
		}
		current = articleFindTextGroup{}
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			start := len(current.text)
			for _, r := range node.Data {
				articleFindAppendRune(&current, r)
			}
			// Whitespace folding can drop every rune of a node; a segment that
			// contributed nothing has nothing to paint.
			if end := len(current.text); end > start {
				current.segments = append(current.segments, articleFindTextSegment{node: node, start: start, end: end})
			}
			return
		}
		if node.Type != html.ElementNode {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			return
		}
		tag := strings.ToLower(node.Data)
		if articleFindSkipTags[tag] || articleFindHidden(node, includeExamples) {
			flush()
			return
		}
		if tag == "details" && !articleFindHasAttr(node, "open") {
			flush()
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && child.Data == "summary" {
					walk(child)
				}
			}
			flush()
			return
		}
		isBlock := articleFindBlockTags[tag] || articleFindHasClass(node, "wu-m")
		if isBlock {
			flush()
		}
		if tag == "br" {
			if len(current.text) > 0 && current.text[len(current.text)-1] != ' ' {
				articleFindAppendRune(&current, ' ')
			}
			if isBlock {
				flush()
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if isBlock {
			flush()
		}
	}
	for _, node := range roots {
		walk(node)
	}
	flush()
	return groups
}

func articleFindAppendRune(group *articleFindTextGroup, r rune) {
	if unicode.IsSpace(r) {
		r = ' '
		if n := len(group.text); n > 0 && group.text[n-1] == ' ' {
			return
		}
	}
	group.text = append(group.text, r)
	group.endsUTF = append(group.endsUTF, len(utf16.Encode([]rune{r})))
	if len(group.endsUTF) > 1 {
		group.endsUTF[len(group.endsUTF)-1] += group.endsUTF[len(group.endsUTF)-2]
	}
}

func articleFindHidden(node *html.Node, includeExamples bool) bool {
	if articleFindHasAttr(node, "hidden") {
		return true
	}
	if !includeExamples {
		for _, class := range strings.Fields(articleFindAttr(node, "class")) {
			if articleFindExampleClasses[class] {
				return true
			}
		}
	}
	style := strings.ToLower(strings.ReplaceAll(articleFindAttr(node, "style"), " ", ""))
	return strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden") || strings.Contains(style, "visibility:collapse")
}

func articleFindAttr(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func articleFindHasAttr(node *html.Node, name string) bool {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return true
		}
	}
	return false
}

func articleFindHasClass(node *html.Node, name string) bool {
	for _, class := range strings.Fields(articleFindAttr(node, "class")) {
		if class == name {
			return true
		}
	}
	return false
}

func articleFindNormalizeQuery(query string) string {
	query = strings.TrimSpace(query)
	if len(query) > 1 && strings.HasPrefix(query, "\"") && strings.HasSuffix(query, "\"") {
		query = query[1 : len(query)-1]
	}
	return strings.Join(strings.Fields(query), " ")
}

func matchArticleFindText(text []rune, options articleFindMatchOptions, query string) [][2]int {
	mode := options.mode
	if mode == "phrase" || mode == "near" {
		return articleFindWordMatches(text, query, mode, options.matchCase, options.nearWords)
	}
	variants := options.variants
	if len(variants) == 0 {
		variants = []string{query}
	}
	if mode == "wordLemma" && len(variants) == 1 {
		mode = "word"
	}
	needles := make([][]rune, 0, len(variants))
	for _, variant := range variants {
		variant = articleFindNormalizeQuery(variant)
		if variant != "" {
			needles = append(needles, []rune(variant))
		}
	}
	var out [][2]int
	for at := 0; at < len(text); {
		bestEnd := -1
		for _, needle := range needles {
			end := at + len(needle)
			if end > len(text) || !articleFindEqual(text[at:end], needle, options.matchCase) {
				continue
			}
			beforeWord := at > 0 && articleFindWordRune(text[at-1])
			afterWord := end < len(text) && articleFindWordRune(text[end])
			if (mode == "prefix" || mode == "word" || mode == "wordLemma") && beforeWord ||
				(mode == "suffix" || mode == "word" || mode == "wordLemma") && afterWord {
				continue
			}
			if end > bestEnd {
				bestEnd = end
			}
		}
		if bestEnd >= 0 {
			out = append(out, [2]int{at, bestEnd})
			at = bestEnd
		} else {
			at++
		}
	}
	return out
}

// articleFindFoldMode selects how tokens and terms are compared.
type articleFindFoldMode int

const (
	// articleFindFoldExact compares the text as it stands: the dialog's
	// Match case.
	articleFindFoldExact articleFindFoldMode = iota
	// articleFindFoldCase lowercases per rune: the dialog without Match case.
	articleFindFoldCase
	// articleFindFoldIndex folds the way the full-text index was built
	// (dict.Fold), so a mark cannot disagree with the search that produced it.
	articleFindFoldIndex
)

// fold reduces one run of runes under the mode's rule.
func (m articleFindFoldMode) fold(value []rune) string {
	switch m {
	case articleFindFoldCase:
		folded := make([]rune, len(value))
		for i, r := range value {
			folded[i] = unicode.ToLower(r)
		}
		return string(folded)
	case articleFindFoldIndex:
		return dict.Fold(string(value))
	default:
		return string(value)
	}
}

// articleFindToken is one word of a group's text: where it is, and what it
// reads as under the mode's folding.
type articleFindToken struct {
	folded     string
	start, end int
}

// articleFindTerm is one token of the query: what a token must read as, and
// whether the token may run past it (FTS5's trailing star).
type articleFindTerm struct {
	value  string
	prefix bool
}

// articleFindMatch is one occurrence and the tokens it is made of, in document
// order: a phrase's words, or the words a near window closed on. The first
// fragment is the navigation stop; the article is painted with it as wu-hl and
// the rest as its continuations.
type articleFindMatch struct {
	fragments []articleFindToken
}

func articleFindTermMatches(term articleFindTerm, tok articleFindToken) bool {
	if term.prefix {
		return strings.HasPrefix(tok.folded, term.value)
	}
	return tok.folded == term.value
}

// articleFindTokens splits text into words under the mode's folding. A token
// spans markup: the group's text is a concatenation, so `<b>джазо</b>вый` is
// one token, which is what the article index sees too.
func articleFindTokens(text []rune, fold articleFindFoldMode) []articleFindToken {
	var tokens []articleFindToken
	for i := 0; i < len(text); {
		if !articleFindWordRune(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && articleFindWordRune(text[i]) {
			i++
		}
		tokens = append(tokens, articleFindToken{folded: fold.fold(text[start:i]), start: start, end: i})
	}
	return tokens
}

// articleFindTerms tokenizes a query the way FTS5 tokenizes the literal it was
// quoted into: a word carrying punctuation contributes the tokens the
// tokenizer finds in it, and a trailing star belongs to the last of them.
func articleFindTerms(text []rune, fold articleFindFoldMode, prefixLast bool) []articleFindTerm {
	tokens := articleFindTokens(text, fold)
	terms := make([]articleFindTerm, 0, len(tokens))
	for i, tok := range tokens {
		terms = append(terms, articleFindTerm{value: tok.folded, prefix: prefixLast && i == len(tokens)-1})
	}
	return terms
}

// articleFindMatches finds the occurrences of terms in one group's tokens.
// phrase: the terms in order, on consecutive tokens. near: a window started by
// any term and closed by the last one it still needed, within bound tokens of
// the start - the rule the reading's label states.
func articleFindMatches(tokens []articleFindToken, terms []articleFindTerm, mode string, bound int) []articleFindMatch {
	if len(tokens) == 0 || len(terms) == 0 {
		return nil
	}
	if mode == "phrase" {
		return articleFindPhraseMatches(tokens, terms)
	}
	if bound < 1 {
		bound = 10
	}
	return articleFindNearMatches(tokens, terms, bound)
}

func articleFindPhraseMatches(tokens []articleFindToken, terms []articleFindTerm) []articleFindMatch {
	var out []articleFindMatch
	for i := 0; i+len(terms) <= len(tokens); {
		matched := true
		for j := range terms {
			if !articleFindTermMatches(terms[j], tokens[i+j]) {
				matched = false
				break
			}
		}
		if !matched {
			i++
			continue
		}
		out = append(out, articleFindMatch{fragments: append([]articleFindToken(nil), tokens[i:i+len(terms)]...)})
		i += len(terms)
	}
	return out
}

func articleFindNearMatches(tokens []articleFindToken, terms []articleFindTerm, bound int) []articleFindMatch {
	// Repeated query words need one token each, so the terms are counted per
	// distinct pattern - the same arithmetic the window has always run on.
	var distinct []articleFindTerm
	var need []int
	for _, term := range terms {
		at := -1
		for i := range distinct {
			if distinct[i] == term {
				at = i
				break
			}
		}
		if at < 0 {
			distinct = append(distinct, term)
			need = append(need, 0)
			at = len(distinct) - 1
		}
		need[at]++
	}
	var out []articleFindMatch
	for i := 0; i < len(tokens); i++ {
		first := -1
		for d := range distinct {
			if articleFindTermMatches(distinct[d], tokens[i]) {
				first = d
				break
			}
		}
		if first < 0 {
			continue
		}
		held := make([]int, len(distinct))
		held[first] = 1
		left, last := len(terms)-1, i
		fragments := []articleFindToken{tokens[i]}
		for j := i + 1; left > 0 && j < len(tokens) && j-i <= bound+1; j++ {
			next := -1
			for d := range distinct {
				if held[d] < need[d] && articleFindTermMatches(distinct[d], tokens[j]) {
					next = d
					break
				}
			}
			if next < 0 {
				continue
			}
			held[next]++
			left--
			last = j
			fragments = append(fragments, tokens[j])
		}
		if left == 0 {
			out = append(out, articleFindMatch{fragments: fragments})
			i = last
		}
	}
	return out
}

// articleFindWordMatches is the dialog's literal phrase and near reading: the
// words as typed, whole unless Match case says otherwise, one span per
// occurrence.
func articleFindWordMatches(text []rune, query, mode string, matchCase bool, bound int) [][2]int {
	fold := articleFindFoldCase
	if matchCase {
		fold = articleFindFoldExact
	}
	terms := articleFindTerms([]rune(query), fold, false)
	matches := articleFindMatches(articleFindTokens(text, fold), terms, mode, bound)
	if len(matches) == 0 {
		return nil
	}
	out := make([][2]int, 0, len(matches))
	for _, m := range matches {
		out = append(out, [2]int{m.fragments[0].start, m.fragments[len(m.fragments)-1].end})
	}
	return out
}

func articleFindWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || r == '_'
}

func articleFindEqual(a, b []rune, matchCase bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if matchCase {
			if a[i] != b[i] {
				return false
			}
		} else if unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return false
		}
	}
	return true
}
