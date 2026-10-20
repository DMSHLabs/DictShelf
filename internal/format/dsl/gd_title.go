// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import "strings"

func gdTitleWithRepairs(line string, repair bool) titleResult {
	t := gdTitle(line)
	if !repair {
		return t
	}
	// Brackets used as editorial optionality are common in imported headings.
	// Leave escaped delimiters and DSL-tag-like bracket content untouched.
	var normalized strings.Builder
	for i := 0; i < len(line); {
		if line[i] == '\\' && i+1 < len(line) {
			normalized.WriteString(line[i : i+2])
			i += 2
			continue
		}
		if line[i] == '[' {
			if end := strings.IndexByte(line[i+1:], ']'); end >= 0 {
				end += i + 1
				inner := line[i+1 : end]
				_, isDSLTag := lookupTag(strings.TrimSpace(inner))
				if inner != "" && !isDSLTag && !strings.ContainsAny(inner, "[]") && !strings.HasPrefix(inner, "/") {
					normalized.WriteByte('(')
					normalized.WriteString(inner)
					normalized.WriteByte(')')
					i = end + 1
					continue
				}
			}
		}
		if line[i] == '{' {
			if end := strings.IndexByte(line[i+1:], '}'); end >= 0 {
				end += i + 1
				inner := line[i+1 : end]
				if inner != "" && !strings.ContainsAny(inner, "{}[]") {
					normalized.WriteByte('(')
					normalized.WriteString(inner)
					normalized.WriteByte(')')
					i = end + 1
					continue
				}
			}
		}
		normalized.WriteByte(line[i])
		i++
	}
	repaired := gdTitle(normalized.String())
	t.Keys = appendUnique(t.Keys, repaired.Keys...)
	// Slash alternatives are expanded only when every segment looks like a
	// word. All-uppercase short forms such as AC/DC are retained literally.
	variants := []string{collapseSpace(stripTitleSyntax(line))}
	for _, word := range strings.Fields(stripTitleSyntax(line)) {
		if !strings.Contains(word, "/") {
			continue
		}
		parts := strings.Split(word, "/")
		valid := len(parts) > 1
		for _, part := range parts {
			if len([]rune(part)) < 2 || part == "" || allUpperASCII(part) {
				valid = false
			}
		}
		if !valid {
			continue
		}
		var next []string
		for _, prefix := range variants {
			for _, part := range parts {
				next = append(next, strings.TrimSpace(strings.Replace(prefix, word, part, 1)))
			}
		}
		if len(next) > 32 {
			break
		}
		variants = next
	}
	return titleResult{Keys: appendUnique(t.Keys, variants...), Display: t.Display}
}

func stripTitleSyntax(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			continue
		}
		if strings.ContainsRune("(){}", rune(s[i])) {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
func allUpperASCII(s string) bool {
	has := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'z' {
			return false
		}
		if s[i] >= 'A' && s[i] <= 'Z' {
			has = true
		}
	}
	return has
}
func appendUnique(dst []string, values ...string) []string {
	seen := map[string]bool{}
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		v = collapseSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			dst = append(dst, v)
		}
	}
	return dst
}

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
