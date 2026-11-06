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

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/logx"
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

// The share link: any address with the link after "#" -
//
//	https://dictshelf.com/share/#https://example.org/dicts/oxford.zip
//
// The fragment is the whole point. A browser never sends it to the server, so
// the page in front of it is only what a reader sees when nothing opened the
// link as a dictionary (Android handing it to the app, or the setup page after
// a paste) - and that page needs to know nothing about dictionaries at all. Any
// host can carry one, which is what makes it a format rather than an address,
// and "#/dict/x/", one leading slash, is short for a path on the sharing site
// itself. Our own page of dictionaries does not exist yet; when it does, it is
// a page like any other here.
//
// SharePages are the pages whose BARE address is such a page of links rather
// than something to read: its rows are drawn by script, so reading it as a
// folder finds nothing ("no dictionaries found" about a page full of them), and
// saying what it is instead is the whole point of listing it. Only the wudict
// project's page is listed: the links already in the wild point at it, and the
// site verifies THIS app through /.well-known/assetlinks.json, so on Android 12+
// they open DictShelf with no setting to find (android/…/AndroidManifest.xml).
var SharePages = []struct{ Host, Path string }{
	{"legbehindneck.com", "/wudict"},
}

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

// ErrSharePage is a share link with nothing after "#": the page itself, a page
// of dictionary links rather than a link to any dictionary.
var ErrSharePage = errors.New("that is a page of dictionary links, not a dictionary: open it in a browser and choose one there")

// IsSharePage reports the address of a share page with no link after "#" - a
// listed page, whose bare address is a page of links (see SharePages).
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
	return isSharePage(u)
}

// isSharePage matches a parsed address against the listed share pages.
func isSharePage(u *url.URL) bool {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	page := strings.TrimSuffix(u.Path, "/")
	for _, sp := range SharePages {
		if host == sp.Host && page == sp.Path {
			return true
		}
	}
	return false
}

// ErrNoLinks is a folder page or list that names no dictionary this build
// reads - which, for a page, is the answer the single-file path gave before
// collections existed, now said about the right thing.
var ErrNoLinks = errors.New("no dictionaries found at that link")

// Unwrap returns the link a share link carries, or s unchanged when s is not
// one. ANY http(s) address can be a share link - that is the format, and it is
// what keeps this from depending on a host we do not own - and what decides is
// the fragment:
//
//   - a link ("#https://…", or the same %-escaped whole, as some senders
//     re-encode a fragment) is the link;
//   - one leading slash ("#/dict/x/") is a path on the sharing site itself;
//   - anything else is not a share link: an ordinary URL that happens to carry
//     an anchor ("…/dict.mdx#top") is returned as it came, and the fragment is
//     not consulted again.
//
// The link inside keeps its own %-escapes: only a fragment escaped as a whole
// is decoded.
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
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https", "http":
	default:
		return s
	}
	inner := strings.TrimSpace(s[i+1:])
	if l := strings.ToLower(inner); strings.HasPrefix(l, "https%3a") || strings.HasPrefix(l, "http%3a") {
		if d, err := url.QueryUnescape(inner); err == nil {
			inner = d
		}
	}
	if l := strings.ToLower(inner); strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") {
		return inner
	}
	// The short form: "#/dict/euskera/" is a path on the share link's own site.
	// One leading slash only - "//other.org/x" is a link to another host
	// written without its scheme, and the short form must never be a way to
	// point somewhere the long form would have had to name.
	if strings.HasPrefix(inner, "/") && !strings.HasPrefix(inner, "//") {
		return scheme + "://" + u.Host + inner
	}
	return s
}

// UnwrapAll is Unwrap for a share link that carries several links:
//
//	https://dictshelf.com/share/#https://a.org/x.mdx,https://b.org/x.mdd
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
	text  bool
	files []*remoteFile // already resolved nodes, used by MEGA public folders
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
	if l, ok, err := f.megaFolderListing(ctx, u); ok {
		return l, err
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
	cands       []Candidate
	extras      []Extra
	rows        []colRow
	folderMode  bool
	folderFiles []FolderFile
}

// build describes the files a listing names, without downloading any of
// them: which exist and how big they are (HEAD), how they group into
// dictionaries (Sniff, over remoteArchive), and which are already installed
// (markExisting, from names and sizes).
func build(ctx context.Context, f Fetcher, dest string, lib []string, l listing) (collection, error) {
	return buildWithProgress(ctx, f, dest, lib, l, nil)
}

