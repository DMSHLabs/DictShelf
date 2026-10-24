// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

func openSample(t *testing.T) *Dict {
	t.Helper()
	d, err := Open(writeZIM(t, sample()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMeta(t *testing.T) {
	d := openSample(t)
	m := d.Meta()
	if m.Name != "Test ZIM" {
		t.Errorf("Name = %q, want %q", m.Name, "Test ZIM")
	}
	if m.Format != "zim" {
		t.Errorf("Format = %q", m.Format)
	}
	if m.Description != "A test file" {
		t.Errorf("Description = %q", m.Description)
	}
	if m.IndexLang != "ro" {
		t.Errorf("IndexLang = %q, want ro", m.IndexLang)
	}
	// Every C/ entry, resources included - the same range Keywords browses,
	// so a count and a browse can never disagree.
	if m.EntryCount != 6 {
		t.Errorf("EntryCount = %d, want 6", m.EntryCount)
	}
	if got := d.c.articleCount(); got != 3 {
		t.Errorf("M/Counter articles = %d, want 3", got)
	}
	if c := d.c.contentNS(); c != 'C' {
		t.Errorf("contentNS = %q, want C", c)
	}
	if !d.SelfIndexed() {
		t.Error("SelfIndexed = false")
	}
	if w := d.PreviewBytes(); w <= clusterCacheBytes {
		t.Errorf("PreviewBytes = %d, want > cache size", w)
	}
}

func TestExact(t *testing.T) {
	d := openSample(t)
	cases := []struct {
		name     string
		word     string
		headword string
		wantBody string // substring, "" = expect no hit
	}{
		{"plain", "cat", "cat", "odrasl"},
		// The path spells a space as '_' and MediaWiki upper-cases the first
		// letter; neither costs a resident index, only another binary search.
		{"underscored", "New York", "New_York", "big"},
		{"lowercased", "new york", "New_York", "big"},
		{"redirect", "kitty", "kitty", "odrasl"},
		{"whitespace", "  cat  ", "cat", "odrasl"},
		{"missing", "nothing here", "", ""},
		{"empty", "", "", ""},
		// A stored blob that is not an article is never a search hit.
		{"resource", "f.png", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := d.Exact(tc.word, 10)
			if err != nil {
				t.Fatal(err)
			}
			if tc.headword == "" {
				if len(res) != 0 {
					t.Fatalf("got %d results, want none", len(res))
				}
				return
			}
			if len(res) != 1 {
				t.Fatalf("got %d results, want 1", len(res))
			}
			if res[0].Headword != tc.headword {
				t.Errorf("Headword = %q, want %q", res[0].Headword, tc.headword)
			}
			if tc.wantBody != "" && !strings.Contains(res[0].Body, tc.wantBody) {
				t.Errorf("Body = %q, want to contain %q", res[0].Body, tc.wantBody)
			}
		})
	}
}

func TestExactBodyIsRewritten(t *testing.T) {
	d := openSample(t)
	res, err := d.Exact("cat", 1)
	if err != nil || len(res) != 1 {
		t.Fatalf("Exact: %v, %d results", err, len(res))
	}
	body := res[0].Body
	if strings.Contains(body, "<html") || strings.Contains(body, "DOCTYPE") {
		t.Errorf("document shell survived: %q", body)
	}
	// The head's stylesheet is hoisted, resolved from the archive's root; its
	// icon link is not.
	if !strings.Contains(body, `href="_res_/a.css"`) {
		t.Errorf("stylesheet not hoisted: %q", body)
	}
	if strings.Contains(body, "f.png\"") && strings.Contains(body, `rel="icon"`) {
		t.Errorf("icon link hoisted: %q", body)
	}
	// The cross-reference becomes a lookup, percent-decoded.
	if !strings.Contains(body, `href="entry://odraslă"`) {
		t.Errorf("anchor not rewritten: %q", body)
	}
	// The image reference stays a resource reference, for the server to map.
	if !strings.Contains(body, `src="f.png"`) {
		t.Errorf("img src touched: %q", body)
	}
}

func TestPrefix(t *testing.T) {
	d := openSample(t)
	// An exact hit comes first but does not stand alone: "starts with" has to
	// show the siblings under the word, as every other backend now does.
	res, err := d.Prefix("cat", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Headword != "cat" || res[1].Headword != "catalog" {
		t.Fatalf("Prefix(cat) = %+v", res)
	}
	// Scan forward in byte order. "_res_/a.css" sorts inside this range and
	// must not be returned.
	res, err = d.Prefix("ca", 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range res {
		got = append(got, r.Headword)
	}
	if len(got) != 2 || got[0] != "cat" || got[1] != "catalog" {
		t.Fatalf("Prefix(ca) = %v, want [cat catalog]", got)
	}
	if res, err = d.Prefix("ca", 1); err != nil || len(res) != 1 {
		t.Fatalf("Prefix limit ignored: %v, %d results", err, len(res))
	}
	if res, err = d.Prefix("zzz", 10); err != nil || len(res) != 0 {
		t.Fatalf("Prefix(zzz) = %v, %v", res, err)
	}
}

func TestKeywords(t *testing.T) {
	d := openSample(t)
	all := d.Keywords(0, 0)
	// Path byte order: 'N' < '_' < 'c' < 'f' < 'k'. Only articles and
	// redirects are headwords; the css and the png are not.
	want := []string{"New_York", "cat", "catalog", "kitty"}
	if strings.Join(all, "|") != strings.Join(want, "|") {
		t.Fatalf("Keywords = %v, want %v", all, want)
	}
	if got := d.Keywords(1, 2); len(got) > 2 {
		t.Errorf("Keywords(1,2) = %v", got)
	}
	if got := d.Keywords(-5, 3); len(got) == 0 {
		t.Error("negative offset returned nothing")
	}
	if got := d.Keywords(9999, 10); got != nil {
		t.Errorf("Keywords past the end = %v, want nil", got)
	}
}

func TestResources(t *testing.T) {
	d := openSample(t)
	// Both live in the extended (8-byte offset) cluster, so reading them
	// also exercises that width.
	rc, mime, err := d.Resource("f.png")
	if err != nil {
		t.Fatal(err)
	}
	// Seekable, or the server cannot answer a Range request for it.
	if _, ok := rc.(io.ReadSeeker); !ok {
		t.Errorf("Resource(f.png) is a %T, not an io.ReadSeeker", rc)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "\x89PNG\r\n\x1a\n" || mime != "image/png" {
		t.Errorf("Resource(f.png) = %q, %q", b, mime)
	}
	// The relative form an article writes must resolve to the same blob.
	rc, mime, err = d.Resource("./_res_/a.css")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(rc)
	rc.Close()
	if string(b) != "b{color:red}" || mime != "text/css" {
		t.Errorf("Resource(a.css) = %q, %q", b, mime)
	}
	if _, _, err := d.Resource("absent.png"); !errors.Is(err, dict.ErrNotFound) {
		t.Errorf("missing resource err = %v, want ErrNotFound", err)
	}
	if _, _, err := d.Resource(""); !errors.Is(err, dict.ErrNotFound) {
		t.Errorf("empty resource err = %v, want ErrNotFound", err)
	}
	list := d.Resources()
	if strings.Join(list, "|") != "_res_/a.css|f.png" {
		t.Errorf("Resources = %v", list)
	}
}

func TestReader(t *testing.T) {
	r, err := NewReader(writeZIM(t, sample()))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Meta().Name != "Test ZIM" {
		t.Errorf("Meta not available before Next: %+v", r.Meta())
	}
	var heads []string
	links := map[string]string{}
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(e.Headwords) != 1 {
			t.Fatalf("headwords = %v", e.Headwords)
		}
		heads = append(heads, e.Headwords[0])
		if e.LinkTo != "" {
			links[e.Headwords[0]] = e.LinkTo
			continue
		}
		if e.Kind != dict.BodyHTML || e.Body == "" {
			t.Errorf("%q: Kind=%v body=%q", e.Headwords[0], e.Kind, e.Body)
		}
	}
	// Articles in cluster order (cat is in cluster 0), redirects last.
	want := []string{"cat", "New_York", "catalog", "kitty"}
	if strings.Join(heads, "|") != strings.Join(want, "|") {
		t.Fatalf("Next order = %v, want %v", heads, want)
	}
	if links["kitty"] != "cat" {
		t.Errorf("redirect target = %q, want cat", links["kitty"])
	}
	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next after EOF = %v", err)
	}
}

// Every codec the format defines and this builder can write. The corpus only
// contains 1 and 5, but a file from an older writer must still open.
func TestCodecs(t *testing.T) {
	for _, codec := range []byte{0, 1, 2, 4, 5} {
		f := sample()
		for i := range f.clusters {
			f.clusters[i].codec = codec
		}
		d, err := Open(writeZIM(t, f))
		if err != nil {
			t.Fatalf("codec %d: %v", codec, err)
		}
		res, err := d.Exact("catalog", 1)
		if err != nil || len(res) != 1 || !strings.Contains(res[0].Body, "list") {
			t.Errorf("codec %d: %v, %+v", codec, err, res)
		}
		d.Close()
	}
}

// A pre-6.1 file: articles live in A/, resources in I/, and cross-references
// carry the namespace segment.
func TestOldNamespaces(t *testing.T) {
	f := bFile{
		major: 6, minor: 0,
		clusters: []bCluster{{codec: 1}},
		entries: []bEntry{
			{ns: 'A', path: "dog", title: "dog", mime: "text/html",
				body: []byte(`<html><body><a href="../A/wolf">w</a><a href="../I/p.png">p</a></body></html>`)},
			{ns: 'A', path: "wolf", mime: "text/html", body: []byte("<html><body>howl</body></html>")},
			{ns: 'I', path: "p.png", mime: "image/png", body: []byte("PNG")},
			{ns: 'M', path: "Title", mime: "text/plain", body: []byte("Old")},
		},
	}
	d, err := Open(writeZIM(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.c.contentNS() != 'A' || d.c.newNamespaces() {
		t.Fatalf("contentNS = %q, newNamespaces = %v", d.c.contentNS(), d.c.newNamespaces())
	}
	res, err := d.Exact("dog", 1)
	if err != nil || len(res) != 1 {
		t.Fatalf("Exact(dog) = %v, %+v", err, res)
	}
	if !strings.Contains(res[0].Body, `href="entry://wolf"`) {
		t.Errorf("A/ reference not rewritten: %q", res[0].Body)
	}
	if !strings.Contains(res[0].Body, `href="I/p.png"`) {
		t.Errorf("I/ reference not resolved to the stored file: %q", res[0].Body)
	}
	// The image namespace is only reachable on old-scheme files.
	if _, _, err := d.Resource("p.png"); err != nil {
		t.Errorf("Resource(p.png): %v", err)
	}
	// And this is the spelling the article above actually carries, so it is
	// the one the server asks for: the namespace segment survives the
	// relative trim, while the dirent stores the path without it.
	for _, name := range []string{"I/p.png", "../I/p.png", "./I/p.png"} {
		if _, _, err := d.Resource(name); err != nil {
			t.Errorf("Resource(%q): %v", name, err)
		}
	}
	// A prefix that names no namespace we search is part of the path, not a
	// namespace, and must not be eaten.
	if _, _, err := d.Resource("Z/p.png"); err == nil {
		t.Errorf("Resource(Z/p.png) resolved")
	}
	if list := d.Resources(); strings.Join(list, "|") != "I/p.png" {
		t.Errorf("Resources = %v", list)
	}
}

// An entry that is not a type/subtype (bitcoin_en_all_maxi_2021-03.zim lists
// "undefined" for its images) opens, declares no type, and leaves the indices
// after it in place: in the sorted sample the two take slots 1 and 2, ahead of
// text/html.
func TestUntypedMIME(t *testing.T) {
	f := sample()
	f.entries = append(f.entries,
		bEntry{ns: 'C', path: "blob", mime: "undefined", body: []byte("raw"), cluster: 0},
		bEntry{ns: 'C', path: "blob2", mime: "not a type\r\n", body: []byte("raw2"), cluster: 0})
	d, err := Open(writeZIM(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for name, want := range map[string]string{"blob": "raw", "blob2": "raw2"} {
		rc, mime, err := d.Resource(name)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if string(b) != want || mime != "" {
			t.Errorf("Resource(%s) = %q, %q", name, b, mime)
		}
	}
	if res, err := d.Exact("cat", 1); err != nil || len(res) != 1 {
		t.Errorf("Exact(cat) = %v, %+v", err, res)
	}
	if list := d.Resources(); strings.Join(list, "|") != "_res_/a.css|blob|blob2|f.png" {
		t.Errorf("Resources = %v", list)
	}
}

type memSource struct{ *bytes.Reader }

func (memSource) Close() error { return nil }

// The list is judged by structure alone, as libzim judges it: terminated by an
// empty string inside its bound, whatever the text of the entries.
func TestParseMIMEList(t *testing.T) {
	many := bytes.Repeat([]byte("a/b\x00"), mimeDeleted+1)
	most := slices.Repeat([]string{"a/b"}, mimeDeleted)
	for _, tc := range []struct {
		name  string
		list  string
		bound int // bytes of list inside the bound; -1 = all
		want  []string
		html  []uint16
	}{
		{"typed", "text/html; charset=UTF-8\x00image/png\x00\x00", -1, []string{"text/html; charset=UTF-8", "image/png"}, []uint16{0}},
		{"untyped keeps its slot", "undefined\x00\x01\x7f\xff\x00text/html\x00\x00", -1, []string{"", "", "text/html"}, []uint16{2}},
		{"terminator past the bound", "text/html\x00\x00", 10, nil, nil},
		{"no NUL in the bound", "This is a bad mimetype value\x00\x00", 20, nil, nil},
		{"empty", "\x00", -1, nil, nil},
		{"zero-length region", "\x00", 0, nil, nil},
		{"too many entries", string(many) + "\x00", -1, nil, nil},
		{"every index below the markers", string(many[4:]) + "\x00", -1, most, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bound := len(tc.list)
			if tc.bound >= 0 {
				bound = tc.bound
			}
			b := append(make([]byte, headerSize), tc.list...)
			c := &container{f: memSource{bytes.NewReader(b)}, size: int64(len(b))}
			err := c.parseMIMEList(headerSize, uint64(headerSize+bound))
			if tc.want == nil {
				if err == nil {
					t.Errorf("accepted: %q", c.mimes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(c.mimes, tc.want) {
				t.Errorf("mimes = %q, want %q", c.mimes, tc.want)
			}
			var html []uint16
			for i := range c.mimes {
				if c.htmlMIME[uint16(i)] {
					html = append(html, uint16(i))
				}
			}
			if !slices.Equal(html, tc.html) {
				t.Errorf("html = %v, want %v", html, tc.html)
			}
		})
	}
}

// zim-testing-suite's invalid.bad_mimetype_list, rebuilt: the list region
// overwritten up to the first structure with no NUL in it. Read up to the path
// pointer list instead, it would split at the NULs of the dirents behind it.
func TestOverwrittenMIMEListRefused(t *testing.T) {
	b := buildZIM(t, sample())
	end := headerSize + bytes.Index(b[headerSize:], []byte{0, 0}) + 2
	for i := headerSize; i < end; i++ {
		b[i] = 'x'
	}
	if d, err := Open(writeTruncated(t, b)); err == nil {
		d.Close()
		t.Fatal("opened with an overwritten MIME list")
	}
}

// Hostile input: every prefix of a valid file must be rejected without a
// panic, a hang, or a wild read.
func TestTruncatedRejected(t *testing.T) {
	full := buildZIM(t, sample())
	for _, n := range []int{0, 4, 40, 79, 80, 200, len(full) / 2, len(full) - 40} {
		p := writeTruncated(t, full[:n])
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%d bytes: PANIC %v", n, r)
				}
			}()
			d, err := Open(p)
			if err == nil {
				d.Close()
				t.Errorf("%d bytes: opened without error", n)
			}
		}()
	}
}

func TestGarbageRejected(t *testing.T) {
	for _, b := range [][]byte{
		[]byte("not a zim file at all"),
		make([]byte, headerSize), // right size, wrong magic
	} {
		if d, err := Open(writeTruncated(t, b)); err == nil {
			d.Close()
			t.Errorf("%q opened without error", b[:min(len(b), 8)])
		}
	}
}

func writeTruncated(t *testing.T, b []byte) string {
	t.Helper()
	p := t.TempDir() + "/x.zim"
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Integration against a real ZIM; skips unless WUDICT_TEST_ZIM is set.
func TestRealFile(t *testing.T) {
	p := os.Getenv("WUDICT_TEST_ZIM")
	if p == "" {
		t.Skip("WUDICT_TEST_ZIM not set")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("WUDICT_TEST_ZIM unreadable: %v", err)
	}
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	m := d.Meta()
	t.Logf("%s: %d entries, lang %q, %d articles claimed, %d MiB preview",
		m.Name, m.EntryCount, m.IndexLang, d.c.articleCount(), d.PreviewBytes()>>20)
	if m.Name == "" || m.EntryCount == 0 {
		t.Fatalf("meta looks empty: %+v", m)
	}
	kws := d.Keywords(m.EntryCount/2, 20)
	if len(kws) == 0 {
		t.Fatal("Keywords returned nothing mid-file")
	}
	// Every headword the file offers must be findable by the same lookup a
	// user's query takes - the one property the whole binary-search design
	// rests on.
	for _, k := range kws {
		res, err := d.Exact(k, 1)
		if err != nil {
			t.Fatalf("Exact(%q): %v", k, err)
		}
		if len(res) == 0 {
			t.Errorf("Exact(%q) found nothing", k)
		}
	}
	if res, err := d.Prefix(kws[0][:1], 5); err != nil || len(res) == 0 {
		t.Errorf("Prefix(%q) = %d results, %v", kws[0][:1], len(res), err)
	}
}

// More entries than a page of path pointers holds: every lookup, the walks
// across page boundaries, and lookups from many goroutines at once, which
// share no pointer state.
func TestManyEntries(t *testing.T) {
	const n = 3*ptrPageLen + 7
	// a stored cluster last, as in sample: the builder writes the pointer
	// lists after the clusters, inside the last one's extent
	f := bFile{major: 6, minor: 3, clusters: []bCluster{{codec: 5}, {codec: 1}}}
	for i := range n {
		w := fmt.Sprintf("w%05d", i)
		f.entries = append(f.entries, bEntry{ns: 'C', path: w, mime: "text/html",
			body: []byte("<html><body>" + w + "</body></html>"), cluster: i % 2})
	}
	f.entries = append(f.entries, bEntry{ns: 'M', path: "Title", mime: "text/plain", body: []byte("Many"), cluster: 0})
	d, err := Open(writeZIM(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	kw := d.Keywords(0, 0)
	if len(kw) != n || kw[0] != "w00000" || kw[n-1] != fmt.Sprintf("w%05d", n-1) {
		t.Fatalf("Keywords: %d, first %q last %q", len(kw), kw[0], kw[len(kw)-1])
	}
	if got := d.Keywords(ptrPageLen-2, 4); strings.Join(got, " ") !=
		fmt.Sprintf("w%05d w%05d w%05d w%05d", ptrPageLen-2, ptrPageLen-1, ptrPageLen, ptrPageLen+1) {
		t.Errorf("Keywords across a page boundary = %q", got)
	}
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := g; i < n; i += 8 {
				w := fmt.Sprintf("w%05d", i)
				res, err := d.Exact(w, 1)
				if err != nil || len(res) != 1 || !strings.Contains(res[0].Body, w) {
					errs <- fmt.Sprintf("Exact(%s) = %v, %v", w, res, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	res, err := d.Prefix(fmt.Sprintf("w%04d", (ptrPageLen+3)/10), 20)
	if err != nil || len(res) != 10 {
		t.Errorf("Prefix across entries = %d results, %v", len(res), err)
	}
}
