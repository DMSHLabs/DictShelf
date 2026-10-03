// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"regexp"
	"strings"
)

// The embedded pages, scripts and stylesheets are served without their
// comments. The sources keep them - they are this program's documentation -
// but a browser has no use for half of index.html, and every load of the page
// paid for it. Stripping runs once, at start, on the embedded bytes, so there
// is no second file to build, version or keep in step, and a tag-less
// `go build` serves exactly what `make build` does.
//
// Only a comment that occupies whole lines is removed:
//
//   - <!-- … --> in HTML, outside <script>;
//   - a /* … */ that starts a line and ends one, in CSS, in <style> and in
//     scripts;
//   - a line whose first non-blank characters are //, in scripts.
//
// A comment sharing a line with code stays: the line rule takes nearly all of
// them, and leaving the rest is what keeps this from needing a full parser.
// So does a licence notice: the files carry it on purpose.
// Scripts are read with just enough lexing to know when a line starts inside a
// string, a template literal or a block comment - a template literal can hold
// lines that look like comments and are content, such as the Custom styles
// presets, whose CSS comments are inserted into the user's stylesheet.

type assetKind int

const (
	htmlAsset assetKind = iota
	jsAsset
	cssAsset
)

// webAsset is src as it is served.
func webAsset(src []byte, kind assetKind) []byte {
	return []byte(stripComments(string(src), kind))
}

var scriptRE = regexp.MustCompile(`(?s)(<script\b[^>]*>)(.*?)(</script>)`)

func stripComments(s string, kind assetKind) string {
	switch kind {
	case jsAsset:
		return stripJS(s)
	case cssAsset:
		return stripBlocks(s)
	}
	var b strings.Builder
	last := 0
	for _, m := range scriptRE.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(stripBlocks(stripHTMLComments(s[last:m[3]])))
		b.WriteString(stripJS(s[m[4]:m[5]]))
		last = m[5]
	}
	b.WriteString(stripBlocks(stripHTMLComments(s[last:])))
	return b.String()
}

// stripHTMLComments removes every <!-- … -->, and with it the lines it stood
// on alone.
func stripHTMLComments(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "<!--")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+4:], "-->")
		if j < 0 {
			break
		}
		end := i + 4 + j + 3
		if licence(s[i:end]) {
			b.WriteString(s[:end])
			s = s[end:]
			continue
		}
		lineStart := strings.LastIndexByte(s[:i], '\n') + 1
		if blank(s[lineStart:i]) && blank(restOfLine(s, end)) {
			i, end = lineStart, nextLine(s, end)
		}
		b.WriteString(s[:i])
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}

// stripBlocks removes the /* … */ comments that start a line and end one: CSS,
// where nothing else can look like a comment.
func stripBlocks(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		line, rest := cutLine(s)
		if t := strings.TrimLeft(line, " \t"); strings.HasPrefix(t, "/*") {
			start := len(line) - len(t)
			if j := strings.Index(s[start+2:], "*/"); j >= 0 {
				end := start + 2 + j + 2
				if blank(restOfLine(s, end)) && !licence(s[start:end]) {
					s = s[nextLine(s, end):]
					continue
				}
			}
		}
		b.WriteString(line)
		s = rest
	}
	return b.String()
}

// stripJS removes the whole-line comments of a script, deciding at each line
// start whether that line begins in code or inside a construct that crossed a
// line break (a template literal or a block comment).
func stripJS(s string) string {
	var b strings.Builder
	lx := jsLexer{}
	for len(s) > 0 {
		line, rest := cutLine(s)
		if lx.inCode() {
			t := strings.TrimLeft(line, " \t")
			start := len(line) - len(t)
			if strings.HasPrefix(t, "//") {
				s = rest
				continue
			}
			if strings.HasPrefix(t, "/*") {
				if j := strings.Index(s[start+2:], "*/"); j >= 0 {
					end := start + 2 + j + 2
					if blank(restOfLine(s, end)) && !licence(s[start:end]) {
						s = s[nextLine(s, end):]
						continue
					}
				}
			}
		}
		lx.feed(line)
		b.WriteString(line)
		s = rest
	}
	return b.String()
}

// jsLexer carries what survives a line break: an open block comment, an open
// template literal, and the ${ … } nesting that can contain more of both.
type jsLexer struct {
	block bool
	stack []int // each entry: brace depth inside a ${ … } of a template literal
	tmpl  bool  // inside a template literal's text
	prev  byte  // last significant code character, for telling a regex from a division
}

func (l *jsLexer) inCode() bool { return !l.block && !l.tmpl }

func (l *jsLexer) feed(line string) {
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case l.block:
			if c == '*' && i+1 < len(line) && line[i+1] == '/' {
				l.block = false
				i++
			}
		case l.tmpl:
			switch {
			case c == '\\':
				i++
			case c == '`':
				l.tmpl = false
				l.prev = '`'
			case c == '$' && i+1 < len(line) && line[i+1] == '{':
				l.tmpl = false
				l.stack = append(l.stack, 0)
				l.prev = '{'
				i++
			}
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			return // the rest of the line is a comment
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			l.block = true
			i++
		case c == '"' || c == '\'':
			i = skipQuoted(line, i)
			l.prev = c
		case c == '`':
			l.tmpl = true
		case c == '/' && regexCanStart(l.prev, line[:i]):
			i = skipRegex(line, i)
			l.prev = '/'
		case c == '{':
			if n := len(l.stack); n > 0 {
				l.stack[n-1]++
			}
			l.prev = c
		case c == '}':
			if n := len(l.stack); n > 0 {
				if l.stack[n-1] == 0 {
					l.stack = l.stack[:n-1]
					l.tmpl = true // back into the template's text
					continue
				}
				l.stack[n-1]--
			}
			l.prev = c
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
		default:
			l.prev = c
		}
	}
}

// regexCanStart reports that a / begins a regular expression rather than a
// division: after an operator, an opening bracket, a keyword that takes an
// expression, or nothing at all. prev is the last significant character,
// before the text preceding the slash on its line.
func regexCanStart(prev byte, before string) bool {
	if prev == 0 || strings.IndexByte("(,=:[!&|?{};+-*%~^<>", prev) >= 0 {
		return true
	}
	t := strings.TrimRight(before, " \t")
	i := len(t)
	for i > 0 && isIdent(t[i-1]) {
		i--
	}
	switch t[i:] {
	case "return", "typeof", "case", "do", "else", "in", "of", "new", "delete",
		"void", "throw", "instanceof", "yield", "await":
		return true
	}
	return false
}

func isIdent(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func skipQuoted(line string, i int) int {
	q := line[i]
	for i++; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case q:
			return i
		}
	}
	return i
}

func skipRegex(line string, i int) int {
	class := false
	for i++; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\\':
			i++
		case c == '[':
			class = true
		case c == ']':
			class = false
		case c == '/' && !class:
			return i
		}
	}
	return i
}

func cutLine(s string) (line, rest string) {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i+1], s[i+1:]
	}
	return s, ""
}

func restOfLine(s string, i int) string {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return s[i : i+j]
	}
	return s[i:]
}

func nextLine(s string, i int) int {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return i + j + 1
	}
	return len(s)
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func licence(comment string) bool { return strings.Contains(comment, "SPDX-License-Identifier") }