func buildWithProgress(ctx context.Context, f Fetcher, dest string, lib []string, l listing, progress func(string)) (collection, error) {
	files := l.files
	if files == nil {
		files = pickFiles(f, l.links)
	}
	if len(files) == 0 {
		logx.Download("collection inspect stopped reason=no-files links=%d", len(l.links))
		return collection{}, ErrNoLinks
	}
	logx.Download("collection inspect start files=%d folder_mode=%t text_list=%t", len(files), l.files != nil, l.text)
	drive := false
	for _, rf := range files {
		if u, err := url.Parse(rf.url); err == nil && driveID(u) != "" {
			drive = true
		}
	}
	if l.files == nil {
		describe(ctx, f, files)
	}
	// Only now is every name known: a Drive file was named by the server's
	// answer, or by nothing - and then Drive sent a page, not the file. Two links can turn out to be one name; the first stays.
	var alive []*remoteFile
	named := map[string]bool{}
	for _, rf := range files {
		if rf == nil || (l.files == nil && !takeable(rf.name)) || rf.name == "" || named[strings.ToLower(rf.name)] {
			continue
		}
		named[strings.ToLower(rf.name)] = true
		alive = append(alive, rf)
	}
	logx.Download("collection file selection files=%d usable_names=%d", len(files), len(alive))
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
	logx.Download("collection dictionary inspection entries=%d standalone_archives=%d", len(entries), len(archives))
	sniffStarted := time.Now()
	logx.Download("inspect folder dictionaries start entries=%d", len(entries))
	cands, err := Sniff(&remoteArchive{ctx: ctx, f: f, entries: entries, files: byName, progress: progress})
	logx.Download("inspect folder dictionaries finished entries=%d candidates=%d elapsed=%s error=%v", len(entries), len(cands), time.Since(sniffStarted).Round(time.Millisecond), err)
	if err != nil {
		logx.Download("collection dictionary inspection aborted error=%v", err)
		return collection{}, err
	}
	// Marked BEFORE the media come out of Files: "unchanged" is a claim about
	// every file of the dictionary, and an .mdd that grew is a change.
	markStarted := time.Now()
	logx.Download("compare installed dictionaries start candidates=%d library_entries=%d", len(cands), len(lib))
	markExisting(dest, lib, cands)
	logx.Download("compare installed dictionaries finished candidates=%d elapsed=%s", len(cands), time.Since(markStarted).Round(time.Millisecond))
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
	for i, c := range cands {
		logx.Download("collection candidate index=%d name=%q format=%q files=%q missing=%q complete=%t existing=%t unchanged=%t size=%d", i, c.Name, c.Format, c.Files, c.Missing, c.Complete(), c.Existing != "", c.Unchanged, c.Size)
	}

	var c collection
	c.folderMode = l.files != nil
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
	// A MEGA folder has no sibling probe: its direct-child listing is the
	// complete set. Offer same-stem files the format contract does not consume
	// as optional companions so selecting a dictionary selects its whole set.
	if l.files != nil {
		known := map[string]bool{}
		for _, cand := range cands {
			for _, name := range cand.Files {
				known[strings.ToLower(name)] = true
			}
		}
		for _, ex := range c.extras {
			known[strings.ToLower(ex.Name)] = true
		}
		for i, cand := range cands {
			stem := strings.ToLower(dict.Stem(cand.Main))
			for _, rf := range alive {
				key := strings.ToLower(rf.name)
				if known[key] || strings.ToLower(dict.Stem(rf.name)) != stem {
					continue
				}
				known[key] = true
				need := NeedExtra
				if dictionaryIconName(rf.name) {
					need = NeedIcon
				}
				c.extras = append(c.extras, Extra{Name: rf.name, Size: rf.size, Need: need, url: rf.url, row: i})
			}
		}
	}
	c.cands = cands
	logx.Download("collection companions mapped candidates=%d extras=%d", len(c.cands), len(c.extras))
	if c.folderMode {
		groups := map[string]int{}
		usable := map[int]bool{}
		for i, cand := range c.cands {
			usable[i] = cand.Complete()
			for _, n := range cand.Files {
				groups[strings.ToLower(n)] = i
			}
		}
		extraGroups := map[string]int{}
		for _, ex := range c.extras {
			extraGroups[strings.ToLower(ex.Name)] = ex.row
		}
		for i, ex := range c.extras {
			logx.Download("collection extra index=%d name=%q need=%s size=%d row=%d", i, ex.Name, ex.Need, ex.Size, ex.row)
		}
		for _, rf := range l.files {
			row := FolderFile{Name: rf.name, Size: rf.size, Group: -1, Extra: -1}
			if g, ok := groups[strings.ToLower(rf.name)]; ok {
				row.Group = g
				row.Selectable = usable[g]
				row.Main = strings.EqualFold(rf.name, c.cands[g].Main)
			} else if g, ok := extraGroups[strings.ToLower(rf.name)]; ok {
				row.Group = g
				row.Extra = extraIndex(c.extras, rf.name)
				row.Selectable = usable[g]
			}
			c.folderFiles = append(c.folderFiles, row)
			logx.Download("mega folder selection row name=%q size=%d group=%d extra=%d selectable=%t", row.Name, row.Size, row.Group, row.Extra, row.Selectable)
		}
		mapped, selectable := 0, 0
		for _, row := range c.folderFiles {
			if row.Group >= 0 || row.Extra >= 0 {
				mapped++
			}
			if row.Selectable {
				selectable++
			}
		}
		logx.Download("mega folder selection model rows=%d mapped=%d selectable=%d candidates=%d extras=%d", len(c.folderFiles), mapped, selectable, len(c.cands), len(c.extras))
	}
	if len(c.cands) == 0 {
		logx.Download("collection inspect finished without candidates files=%d", len(files))
		return collection{}, ErrNoLinks
	}
	logx.Download("collection inspect complete candidates=%d extras=%d folder_files=%d", len(c.cands), len(c.extras), len(c.folderFiles))
	return c, nil
}

