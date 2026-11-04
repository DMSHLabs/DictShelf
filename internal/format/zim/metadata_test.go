// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import "testing"

// An archive may keep a metadata value behind a redirect — libzim's
// FileImpl::findxMetadata resolves one — so a title stored that way must reach
// Meta().Name here as well. Measured on the three real archives in this machine's
// ZIM folder (2026-11-04): thirteen M entries each and ZERO redirects, so this
// matches the writer for a file we do not have rather than repairing one we do.
//
// The two ways a follow can fail are not covered here because the test builder
// refuses to write either: a target that does not exist (`resolve` returns an
// error, and metadata() answers "") and an entry the writer deleted (the
// `isContent` guard). Both are one line of the function, and both answer "".
func TestMetadataFollowsARedirect(t *testing.T) {
	d, err := Open(writeZIM(t, bFile{
		major: 6, minor: 3,
		clusters: []bCluster{{codec: 1}},
		entries: []bEntry{
			{ns: 'C', path: "cat", title: "cat", mime: "text/html", body: page("cat"), cluster: 0},
			{ns: 'M', path: "Title", mime: "", target: "M/RealTitle"},
			{ns: 'M', path: "RealTitle", mime: "text/plain", body: []byte("Redirected title"), cluster: 0},
			{ns: 'M', path: "Language", mime: "text/plain", body: []byte("ron"), cluster: 0},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	m := d.Meta()
	if m.Name != "Redirected title" {
		t.Errorf("Meta().Name = %q, want the value behind the M/Title redirect", m.Name)
	}
	// The follow is per entry: the rest of the namespace reads as before.
	if m.IndexLang != "ro" {
		t.Errorf("IndexLang = %q, want %q from M/Language", m.IndexLang, "ro")
	}
}
