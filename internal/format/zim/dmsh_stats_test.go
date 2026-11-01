// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import "testing"

// The directory walk is the whole input of a size estimate, so it is checked
// against a file whose contents the test can count by hand: sample() holds
// three articles and one redirect in the content namespace, one image and one
// stylesheet as resources, and four metadata entries in the M namespace.
func TestDirectoryStats(t *testing.T) {
	b := buildZIM(t, sample())
	d := openZIMBytes(t, b)
	got, err := d.contentStats()
	if err != nil {
		t.Fatal(err)
	}
	// The M namespace entries are deliberately not counted: nothing in them is
	// text this program would index, and they share a cluster with the first
	// article.
	if got.Entries != 6 || got.Articles != 3 || got.Redirects != 1 || got.Media != 1 || got.Other != 1 {
		t.Errorf("entries=%d articles=%d redirects=%d media=%d other=%d, want 6/3/1/1/1",
			got.Entries, got.Articles, got.Redirects, got.Media, got.Other)
	}
	if got.Clusters != 3 {
		t.Errorf("clusters = %d, want 3", got.Clusters)
	}
	if got.TextBytes == 0 {
		t.Error("the two article clusters carry no bytes")
	}
	if got.MixedBytes != 0 {
		t.Errorf("no cluster in this fixture mixes articles with resources, got %d bytes", got.MixedBytes)
	}
	if got.OtherBytes == 0 {
		t.Error("the resource-only cluster carries no bytes")
	}
	if total := got.TextBytes + got.MixedBytes + got.OtherBytes; total > uint64(len(b)) {
		t.Errorf("cluster bytes %d exceed the file's %d", total, len(b))
	}
}

// A prepared database records these numbers so an estimate about a later build
// needs no second walk of an archive that may no longer be reachable.
func TestDirectoryStatsReachExtraMeta(t *testing.T) {
	path := writeZIM(t, sample())
	for _, tc := range []struct {
		name          string
		open          func(string) (*Reader, error)
		wantIndexKind string
	}{
		{"headwords", NewHeadwordReader, "headwords"},
		{"articles", NewReader, "articles"},
	} {
		r, err := tc.open(path)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		meta := r.ExtraMeta()
		r.Close()
		if meta["index_kind"] != tc.wantIndexKind {
			t.Errorf("%s: index_kind = %q, want %q", tc.name, meta["index_kind"], tc.wantIndexKind)
		}
		if meta["zim_articles"] != "3" || meta["zim_redirects"] != "1" || meta["zim_media"] != "1" {
			t.Errorf("%s: recorded articles=%q redirects=%q media=%q, want 3/1/1",
				tc.name, meta["zim_articles"], meta["zim_redirects"], meta["zim_media"])
		}
		if meta["zim_text_bytes"] == "" || meta["zim_text_bytes"] == "0" {
			t.Errorf("%s: no article-cluster bytes recorded (%q)", tc.name, meta["zim_text_bytes"])
		}
	}
}