func extraIndex(extras []Extra, name string) int {
	for i, e := range extras {
		if strings.EqualFold(e.Name, name) {
			return i
		}
	}
	return -1
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
	ctx      context.Context
	f        Fetcher
	entries  []Entry
	files    map[string]*remoteFile
	progress func(string)
}

func (a *remoteArchive) Entries() []Entry { return a.entries }
func (a *remoteArchive) Close() error     { return nil }

func (a *remoteArchive) Open(name string) (io.ReadCloser, error) {
	rf := a.files[name]
	if rf == nil {
		return nil, errors.New("no such file")
	}
	if a.progress != nil {
		a.progress(rf.name)
	}
	ctx, cancel := context.WithTimeout(a.ctx, megaRequestTimeout)
	defer cancel()
	started := time.Now()
	logx.Download("inspect remote file start name=%q size=%d", rf.name, rf.size)
	u, err := url.Parse(rf.url)
	if err != nil {
		logx.Download("inspect remote file failed name=%q stage=parse error=%v", rf.name, err)
		return nil, err
	}
	var mf *megaFile
	if id, key, ok := megaLink(u); ok {
		mf, err = a.f.megaResolveContext(ctx, id, key, u.Query().Get("folder"))
		if err != nil {
			logx.Download("inspect remote file failed name=%q stage=resolve elapsed=%s error=%v", rf.name, time.Since(started).Round(time.Millisecond), err)
			return nil, err
		}
	}
	target := rf.url
	if mf != nil {
		target = mf.url.String()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes=0-"+strconv.Itoa(sniffHeadBytes-1))
	logx.Download("inspect remote range request name=%q host=%s range_bytes=%d", rf.name, req.URL.Hostname(), sniffHeadBytes)
	client := a.f.Client()
	if mf != nil {
		client = a.f.megaFetcher().Client()
	}
	resp, err := client.Do(req)
	if err != nil {
		logx.Download("inspect remote file failed name=%q stage=request elapsed=%s error=%v", rf.name, time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	defer resp.Body.Close()
	logx.Download("inspect remote range response name=%q status=%d content_length=%d content_range=%q", rf.name, resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Range"))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		logx.Download("inspect remote file failed name=%q stage=response status=%d elapsed=%s", rf.name, resp.StatusCode, time.Since(started).Round(time.Millisecond))
		return nil, errors.New("the site did not answer")
	}
	var body io.Reader = resp.Body
	if mf != nil {
		body = mf.reader(resp.Body, 0)
	}
	b, err := io.ReadAll(io.LimitReader(body, sniffHeadBytes))
	if err != nil {
		logx.Download("inspect remote file failed name=%q stage=read elapsed=%s error=%v", rf.name, time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	logx.Download("inspect remote file finished name=%q bytes=%d elapsed=%s", rf.name, len(b), time.Since(started).Round(time.Millisecond))
	return io.NopCloser(bytes.NewReader(b)), nil
}
