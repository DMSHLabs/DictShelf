// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package facet

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The groups after language and language pair come from a text file the user
// owns (D162): groups.ini beside the wudict.toml in effect, and while there is
// none, the copy embedded here. One file, not built-ins plus additions - so
// deleting a line removes a group, and there is no merge to reason about.
//
//	[Section]                a facet: a "Group by" heading
//	Name = text, `regex`     a group in it; ':' works as well as '='
//	; or #                   a comment, on its own line or after the items
//
// A plain item matches anywhere in the title or the file name, in any case. A
// `regex` is Go RE2, case-insensitive, read verbatim between the backticks -
// commas, ';' and '#' inside it are part of the pattern - and tested against
// the title and the file name separately, so ^ and $ mean what they say.
//
// Ids are the names, trimmed and lower-cased: what a remembered choice is
// stored under, so an edit that only changes case keeps it.

//go:embed groups.ini
var DefaultText string

var defaultRules, _ = Parse(DefaultText)

// Default is the embedded groups.ini, parsed.
func Default() *Rules { return defaultRules }

// Rules is a parsed groups.ini: its facets and their groups, in file order.
type Rules struct{ facets []ruleFacet }

type ruleFacet struct {
	id, label string
	groups    []ruleGroup
}

type ruleGroup struct {
	id, label string
	subs      []string         // plain items, lower-cased
	res       []*regexp.Regexp // `regex` items, case-insensitive
	items     []string         // both, as the editor lists them: plain lower-cased, regex in backticks
	seen      map[string]bool  // parse-time dedup by items' spelling; nil once Parse returns
}

// Problem is a line Parse skipped, or an item on it, for the editor to point at.
type Problem struct {
	Line int    `json:"line"` // 1-based; 0 is the file as a whole
	Text string `json:"text"`
	Msg  string `json:"msg"`
	// More is set on a last entry that stands for the problems past
	// maxProblems, which are counted but not listed.
	More int `json:"more,omitempty"`
}

// CountProblems is how many problems a list stands for, the unlisted included.
func CountProblems(ps []Problem) int {
	n := 0
	for _, p := range ps {
		if p.More > 0 {
			n += p.More
		} else {
			n++
		}
	}
	return n
}

// MyGroups is the section a group line written before any [Section] joins.
const MyGroups = "My groups"

// The bounds a section shares with what a hidden "Group by" chip is
// remembered as (server/prefs.go facetIDs): longer or more could not stay
// hidden, so they are refused here, where the user can see why.
const (
	maxSectionBytes = 64
	maxSections     = 32
)

// What one file may cost, whoever wrote it. A problem quotes its line and the
// regex package quotes its pattern, so both are clipped, and past maxProblems
// they are counted, not listed: one 256 KB line of bad items would otherwise
// be 65,536 copies of itself (about 16 GB of JSON). Every regex is run against
// every dictionary on every list load: 500 cost a few milliseconds, 30,000
// cost seconds.
const (
	maxProblems   = 100
	maxProblemLen = 200 // bytes of Text and of Msg
	maxRegexes    = 500
)

// clip shortens s to at most n bytes on a rune boundary, marking the cut.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

