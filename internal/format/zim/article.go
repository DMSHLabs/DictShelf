// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/htmlref"
)

// A ZIM article is a complete HTML DOCUMENT - doctype, <html>, a <head> full
// of stylesheet links, then a page shell around the content - written to be
// served at its own path. Two transforms turn one into a wudict article body.
// Both belong here rather than in the server: converting a format's native
// markup is the format package's job (stardict/xdxf.go, dsl/transform.go),
// while mapping resource references to /res/ is htmlref's, and this leaves
// that half alone.

var (
	// linkTag matches a <link> element in the document head. The head of a
	// ZIM article is machine-generated and well-formed, which is the only
	// reason a pattern is enough here; the article BODY is parsed with a real
	// tokenizer below, because that is where malformed markup lives.
	linkTag = regexp.MustCompile(`(?is)<link\b[^>]*>`)

	// absoluteRef matches a reference that already names a scheme
	// ("https:", "mailto:", "data:").
	absoluteRef = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)
)

// articleHTML renders one stored article as a wudict article body. de is the
// entry the bytes are stored under - after any redirect - because every
// reference in them is relative to where they are stored.
func (d *Dict) articleHTML(de dirent, raw []byte) string {
	return rewriteRefs(articleBody(string(raw)), d.c.longPath(de), d.c.newNamespaces())
}

// articleBody extracts the inner HTML of <body>, keeping the head's
// stylesheet <link>s in front of it so the article still renders.
//
// This is 22% smaller than the stored document on a real wiktionary and drops
// per-entry page chrome, but it is not scraper-specific: a document with no
// <body> - which is what devdocs and zimit captures can contain - is returned
// whole rather than emptied.
//
// Tag names are ASCII, and they are matched as ASCII in s itself. Lower-casing
// a copy to search in, as this did, shifts every offset after a letter whose
// lower case is shorter - "İ" (2 bytes) lowers to "i" (1), the Kelvin sign
// (3) to "k" - so a Turkish article whose head held "İ" was cut at the wrong
// bytes.
func articleBody(s string) string {
	i := indexTag(s, "body")
	if i < 0 {
		return s
	}
	j := strings.IndexByte(s[i:], '>')
	if j < 0 {
		return s
	}
	start := i + j + 1
	end := lastIndexFold(s, "</body>")
	if end < start {
		end = len(s)
	}

	var styles strings.Builder
	for _, tag := range linkTag.FindAllString(s[:i], -1) {
		if strings.Contains(strings.ToLower(tag), "stylesheet") {
			styles.WriteString(tag)
		}
	}
	return styles.String() + s[start:end]
}

// indexTag is the offset of the first start tag named name (lower-case
// ASCII) in s, or -1.
func indexTag(s, name string) int {
	for i := 0; ; i++ {
		k := strings.IndexByte(s[i:], '<')
		if k < 0 {
			return -1
		}
		i += k
		if rest := s[i+1:]; hasPrefixASCIIFold(rest, name) {
			if len(rest) == len(name) {
				return i
			}
			switch rest[len(name)] {
			case '>', '/', ' ', '\t', '\n', '\r', '\f':
				return i
			}
		}
	}
}

// lastIndexFold is the offset of the last occurrence of sub (lower-case ASCII,
// starting with '<') in s, compared ASCII case-insensitively, or -1.
func lastIndexFold(s, sub string) int {
	for j := len(s); ; {
		k := strings.LastIndexByte(s[:j], '<')
		if k < 0 {
			return -1
		}
		if hasPrefixASCIIFold(s[k:], sub) {
			return k
		}
		j = k
	}
}

func hasPrefixASCIIFold(s, lower string) bool {
	if len(s) < len(lower) {
		return false
	}
	for i := 0; i < len(lower); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

// rewriteRefs resolves every reference of an article stored at base (its
// long path, see resolveRef) to the entry it names, as the browser resolves
// it against the page Kiwix serves at that path:
//
//   - a cross-reference (<a>/<area> href) to an article becomes an entry://
//     lookup, because the UI navigates on that scheme;
//   - anything else - an image, a stylesheet, a link to a stored file - becomes
//     the target's path from the archive's root, which the server's /res/
//     rewriter then maps as it maps any dictionary's resource name.
//
// Left relative, a reference was resolved by the browser against wudict's own
// URL: "../I/pic.png" under /res/{id}/ climbs out of the dictionary's folder
// ("/res/I/pic.png"), which broke every image, stylesheet and script of a
// pre-6.1 file and of devdocs ("../assets/index.css"); a link to an article
// in a subdirectory (devdocs' "../show-authors/index", wikibooks' subpage
// "Python/…") led nowhere at all.
func rewriteRefs(body, base string, newNS bool) string {
	if !htmlref.MayRef(body) {
		return body
	}
	return htmlref.Rewriter{
		URL: func(r htmlref.Ref) string {
			if r.Site == htmlref.SiteAttr && r.Attr == "href" && (r.Tag == "a" || r.Tag == "area") {
				return linkRef(base, r.URL, newNS)
			}
			return resourceRef(base, r.URL)
		},
	}.Rewrite(body)
}

// linkRef maps one cross-reference to an entry:// lookup of the article it
// names, or to the stored file it names, or returns it unchanged when it
// names neither.
func linkRef(base, ref string, newNS bool) string {
	p, query, frag, ok := resolveRef(base, ref)
	if !ok || query != "" {
		return ref // a query is a live-web reference, not a stored entry
	}
	name := p
	if !newNS {
		// The namespace is the first segment. A path with none is looked
		// up as an article, as libzim does (Archive::getEntryByPath).
		if len(p) >= 2 && p[1] == '/' {
			switch {
			case len(p) == 2:
				return ref // a namespace, not an entry in it
			case p[0] == 'A':
				name = p[2:]
			case p[0] == 'I' || p[0] == '-' || p[0] == 'J':
				return storedRef(p) + frag
			default:
				return ref
			}
		}
	} else if isFileLink(p) {
		return storedRef(p) + frag
	}
	if u, err := url.PathUnescape(name); err == nil {
		name = u
	}
	// Paths spell a space as '_'; the headword a user (or another dictionary)
	// would recognise does not.
	return htmlref.EntryHref(strings.ReplaceAll(name, "_", " ")) + frag
}

// isFileLink reports a link to a stored file rather than to an article: an
// asset name other than .html and .htm, which is how zimwriterfs and zimit
// name their articles.
func isFileLink(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".html", ".htm":
		return false
	}
	return dict.IsAssetName(p)
}

