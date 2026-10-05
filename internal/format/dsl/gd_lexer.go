// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import "strings"

// gdLexer preserves GoldenDict-compatible attribute and media tokenization.
type gdLexer struct {
	input string
	pos   int
}

func (tr *gdLexer) end() bool { return tr.pos >= len(tr.input) }

func (tr *gdLexer) next() byte { c := tr.input[tr.pos]; tr.pos++; return c }

func (tr *gdLexer) follows(s string) bool {
	return strings.HasPrefix(tr.input[tr.pos:], s)
}

func (tr *gdLexer) skipAny(chars string) {
	for tr.pos < len(tr.input) && strings.IndexByte(chars, tr.input[tr.pos]) >= 0 {
		tr.pos++
	}
}

func (tr *gdLexer) lexAttrs() map[string]string {
	attrs := map[string]string{}
	name := ""
	for {
		if tr.end() {
			if name != "" {
				attrs[name] = ""
			}
			return attrs
		}
		c := tr.next()
		if c == ']' {
			if name != "" {
				attrs[name] = ""
			}
			return attrs
		}
		if c == '=' {
			tr.skipAny(" \t")
			attrs[name] = tr.lexAttrValue()
			name = ""
			continue
		}
		if c == ' ' || c == '\t' {
			if name != "" {
				attrs[name] = ""
				name = ""
			}
			tr.skipAny(" \t")
			continue
		}
		name += tr.input[tr.pos-1 : tr.pos]
	}
}

func (tr *gdLexer) lexAttrValue() string {
	if tr.end() {
		return ""
	}
	c := tr.next()
	quote := byte(0)
	var val strings.Builder
	if c == '\'' || c == '"' {
		quote = c
	} else {
		val.WriteByte(c)
	}
	for {
		if tr.end() {
			return val.String()
		}
		c = tr.next()
		if c == '\\' {
			if tr.end() {
				return val.String()
			}
			val.WriteByte(tr.next())
			continue
		}
		if c == ']' {
			tr.pos--
			return val.String()
		}
		if quote != 0 && c == quote {
			return val.String()
		}
		if quote == 0 && (c == ' ' || c == '\t') {
			return val.String()
		}
		val.WriteByte(c)
	}
}

func (tr *gdLexer) collectMediaName() string {
	var b strings.Builder
	for !tr.end() {
		if tr.follows("[preview]") {
			tr.pos += len("[preview]")
			continue
		}
		if tr.follows("[/preview]") {
			tr.pos += len("[/preview]")
			continue
		}
		c := tr.next()
		if c == '[' {
			tr.pos--
			break
		}
		b.WriteByte(c)
	}
	return strings.TrimSpace(b.String())
}