// Parse reads a groups.ini. What it cannot use is skipped and reported; the
// rest still applies, so one typo never costs the whole file.
func Parse(text string) (*Rules, []Problem) {
	r := &Rules{}
	var probs []Problem
	more, regexes := 0, 0
	text = strings.TrimPrefix(text, "\uFEFF") // Notepad's byte-order mark
	cur := -1                                 // index into r.facets; -1 before any [Section]
	skip := false                             // inside a section that was refused
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		bad := func(msg string) {
			if len(probs) == maxProblems {
				more++
				return
			}
			probs = append(probs, Problem{Line: i + 1, Text: clip(line, maxProblemLen), Msg: clip(msg, maxProblemLen)})
		}
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			end := strings.IndexByte(line, ']')
			var name, rest string
			if end > 0 {
				name, rest = strings.TrimSpace(line[1:end]), strings.TrimSpace(line[end+1:])
			}
			skip = true
			switch {
			case end < 0:
				bad("a section name ends with ]")
			case rest != "" && rest[0] != ';' && rest[0] != '#':
				bad("only a comment can follow ]")
			case name == "":
				bad("the section has no name")
			case reserved(name):
				bad("[" + name + "] is the name of a built-in section")
			case len(name) > maxSectionBytes:
				bad("a section name is at most 64 bytes")
			default:
				if c, ok := r.facet(name); ok {
					cur, skip = c, false
				} else {
					bad("at most 32 sections")
				}
			}
			continue
		}
		if skip {
			continue
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			bad("no = between the group's name and what it matches")
			continue
		}
		name := strings.TrimSpace(line[:sep])
		if name == "" {
			bad("no group name before =")
			continue
		}
		its, errs := splitItems(line[sep+1:])
		for _, e := range errs {
			bad(e)
		}
		var g *ruleGroup
		for _, it := range its {
			key := it.text
			if it.re {
				key = "`" + it.text + "`"
			} else {
				it.text = strings.ToLower(it.text)
				key = it.text
			}
			if !it.re && utf8.RuneCountInString(it.text) < 2 {
				bad(fmt.Sprintf("%q is too short: at least 2 characters", it.text))
				continue
			}
			var re *regexp.Regexp
			if it.re {
				if regexes == maxRegexes {
					bad("at most 500 regular expressions")
					continue
				}
				var err error
				if re, err = regexp.Compile("(?i)" + it.text); err != nil {
					_, err = regexp.Compile(it.text)                 // the message without the (?i)
					bad("invalid regular expression: " + reMsg(err)) // the message quotes the pattern
					continue
				}
			}
			if g == nil {
				if cur < 0 {
					c, ok := r.facet(MyGroups)
					if !ok {
						bad("at most 32 sections")
						break
					}
					cur = c
				}
				g = r.facets[cur].group(name)
			}
			if g.seen[key] {
				continue
			}
			g.seen[key] = true
			g.items = append(g.items, key)
			if re != nil {
				g.res = append(g.res, re)
				regexes++
			} else {
				g.subs = append(g.subs, it.text)
			}
		}
		if g == nil && len(errs) == 0 && len(its) == 0 {
			bad("nothing to match after =")
		}
	}
	for i := range r.facets {
		for j := range r.facets[i].groups {
			r.facets[i].groups[j].seen = nil
		}
	}
	if more > 0 {
		probs = append(probs, Problem{Msg: fmt.Sprintf("and %d more problems", more), More: more})
	}
	return r, probs
}

// reMsg is a compile error as a user reads it: the parser's own words, without
// Go's "error parsing regexp:" lead.
func reMsg(err error) string {
	if err == nil {
		return "not valid"
	}
	return strings.TrimPrefix(err.Error(), "error parsing regexp: ")
}

type item struct {
	text string
	re   bool
}

// splitItems reads what follows '=': ',' separates items, ';' or '#' ends the
// line, and a `backtick` span is one regex item read verbatim. Errors name what
// was dropped; the items around them still count.
func splitItems(v string) ([]item, []string) {
	var out []item
	var errs []string
	var b strings.Builder
	flush := func() {
		if t := strings.TrimSpace(b.String()); t != "" {
			out = append(out, item{text: t})
		}
		b.Reset()
	}
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case ',':
			flush()
		case ';', '#':
			flush()
			return out, errs
		case '`':
			if strings.TrimSpace(b.String()) != "" {
				errs = append(errs, "text before a `regular expression`: put a comma between them")
			}
			b.Reset()
			j := strings.IndexByte(v[i+1:], '`')
			if j < 0 {
				errs = append(errs, "a regular expression ends with `")
				return out, errs
			}
			pat := v[i+1 : i+1+j]
			i += j + 1
			if strings.TrimSpace(pat) == "" {
				errs = append(errs, "an empty regular expression ``")
			} else {
				out = append(out, item{text: pat, re: true})
			}
			// only blanks may follow, up to the next item or a comment
			for i+1 < len(v) && (v[i+1] == ' ' || v[i+1] == '\t') {
				i++
			}
			if i+1 < len(v) && v[i+1] != ',' && v[i+1] != ';' && v[i+1] != '#' {
				errs = append(errs, "text after a `regular expression`: put a comma between them")
				for i+1 < len(v) && v[i+1] != ',' && v[i+1] != ';' && v[i+1] != '#' {
					i++
				}
			}
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return out, errs
}

