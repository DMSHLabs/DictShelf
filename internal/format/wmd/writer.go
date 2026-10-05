// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/htmlref"
)

// The writer (spec §6). A source is walked once: each body is converted at
// once and spooled to a temporary file, and only the names are kept, so that
// redirects - which may come before or after their targets - can be folded
// into the heading groups before anything is written (R6.4).

// Mode is how bodies are written (R6.5), chosen for the whole file.
type Mode int

const (
	ModeHTML  Mode = iota // the default: the article's HTML, verbatim, as one HTML block
	ModeClean             // markdown primitives only; destructive cleanup
)

// ParseMode reads a -mode value.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "html":
		return ModeHTML, nil
	case "clean":
		return ModeClean, nil
	}
	return 0, fmt.Errorf("mode %q: want clean or html", s)
}

func (m Mode) String() string {
	if m == ModeHTML {
		return "html"
	}
	return "clean"
}

// Head is what the file's header holds (R6.1, R6.3).
type Head struct {
	Name        string       // title; empty → Stem
	Stem        string       // the source file's stem
	From, To    string       // language tags, when known
	Fields      []dict.Field // further header fields, source order
	Description string       // HTML; the description body
}

type rec struct {
	names  []string
	off, n int64  // converted body in the spool; n == 0 for a redirect
	see    string // redirect target
}

// Writer builds one file. Its counts say what R6.2 and R6.4 changed.
type Writer struct {
	mode  Mode
	head  Head
	spool *os.File
	off   int64
	recs  []rec

	// Styles is the dictionary's display table (R6.6), used by `clean` from
	// the next article on; nil when the dictionary has no stylesheet.
	Styles htmlref.Styles

	// Progress, when set, is called by WriteTo with the entries written so
	// far, of Entries.
	Progress func(done int)

	Articles int // articles written
	Empty    int // articles dropped for an empty body
	Nameless int // entries dropped without a name
	Repaired int // names and values R6.2 had to repair
}

// NewWriter spools bodies in a temporary file in dir.
func NewWriter(h Head, mode Mode, dir string) (*Writer, error) {
	f, err := os.CreateTemp(dir, ".wudict-md-*.spool")
	if err != nil {
		return nil, err
	}
	return &Writer{mode: mode, head: h, spool: f}, nil
}

// Entries is the number of entries WriteTo goes through: every article and
// redirect added and kept.
func (w *Writer) Entries() int { return len(w.recs) }

// Close removes the spool.
func (w *Writer) Close() error {
	if w.spool == nil {
		return nil
	}
	name := w.spool.Name()
	w.spool.Close()
	w.spool = nil
	return os.Remove(name)
}

// body converts one article's HTML in the writer's mode.
func (w *Writer) body(src string) (string, error) {
	if w.mode == ModeHTML {
		return htmlBody(src), nil
	}
	return cleanBody(src, w.Styles)
}

// Article adds an entry with an HTML body. A body `clean` mode cannot write
// returns *CleanError, and the caller aborts (R6.8).
func (w *Writer) Article(names []string, body string) error {
	ns := w.cleanNames(names)
	if len(ns) == 0 {
		w.Nameless++
		return nil
	}
	md, err := w.body(body)
	if err != nil {
		return err
	}
	if md == "" {
		w.Empty++
		return nil
	}
	n, err := io.WriteString(w.spool, md)
	if err != nil {
		return err
	}
	w.recs = append(w.recs, rec{names: ns, off: w.off, n: int64(n)})
	w.off += int64(n)
	return nil
}

// Redirect adds an entry whose names lead to target.
func (w *Writer) Redirect(names []string, target string) {
	ns := w.cleanNames(names)
	t := w.clean(target)
	if len(ns) == 0 || t == "" {
		w.Nameless++
		return
	}
	w.recs = append(w.recs, rec{names: ns, see: t})
}

// clean is R6.2 on one name or value: valid UTF-8; CR, LF and TAB as SP;
// other control characters removed; trimmed.
func (w *Writer) clean(s string) string {
	c := strings.TrimFunc(strings.Map(func(r rune) rune {
		switch {
		case r == '\r' || r == '\n' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7F:
			return -1
		}
		return r
	}, validUTF8(s)), func(r rune) bool { return r == ' ' })
	if c != strings.Trim(s, " ") {
		w.Repaired++
	}
	return c
}