// resourceRef maps one resource reference to its path from the archive's
// root, or returns it unchanged when it names no stored entry.
func resourceRef(base, ref string) string {
	p, query, frag, ok := resolveRef(base, ref)
	if !ok {
		return ref
	}
	return storedRef(p) + query + frag
}

// storedRef writes a resolved path as a relative reference. A first segment
// holding ':' would read as a scheme, so it is written after "./" (RFC 3986
// §4.2), which the server strips (resURL).
func storedRef(p string) string {
	first, _, _ := strings.Cut(p, "/")
	if strings.IndexByte(first, ':') >= 0 {
		return "./" + p
	}
	return p
}

// resolveRef resolves ref, written in an article stored at base, as a browser
// resolves it against the URL Kiwix serves that article at (RFC 3986 §5.2:
// merged with the article's directory, then dot segments removed). base is
// the article's long path - "A/Foo" before 6.1, where the namespace is the
// first segment of every URL, "fmt/index" after. The path comes back still
// percent-encoded, query and fragment with their '?' and '#'.
//
// ok is false for a reference that names no stored entry: empty, fragment- or
// query-only, network-path, with a scheme, or the archive's root. A leading
// '/' is read as the archive's root, as the server reads one. A path may end
// in '/': zimit stores a site's index page as "example.com/".
func resolveRef(base, ref string) (p, query, frag string, ok bool) {
	if ref == "" || ref[0] == '#' || ref[0] == '?' || strings.HasPrefix(ref, "//") || absoluteRef.MatchString(ref) {
		return "", "", "", false
	}
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		ref, frag = ref[:i], ref[i:]
	}
	if i := strings.IndexByte(ref, '?'); i >= 0 {
		ref, query = ref[:i], ref[i:]
	}
	if strings.HasPrefix(ref, "/") {
		ref = ref[1:]
	} else if i := strings.LastIndexByte(base, '/'); i >= 0 {
		ref = escapeStored(base[:i+1]) + ref
	}
	p = removeDots(ref)
	return p, query, frag, p != ""
}

// escapeStored percent-encodes what a raw stored path holds that would change
// how it reads inside a reference: '%', '?', '#', space and control bytes.
// Every other byte - '/', non-ASCII - reads as itself.
func escapeStored(s string) string {
	i := strings.IndexFunc(s, func(r rune) bool { return escapesInRef(r) })
	if i < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for k := 0; k < len(s); k++ {
		if c := s[k]; c < 0x80 && escapesInRef(rune(c)) {
			b.WriteByte('%')
			b.WriteByte("0123456789ABCDEF"[c>>4])
			b.WriteByte("0123456789ABCDEF"[c&15])
			continue
		}
		b.WriteByte(s[k])
	}
	return b.String()
}

func escapesInRef(r rune) bool {
	return r == '%' || r == '?' || r == '#' || r <= ' ' || r == 0x7F
}

// removeDots removes the "." and ".." segments of a relative path (RFC 3986
// §5.2.4); a ".." above the root goes nowhere, as in a browser. Browsers also
// read "%2e" as a dot in a segment (WHATWG URL), so this does too.
func removeDots(p string) string {
	if !strings.Contains(p, ".") && !strings.Contains(p, "%2") {
		return p
	}
	segs := strings.Split(p, "/")
	out := segs[:0]
	for k, s := range segs {
		last := k == len(segs)-1
		switch dots(s) {
		case 1:
			if last {
				out = append(out, "")
			}
		case 2:
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			if last {
				out = append(out, "")
			}
		default:
			out = append(out, s)
		}
	}
	return strings.Join(out, "/")
}

// dots is 1 for a "." segment, 2 for "..", 0 otherwise.
func dots(s string) int {
	switch {
	case s == "." || strings.EqualFold(s, "%2e"):
		return 1
	case s == ".." || strings.EqualFold(s, ".%2e") || strings.EqualFold(s, "%2e.") || strings.EqualFold(s, "%2e%2e"):
		return 2
	}
	return 0
}

// trimRelative strips the leading "./", "../" and "/" segments of a resource
// name. A name that reached Resource from an article was resolved already
// (resolveRef); this is for one that comes from anywhere else.
func trimRelative(p string) string {
	for {
		switch {
		case strings.HasPrefix(p, "./"):
			p = p[2:]
		case strings.HasPrefix(p, "../"):
			p = p[3:]
		case strings.HasPrefix(p, "/"):
			p = p[1:]
		default:
			return p
		}
	}
}