// reserved: the facets derived in code, by id or by the label they show; a
// section of either would merge into them or sit beside them under the same
// heading.
func reserved(name string) bool {
	switch strings.ToLower(name) {
	case "lang", "pair", "language", "language pair":
		return true
	}
	return false
}

// facet finds or adds a section; false when adding would pass maxSections.
func (r *Rules) facet(label string) (int, bool) {
	id := strings.ToLower(label)
	for i, f := range r.facets {
		if f.id == id {
			return i, true
		}
	}
	if len(r.facets) == maxSections {
		return 0, false
	}
	r.facets = append(r.facets, ruleFacet{id: id, label: label})
	return len(r.facets) - 1, true
}

// group finds or adds a group, so a second line naming it extends it.
func (f *ruleFacet) group(label string) *ruleGroup {
	id := strings.ToLower(label)
	for i := range f.groups {
		if f.groups[i].id == id {
			return &f.groups[i]
		}
	}
	f.groups = append(f.groups, ruleGroup{id: id, label: label, seen: map[string]bool{}})
	return &f.groups[len(f.groups)-1]
}

// match returns the groups one of whose items is in the title or the file
// name: one group per (facet, group), each facet ranked by rank.
func (r *Rules) match(title, file string) []Group {
	lt, lf := strings.ToLower(title), strings.ToLower(file)
	var out []Group
	for i, f := range r.facets {
		fo := r.rank(i)
		for _, g := range f.groups {
			if g.hit(title, file, lt, lf) {
				out = append(out, Group{F: f.id, FL: f.label, FO: fo, V: g.id, VL: g.label})
			}
		}
	}
	return out
}

// rank places facet i: a section of the built-in list after Language pair, any
// other - the user's own - above Language; both in file order.
func (r *Rules) rank(i int) int {
	if r == defaultRules || defaultRules.has(r.facets[i].id) {
		return foRules + i
	}
	return foMine + i
}

func (r *Rules) has(id string) bool {
	for _, f := range r.facets {
		if f.id == id {
			return true
		}
	}
	return false
}

func (g *ruleGroup) hit(title, file, lt, lf string) bool {
	for _, s := range g.subs {
		if strings.Contains(lt, s) || strings.Contains(lf, s) {
			return true
		}
	}
	for _, re := range g.res {
		if re.MatchString(title) || (file != "" && re.MatchString(file)) {
			return true
		}
	}
	return false
}

// FacetInfo is a facet as the editor lists it.
type FacetInfo struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Groups []GroupInfo `json:"groups"`
}

// GroupInfo is a group as the editor lists it: Items is what it matches, as
// parsed, so the editor can tell which groups a save changed.
type GroupInfo struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Items []string `json:"items"`
}

// Facets lists what the rules define, for the editor to show what a save
// produced.
func (r *Rules) Facets() []FacetInfo {
	out := make([]FacetInfo, 0, len(r.facets))
	for _, f := range r.facets {
		fi := FacetInfo{ID: f.id, Label: f.label, Groups: make([]GroupInfo, 0, len(f.groups))}
		for _, g := range f.groups {
			fi.Groups = append(fi.Groups, GroupInfo{ID: g.id, Label: g.label, Items: g.items})
		}
		out = append(out, fi)
	}
	return out
}
