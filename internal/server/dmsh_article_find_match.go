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
	container := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(source), container)
	if err != nil {
		return nil
	}
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
			for _, r := range node.Data {
				articleFindAppendRune(&current, r)
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
	for _, node := range nodes {
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

func articleFindWordMatches(text []rune, query, mode string, matchCase bool, bound int) [][2]int {
	words := articleFindWords([]rune(query), matchCase)
	if len(words) == 0 {
		return nil
	}
	tokens := articleFindWords(text, matchCase)
	var out [][2]int
	if mode == "phrase" {
		for i := 0; i+len(words) <= len(tokens); i++ {
			matched := true
			for j := range words {
				if !articleFindEqual(tokens[i+j].value, words[j].value, true) {
					matched = false
					break
				}
			}
			if matched {
				out = append(out, [2]int{tokens[i].start, tokens[i+len(words)-1].end})
				i += len(words) - 1
			}
		}
		return out
	}
	if bound < 1 {
		bound = 10
	}
	need := make(map[string]int)
	for _, word := range words {
		need[articleFindFold(word.value, matchCase)]++
	}
	for i := 0; i < len(tokens); i++ {
		first := articleFindFold(tokens[i].value, matchCase)
		if need[first] == 0 {
			continue
		}
		held := map[string]int{first: 1}
		left, last := len(words)-1, i
		for j := i + 1; left > 0 && j < len(tokens) && j-i <= bound+1; j++ {
			key := articleFindFold(tokens[j].value, matchCase)
			if held[key] >= need[key] {
				continue
			}
			held[key]++
			left--
			last = j
		}
		if left == 0 {
			out = append(out, [2]int{tokens[i].start, tokens[last].end})
			i = last
		}
	}
	return out
}

type articleFindWord struct {
	value []rune
	start int
	end   int
}

func articleFindWords(text []rune, matchCase bool) []articleFindWord {
	var words []articleFindWord
	for i := 0; i < len(text); {
		if !articleFindWordRune(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && articleFindWordRune(text[i]) {
			i++
		}
		word := append([]rune(nil), text[start:i]...)
		if !matchCase {
			for j := range word {
				word[j] = unicode.ToLower(word[j])
			}
		}
		words = append(words, articleFindWord{value: word, start: start, end: i})
	}
	return words
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

func articleFindFold(value []rune, matchCase bool) string {
	if matchCase {
		return string(value)
	}
	folded := make([]rune, len(value))
	for i, r := range value {
		folded[i] = unicode.ToLower(r)
	}
	return string(folded)
}
