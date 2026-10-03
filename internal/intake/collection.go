// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"bytes"
	"context"
	"errors"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
)

// Collections: one link that names several dictionaries.
//
// A web folder of dictionary files, a .txt list of them, or several links
// pasted at once. The rule that keeps this small is that a collection is not
// a second pipeline: it is the files a person would otherwise have pasted one
// by one, shown on ONE screen before anything is downloaded, and then fed to
// the ordinary download-sniff-install path one dictionary at a time.
//
// The screen is the existing one. The listing is presented to Sniff as an
// archive whose directory is the list of links (remoteArchive), so grouping
// ("x.mdx" + "x.mdd" is one dictionary), the completeness check and the
// "already installed" marking are the rules an archive already gets - and the
// media files are offered as Extras, the per-file tickboxes the page and the
// Android dialog already draw. Nothing about a collection is stored: asking
// again is how a user learns what changed, and the answer comes from names
// and sizes, the test D134 already makes (existing.go).
//
// There is no manifest format. A folder page is read for its links; a text
// list is one file name or link per line, "# " naming it. Everything else on
// either is ignored, so a directory index, a README-like list or a pasted
// forum post all work, and nobody has a syntax to learn.

// The share link: https://legbehindneck.com/wudict#<link>. The part after
// "#" is the link; the page at that address is only what a browser shows when
// Android did not hand the link to the app, and it never sees the fragment.
const (
	ShareHost = "legbehindneck.com"
	SharePath = "/wudict"
)

const (
	// maxListBytes bounds a folder page or a list. Real ones are kilobytes;
	// this is a bound on what a link to something else entirely can cost.
	maxListBytes = 1 << 20
	// maxLinks bounds the files one collection may name - and so the HEAD
	// requests spent describing them. A folder of every Wiktionary is ~200.
	maxLinks = 500
	// headWorkers is how many of those requests run at once: enough that a
	// folder of forty files answers in a second or two, few enough that no
	// host reads it as an attack.
	headWorkers = 4
	// sniffHeadBytes is how much of a name-ambiguous file (".md") is read to
	// decide whether it is a dictionary: dict.Claims reads the head only.
	sniffHeadBytes = 64 << 10
)

// ErrSharePage is the share link with nothing after "#": wudict's own page of
// dictionary links, not a link to any dictionary. Read as a folder page it
// lists nothing (its rows are drawn by script), which said "no dictionaries
// found" about a page that is full of them.
var ErrSharePage = errors.New("that is wudict's page of dictionary links: open it in a browser and choose a dictionary there")

// IsSharePage reports the share link that carries no link: the page itself.
func IsSharePage(s string) bool {
	s = strings.TrimSpace(s)
	page, frag, _ := strings.Cut(s, "#")
	if strings.TrimSpace(frag) != "" {
		return false
	}
	u, err := url.Parse(page)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http":
	default:
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return host == ShareHost && strings.TrimSuffix(u.Path, "/") == SharePath
}

// ErrNoLinks is a folder page or list that names no dictionary this build
// reads - which, for a page, is the answer the single-file path gave before
// collections existed, now said about the right thing.
var ErrNoLinks = errors.New("no dictionaries found at that link")

// Unwrap returns the link a share link carries, or s unchanged when s is not
// one. The fragment is taken as written: the link inside keeps its own
// %-escapes, and one that arrived escaped whole ("https%3A%2F%2F…", as some
// senders re-encode a fragment) is decoded once.
func Unwrap(s string) string {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, '#')
	if i < 0 {
		return s
	}
	u, err := url.Parse(s[:i])
	if err != nil {
		return s
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http":
	default:
		return s
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != ShareHost || strings.TrimSuffix(u.Path, "/") != SharePath {
		return s
	}
	inner := strings.TrimSpace(s[i+1:])
	if l := strings.ToLower(inner); strings.HasPrefix(l, "https%3a") || strings.HasPrefix(l, "http%3a") {
		if d, err := url.QueryUnescape(inner); err == nil {
			inner = d
		}
	}
	// The short form: "#/dict/euskera/" is a path on the share link's own
	// site. One leading slash only - "//other.org/x" is a link to another
	// host written without its scheme, and the short form must never be a way
	// to point somewhere the long form would have had to name.
	if strings.HasPrefix(inner, "/") && !strings.HasPrefix(inner, "//") {
		inner = strings.ToLower(u.Scheme) + "://" + u.Host + inner
	}
	return inner
}

