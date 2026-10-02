// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import "strings"

func expandGDTitleTilde(head, parent string) string {
	var out strings.Builder
	for i := 0; i < len(head); i++ {
		if head[i] == '\\' && i+1 < len(head) {
			out.WriteString(head[i : i+2])
			i++
		} else if head[i] == '^' && i+1 < len(head) && head[i+1] == '~' {
			out.WriteString(titleEscape(flipCaseFirst(strings.TrimSpace(parent))))
			i++
		} else if head[i] == '~' {
			out.WriteString(titleEscape(strings.TrimSpace(parent)))
		} else {
			out.WriteByte(head[i])
		}
	}
	return out.String()
}

func gdTitle(line string) titleResult {
	t := transformTitle(line)
	// Keep the existing display form, but expand nested optional parts with
	// GoldenDict's 32-key ceiling rather than the flat six-part grammar.
	var clean strings.Builder
	depth := 0
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '\\' && i+1 < len(line) {
			if depth == 0 {
				clean.WriteByte(c)
				clean.WriteByte(line[i+1])
			}
			i++
			continue
		}
		if c == '{' {
			depth++
			continue
		}
		if c == '}' && depth > 0 {
			depth--
			continue
		}
		if depth == 0 {
			clean.WriteByte(c)
		}
	}
	raw := clean.String()
	var variants []string
	var expand func(string)
	steps := 0
	expand = func(s string) {
		steps++
		if len(variants) >= 32 || steps > 4096 {
			return
		}
		for i := 0; i < len(s); i++ {
			if s[i] == '\\' {
				i++
				continue
			}
			if s[i] != '(' {
				continue
			}
			depth, end := 1, i+1
			for ; end < len(s); end++ {
				if s[end] == '\\' {
					end++
					continue
				}
				if s[end] == '(' {
					depth++
				}
				if s[end] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if depth == 0 {
				expand(s[:i] + s[i+1:end] + s[end+1:])
				expand(s[:i] + s[end+1:])
			} else {
				expand(s[:i] + s[i+1:])
				expand(s[:i])
			}
			return
		}
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
				continue
			}
			if s[i] != ')' {
				b.WriteByte(s[i])
			}
		}
		key := collapseSpace(b.String())
		if key != "" {
			for _, v := range variants {
				if v == key {
					return
				}
			}
			variants = append(variants, key)
		}
	}
	if len([]rune(raw)) > 500 {
		return t
	}
	expand(raw)
	t.Keys = variants
	return t
}