func (w *Writer) cleanNames(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		if s = w.clean(s); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// headingEsc writes a name as heading text that reads back as the same name
// (R6.2, R3.4).
func headingEsc(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\', '`', '*', '_', '[', ']', '<', '>', '&', '~':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	t := b.String()
	if j := len(strings.TrimRight(t, "#")); j < len(t) && (j == 0 || t[j-1] == ' ') {
		t = t[:j] + `\` + t[j:]
	}
	return t
}

// fieldKey maps a source field name to a header key (R6.3); "" when nothing
// of it is usable.
func fieldKey(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	k := b.String()
	switch {
	case k == "":
	case k[0] >= '0' && k[0] <= '9', k == "wudict", k == "from", k == "to", k == "meta":
		k = "x-" + k
	}
	return k
}

// fold resolves every redirect (R6.4): its names become aliases of every
// article its target names - exactly, else ignoring case - following chains
// through other redirects, and stopping at a repeat. It returns the extra
// names of each article, and the redirects that reach none.
func (w *Writer) fold() (extra map[int][]string, dangling map[int]bool) {
	artExact, artFold := map[string][]int{}, map[string][]int{}
	redExact, redFold := map[string][]int{}, map[string][]int{}
	for i, r := range w.recs {
		for _, n := range r.names {
			if r.see == "" {
				artExact[n] = append(artExact[n], i)
				artFold[strings.ToLower(n)] = append(artFold[strings.ToLower(n)], i)
			} else {
				redExact[n] = append(redExact[n], i)
				redFold[strings.ToLower(n)] = append(redFold[strings.ToLower(n)], i)
			}
		}
	}
	lookup := func(exact, folded map[string][]int, t string) []int {
		if v := exact[t]; len(v) > 0 {
			return v
		}
		return folded[strings.ToLower(t)]
	}
	var resolve func(t string, seen map[string]bool) []int
	resolve = func(t string, seen map[string]bool) []int {
		if seen[t] {
			return nil
		}
		seen[t] = true
		if a := lookup(artExact, artFold, t); len(a) > 0 {
			return a
		}
		var out []int
		for _, ri := range lookup(redExact, redFold, t) {
			out = append(out, resolve(w.recs[ri].see, seen)...)
		}
		return out
	}
	extra, dangling = map[int][]string{}, map[int]bool{}
	has := map[int]map[string]bool{}
	for i, r := range w.recs {
		if r.see == "" {
			continue
		}
		targets := resolve(r.see, map[string]bool{})
		if len(targets) == 0 {
			dangling[i] = true
			continue
		}
		for _, t := range targets {
			set := has[t]
			if set == nil {
				set = map[string]bool{}
				for _, n := range w.recs[t].names {
					set[n] = true
				}
				has[t] = set
			}
			for _, n := range r.names {
				if !set[n] {
					set[n] = true
					extra[t] = append(extra[t], n)
				}
			}
		}
	}
	return extra, dangling
}

// WriteTo writes the file (R6.1): the header, the description, then the
// articles and the redirects that reach no article, in source order.
func (w *Writer) WriteTo(out io.Writer) (int64, error) {
	b := bufio.NewWriterSize(out, 64<<10)
	var total int64
	put := func(s string) {
		n, _ := b.WriteString(s)
		total += int64(n)
	}

	name := w.clean(w.head.Name)
	if name == "" {
		name = w.clean(w.head.Stem)
	}
	if name == "" {
		name = "dictionary"
	}
	put("# " + headingEsc(name) + "\nwudict: 1")
	if v := w.clean(w.head.From); v != "" {
		put("\nfrom: " + v)
	}
	if v := w.clean(w.head.To); v != "" {
		put("\nto: " + v)
	}
	for _, f := range w.head.Fields {
		v := w.clean(f.Value)
		if v == "" {
			continue
		}
		if f.Name == "meta" { // this format's own escape, read back as is
			put("\nmeta: " + v)
		} else if k := fieldKey(f.Name); k != "" {
			put("\n" + k + ": " + v)
		} else if n := w.clean(f.Name); n != "" {
			put("\nmeta: " + n + ": " + v)
		}
	}
	if w.head.Description != "" {
		desc, err := w.body(w.head.Description)
		if err != nil {
			return total, fmt.Errorf("the description: %w", err)
		}
		if desc != "" {
			put("\n\n" + desc)
		}
	}

	extra, dangling := w.fold()
	w.Articles = 0
	buf := make([]byte, 0, 4096)
	for i, r := range w.recs {
		if w.Progress != nil {
			w.Progress(i + 1)
		}
		if r.see != "" && !dangling[i] {
			continue
		}
		put("\n")
		for _, ns := range [][]string{r.names, extra[i]} {
			for _, n := range ns {
				put("\n## " + headingEsc(n))
			}
		}
		if r.see != "" {
			put("\nsee: " + r.see)
			continue
		}
		if int64(cap(buf)) < r.n {
			buf = make([]byte, r.n)
		}
		buf = buf[:r.n]
		if _, err := w.spool.ReadAt(buf, r.off); err != nil {
			return total, err
		}
		put("\n\n")
		n, _ := b.Write(buf)
		total += int64(n)
		w.Articles++
	}
	put("\n")
	return total, b.Flush()
}