// UnwrapAll is Unwrap for a share link that carries several links:
//
//	https://legbehindneck.com/wudict#https://a.org/x.mdx,https://b.org/x.mdd
//
// The fragment is split where a new link begins - "http://" or "https://"
// right after a separator: a comma, a semicolon, a bar or whitespace, written
// plainly or %-escaped as a messenger may have rewritten it. A separator must
// precede the scheme, so a link that carries another inside its query
// ("?u=https://…") stays whole, and a comma inside a link is only a separator
// when a link starts right after it. Anything that is not such a list is
// Unwrap's single answer.
func UnwrapAll(s string) []string {
	one := Unwrap(s)
	if one == strings.TrimSpace(s) {
		return []string{one} // not a share link
	}
	starts := linkStarts(one)
	if len(starts) < 2 || starts[0] != 0 {
		return []string{one}
	}
	out := make([]string, 0, len(starts))
	for i, at := range starts {
		end := len(one)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if v := trimSeparators(one[at:end]); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// linkSeparators are what may stand between two links in a share link, plain
// and %-escaped (space, comma, semicolon, bar, newline).
var linkSeparators = []string{" ", "\t", "\n", "\r", ",", ";", "|",
	"%20", "%2C", "%2c", "%3B", "%3b", "%7C", "%7c", "%0A", "%0a", "%0D", "%0d", "%09"}

// linkStarts lists where a link begins in s: at 0, or after a separator.
func linkStarts(s string) []int {
	lower := strings.ToLower(s)
	var out []int
	for i := 0; i < len(lower); i++ {
		if !strings.HasPrefix(lower[i:], "https://") && !strings.HasPrefix(lower[i:], "http://") {
			continue
		}
		if i == 0 {
			out = append(out, i)
			continue
		}
		for _, sep := range linkSeparators {
			if strings.HasSuffix(s[:i], sep) {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

// trimSeparators strips the separators that end one link of a list.
func trimSeparators(s string) string {
	for {
		t := strings.TrimSpace(s)
		for _, sep := range linkSeparators {
			t = strings.TrimSuffix(t, sep)
		}
		if t == s {
			return t
		}
		s = t
	}
}

// listing is what a folder page, a list or pasted text named.
type listing struct {
	title string
	links []*url.URL
	// text says the links came from a list or pasted text rather than a
	// folder page. A folder page shows every file there is; a list may name
	// only the main files and leave the rest to be found beside them, exactly
	// as a single pasted link does (probe.go).
	text bool
}

// parseText reads a list: one entry per line, a file name (resolved against
// base) or a link. The first "# " line is the title; other lines starting
// with "#" and blank lines are ignored. A line that holds links among other
// words - a pasted sentence - contributes its links. base may be nil, for
// pasted text, and then a bare name has nothing to be resolved against.
func parseText(text string, base *url.URL) listing {
	l := listing{text: true}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			if l.title == "" && strings.HasPrefix(line, "# ") {
				l.title = strings.TrimSpace(line[2:])
			}
			continue
		}
		if found := linksIn(line); len(found) > 0 {
			for _, s := range found {
				for _, w := range UnwrapAll(s) {
					if u, err := url.Parse(w); err == nil {
						l.links = append(l.links, u)
					}
				}
			}
			continue
		}
		if base == nil {
			continue
		}
		// A bare name, as `ls -1` writes it. Escaped as a path segment first,
		// so "Oxford Advanced.mdx" and "C#.mdx" are names and not a query or
		// a fragment of the list's own URL.
		if u, err := base.Parse(escapeRef(line)); err == nil {
			l.links = append(l.links, u)
		}
	}
	return l
}

// escapeRef escapes a bare file name for resolution against a base URL,
// keeping "/" so "sub/x.mdx" stays a relative path.
func escapeRef(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// linksIn picks the http(s) links out of free text, trimming the punctuation
// prose wraps them in: "(see https://x/a.zip)." yields the link, not ")." .
func linksIn(text string) []string {
	var out []string
	for _, tok := range strings.Fields(text) {
		lower := strings.ToLower(tok)
		i := strings.Index(lower, "https://")
		if j := strings.Index(lower, "http://"); j >= 0 && (i < 0 || j < i) {
			i = j
		}
		if i < 0 {
			continue
		}
		s := strings.TrimRight(tok[i:], `.,;:!?)]}>"'`)
		if len(s) > len("https://") {
			out = append(out, s)
		}
	}
	return out
}

// hrefRE finds the targets of a page's links. A regular expression and not
// an HTML parser, deliberately: all that is wanted is the href of each <a>,
// the page is hostile and bounded, and a folder index is the simplest HTML
// there is. What a sloppy match costs is a link that is not a dictionary,
// which the name filter below drops.
var hrefRE = regexp.MustCompile(`(?is)<a\b[^>]*?\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)

// parseHTML reads a folder page: every link on it, resolved against base.
func parseHTML(body string, base *url.URL) listing {
	var l listing
	for _, m := range hrefRE.FindAllStringSubmatch(body, -1) {
		ref := html.UnescapeString(m[1] + m[2] + m[3])
		if ref == "" || strings.HasPrefix(ref, "#") {
			continue
		}
		if u, err := base.Parse(ref); err == nil {
			l.links = append(l.links, u)
		}
	}
	return l
}

// fetchListing reads a link as a folder page or a list. It is asked only
// after the download path has refused the link as not a dictionary, so it
// costs nothing on the common path; the refusal came from the headers, and
// this is the one extra request that reads the body.
func fetchListing(ctx context.Context, f Fetcher, raw string) (listing, error) {
	u, err := f.Check(raw)
	if err != nil {
		return listing{}, err
	}
	// A Nextcloud share's page is drawn by script and names no file; its
	// WebDAV listing does (nextcloud.go).
	if l, ok, err := shareListing(ctx, f, u); ok {
		return l, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return listing{}, err
	}
	req.Header.Set("Accept", "text/html, text/plain;q=0.9, */*;q=0.1")
	resp, err := f.Client().Do(req)
	if err != nil {
		return listing{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return listing{}, ErrNotArchive
	}
	base := resp.Request.URL // after redirects: "…/euskera" answers as "…/euskera/"
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	ct = strings.ToLower(ct)
	isHTML := ct == "text/html" || ct == "application/xhtml+xml"
	isText := strings.HasPrefix(ct, "text/") ||
		((ct == "" || ct == "application/octet-stream") && strings.HasSuffix(strings.ToLower(base.Path), ".txt"))
	if !isHTML && !isText {
		return listing{}, ErrNotArchive
	}
	if isHTML && driveID(u) != "" {
		// Drive's download address answers with a page only when it will not
		// hand the file over; the page's links are Google's, not a folder.
		return listing{}, ErrDriveRefused
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxListBytes+1))
	if err != nil {
		return listing{}, err
	}
	if len(body) > maxListBytes {
		return listing{}, ErrNotArchive
	}
	var l listing
	if isHTML {
		l = parseHTML(string(body), base)
	} else {
		l = parseText(string(body), base)
	}
	if l.title == "" {
		if n := dispositionName(resp.Header); n != "" && driveID(u) != "" {
			l.title = strings.TrimSuffix(n, ".txt") // "download" says nothing
		} else {
			l.title = titleOf(base)
		}
	}
	return l, nil
}

// titleOf is what a collection is called when it does not say: its folder,
// or its list file without ".txt".
func titleOf(u *url.URL) string {
	name := path.Base(strings.TrimSuffix(u.Path, "/"))
	if s, err := url.PathUnescape(name); err == nil {
		name = s
	}
	name = strings.TrimSuffix(name, ".txt")
	if name == "" || name == "." || name == "/" {
		return u.Hostname()
	}
	return name
}

// remoteFile is one file a collection will download: the name the link
// implies, and what the site said about it. The name is the plan's, not a
// promise about the disk - Fetch may save under another, and fetchRow puts a
// row's files back on one stem.
type remoteFile struct {
	name     string
	url      string
	size     int64
	modified time.Time
}

// colRow is one row of a collection: the files installed with the
// dictionary whatever the user ticks (main first). Its media are Extras.
type colRow struct {
	files []remoteFile
}

// collection is a listing turned into the screen: candidates (one per row,
// in the same order as rows), the media offered beside them, and what to
// download for each.
type collection struct {
	cands  []Candidate
	extras []Extra
	rows   []colRow
}

// build describes the files a listing names, without downloading any of
// them: which exist and how big they are (HEAD), how they group into
// dictionaries (Sniff, over remoteArchive), and which are already installed
// (markExisting, from names and sizes).
func build(ctx context.Context, f Fetcher, dest string, lib []string, l listing) (collection, error) {
	files := pickFiles(f, l.links)
	if len(files) == 0 {
		return collection{}, ErrNoLinks
	}
	drive := false
	for _, rf := range files {
		if u, err := url.Parse(rf.url); err == nil && driveID(u) != "" {
			drive = true
		}
	}
	describe(ctx, f, files)
	// Only now is every name known: a Drive file was named by the server's
	// answer, or by nothing - and then Drive sent a page, not the file. Two links can turn out to be one name; the first stays.
	var alive []*remoteFile
	named := map[string]bool{}
	for _, rf := range files {
		if rf == nil || !takeable(rf.name) || named[strings.ToLower(rf.name)] {
			continue
		}
		named[strings.ToLower(rf.name)] = true
		alive = append(alive, rf)
	}
	if len(alive) == 0 && ctx.Err() == nil {
		if drive {
			return collection{}, ErrDriveRefused
		}
		return collection{}, ErrNoLinks
	}
	if l.text {
		alive = append(alive, companionsOf(ctx, f, alive)...)
	}
	if ctx.Err() != nil {
		return collection{}, ctx.Err()
	}
	byName := make(map[string]*remoteFile, len(alive))
	var entries []Entry
	var archives []*remoteFile
	for _, rf := range alive {
		byName[rf.name] = rf
		if dict.ClassifyName(rf.name) == dict.KindOther {
			if SupportedArchive(rf.name) {
				archives = append(archives, rf)
			}
			continue
		}
		entries = append(entries, Entry{Name: rf.name, Base: rf.name, Size: rf.size, Compressed: rf.size})
	}
	cands, err := Sniff(&remoteArchive{ctx: ctx, f: f, entries: entries, files: byName})
	if err != nil {
		return collection{}, err
	}
	// Marked BEFORE the media come out of Files: "unchanged" is a claim about
	// every file of the dictionary, and an .mdd that grew is a change.
	markExisting(dest, lib, cands)
	for _, rf := range archives {
		// An archive is a row of its own, named after itself: what is inside
		// is not known until it is here, and is then installed whole, less
		// what the library already holds unchanged.
		cands = append(cands, Candidate{
			Name: strings.TrimSuffix(rf.name, path.Ext(rf.name)), Format: strings.ToLower(path.Ext(rf.name)),
			Main: rf.name, Files: []string{rf.name}, sizes: []int64{rf.size}, Size: rf.size,
		})
	}
	sort.SliceStable(cands, func(i, j int) bool { return strings.ToLower(cands[i].Name) < strings.ToLower(cands[j].Name) })

	var c collection
	for i := range cands {
		cand := &cands[i]
		ext := dict.MainExt(cand.Main)
		var row colRow
		keep, keepSizes := cand.Files[:0:0], cand.sizes[:0:0]
		for k, name := range cand.Files {
			rf := byName[name]
			if rf == nil {
				continue
			}
			if k > 0 && isMedia(name, ext) {
				c.extras = append(c.extras, Extra{Name: name, Size: rf.size, Need: NeedMedia, url: rf.url, row: i})
				cand.Size -= rf.size
				continue
			}
			keep, keepSizes = append(keep, name), append(keepSizes, cand.sizes[k])
			row.files = append(row.files, *rf)
		}
		cand.Files, cand.sizes = keep, keepSizes
		if rf := byName[cand.Main]; rf != nil && !rf.modified.IsZero() {
			cand.Date = rf.modified.UTC().Format("2006-01-02")
		}
		c.rows = append(c.rows, row)
	}
	c.cands = cands
	if len(c.cands) == 0 {
		return collection{}, ErrNoLinks
	}
	return c, nil
}

// pickFiles keeps the links that name a file this build can take - a
// dictionary, one of its companions, or an archive - and that the download
// policy allows, first occurrence winning for a name. A name, not a URL, is
// the identity, because the files meet again in one download folder where
// two "oxford.mdx" cannot both exist.
//
// A Google Drive file is the one link kept although its URL names no file: its
// address is an id (drive.go). The server names it in its answer to describe's
// HEAD, and build drops it if it does not. Any other link without such a name
// is not asked about - a folder page's parents, sort orders and stylesheets.
func pickFiles(f Fetcher, links []*url.URL) []*remoteFile {
	seen := map[string]bool{}
	var out []*remoteFile
	for _, u := range links {
		if len(out) >= maxLinks {
			break
		}
		if u == nil || u.Host == "" {
			continue
		}
		v := *u
		v.Fragment, v.RawFragment = "", ""
		c, err := f.Check(v.String()) // c: what is fetched, a Drive page rewritten
		if err != nil {
			continue
		}
		name, key := safeDirName(nameFromURL(c)), ""
		if takeable(name) {
			key = strings.ToLower(name)
		} else if driveID(c) != "" {
			name, key = "", "\x00"+c.String()
		} else {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, &remoteFile{name: name, url: c.String()})
	}
	return out
}

// takeable reports a name this build can take: a dictionary, one of its
// companions, or an archive.
func takeable(name string) bool {
	return name != "" && (SupportedArchive(name) || dict.ClassifyName(name) != dict.KindOther)
}

// describe asks the site about each file, headWorkers at a time, and clears
// the entries it says are not there - a list that names a file its folder no
// longer holds should not offer it.
func describe(ctx context.Context, f Fetcher, files []*remoteFile) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, headWorkers)
	for i := range files {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				files[i] = nil
				return
			}
			u, err := url.Parse(files[i].url)
			if err != nil {
				files[i] = nil
				return
			}
			h := headInfo(ctx, f, u)
			if !h.found {
				files[i] = nil
				return
			}
			files[i].size, files[i].modified = h.size, h.modified
			if files[i].name == "" && h.name != "" {
				files[i].name = safeDirName(h.name)
			}
		}(i)
	}
	wg.Wait()
}

// companionsOf finds, beside each main file a LIST named, the companions the
// list did not - the same search a single pasted link gets (probe.go), so a
// list of .mdx links installs with their .mdd as a pasted .mdx does.
func companionsOf(ctx context.Context, f Fetcher, files []*remoteFile) []*remoteFile {
	listed := make(map[string]bool, len(files))
	for _, rf := range files {
		listed[strings.ToLower(rf.name)] = true
	}
	// Read-only while the probes run; a companion two mains both find is
	// deduplicated below, in list order.
	known := func(name string) bool { return listed[strings.ToLower(name)] }
	// headWorkers at a time, as describe: a list of a hundred .mdx is a few
	// hundred requests, minutes when asked one after another on a phone.
	found := make([][]Extra, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, headWorkers)
	for i, rf := range files {
		if dict.ClassifyName(rf.name) != dict.KindMain {
			continue
		}
		u, err := url.Parse(rf.url)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(i int, u *url.URL, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() == nil {
				found[i] = probeSiblings(ctx, f, u, name, known)
			}
		}(i, u, rf.name)
	}
	wg.Wait()
	var out []*remoteFile
	for _, xs := range found {
		for _, x := range xs {
			if known(x.Name) {
				continue
			}
			listed[strings.ToLower(x.Name)] = true
			out = append(out, &remoteFile{name: x.Name, url: x.url, size: x.Size})
		}
	}
	return out
}

// isMedia reports a companion that holds images and sound: offered as a
// tickbox rather than installed unasked, because it is usually most of the
// download.
func isMedia(name, mainExt string) bool {
	lower := strings.ToLower(name)
	if mainExt == ".ifo" && lower == "res.zip" {
		return true
	}
	for _, suf := range dict.MediaSuffixes(mainExt) {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return false
}

// remoteArchive presents a collection's files to Sniff as the directory of an
// archive, so a remote folder is grouped by exactly the rule a zip is. Open
// is needed only for a name whose content decides (".md") and reads the head
// of that one file, never the whole of it.
type remoteArchive struct {
	ctx     context.Context
	f       Fetcher
	entries []Entry
	files   map[string]*remoteFile
}

func (a *remoteArchive) Entries() []Entry { return a.entries }
func (a *remoteArchive) Close() error     { return nil }

func (a *remoteArchive) Open(name string) (io.ReadCloser, error) {
	rf := a.files[name]
	if rf == nil {
		return nil, errors.New("no such file")
	}
	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, rf.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes=0-"+strconv.Itoa(sniffHeadBytes-1))
	resp, err := a.f.Client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, errors.New("the site did not answer")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, sniffHeadBytes))
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
