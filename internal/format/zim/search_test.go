// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import "testing"

// page is a minimal article body: text/html is what isArticle accepts, and a
// non-empty body is what results() keeps.
func page(title string) []byte {
	return []byte(`<!DOCTYPE html><html><head><title>` + title +
		`</title></head><body>` + title + `</body></html>`)
}

// A pre-6.1 file spells an article's path with the namespace as its first
// segment ("A/Foo" - the old per-kind layout), and the stored dirent path is
// "Foo" in namespace 'A'. A link written then is still read that way when it is
// followed in a later file, so a lookup that misses on "A/Foo" is retried
// without the prefix, as libzim's Archive::getEntryByPath does.
func TestExactStripsANamespacePrefixFromAPre61Path(t *testing.T) {
	d, err := Open(writeZIM(t, bFile{
		major: 5, minor: 0,
		clusters: []bCluster{{codec: 1}},
		entries: []bEntry{
			{ns: 'A', path: "Foo", title: "Foo", mime: "text/html", body: page("Foo"), cluster: 0},
			{ns: 'I', path: "pic.png", mime: "image/png", body: []byte("\x89PNG\r\n\x1a\n"), cluster: 0},
			{ns: 'M', path: "Title", mime: "text/plain", body: []byte("Old file"), cluster: 0},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if got := d.Meta().Format; got != "zim" {
		t.Fatalf("Format = %q", got)
	}
	// Both spellings reach the article: the path as the file stores it, and the
	// path as a link of that era wrote it.
	for _, q := range []string{"Foo", "A/Foo"} {
		res, err := d.Exact(q, 5)
		if err != nil {
			t.Fatalf("Exact(%q): %v", q, err)
		}
		if len(res) != 1 || res[0].Headword != "Foo" {
			t.Errorf("Exact(%q) = %+v, want the article Foo", q, res)
		}
	}
	// The retry is an EXACT path lookup, as libzim's getEntryByPath is: the
	// stripped remainder is not run through pathVariants, so a case variant of
	// it misses. Pinned here so that nobody later "fixes" it into a guess -
	// this path exists for a link the writer wrote, not for a typist.
	if res, err := d.Exact("A/foo", 5); err != nil || len(res) != 0 {
		t.Errorf("Exact(%q) = %+v, err=%v; the namespace retry is exact, so this must miss", "A/foo", res, err)
	}
}

// The retry happens only after the plain lookup missed, so an article whose
// path REALLY begins with a namespace segment is not shadowed by it: "A/Foo" as
// a literal path in a 6.1+ file is an article like any other.
func TestExactPrefersAPathThatReallyBeginsWithANamespace(t *testing.T) {
	d, err := Open(writeZIM(t, bFile{
		major: 6, minor: 3,
		clusters: []bCluster{{codec: 1}},
		entries: []bEntry{
			{ns: 'C', path: "Foo", title: "Foo", mime: "text/html", body: page("plain"), cluster: 0},
			{ns: 'C', path: "A/Foo", title: "A/Foo", mime: "text/html", body: page("prefixed"), cluster: 0},
			{ns: 'M', path: "Title", mime: "text/plain", body: []byte("New file"), cluster: 0},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	res, err := d.Exact("A/Foo", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Headword != "A/Foo" {
		t.Errorf("Exact(%q) = %+v, want the article whose path is literally A/Foo", "A/Foo", res)
	}
}
