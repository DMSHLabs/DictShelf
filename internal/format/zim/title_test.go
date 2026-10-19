// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"strings"
	"testing"
)

// A zimwriterfs-shaped file: entries keyed by file name, titled by page name.
func titledFile(minor uint16, listing, header bool) bFile {
	page := func(s string) []byte { return []byte("<html><body>" + s + "</body></html>") }
	f := bFile{major: 6, minor: minor, titleList: header,
		clusters: []bCluster{{codec: 5}, {codec: 1}}}
	f.entries = []bEntry{
		{ns: 'C', path: "index.html", title: "Main Page", mime: "text/html", body: page("main")},
		{ns: 'C', path: "a1.html", title: "Zebra crossing", mime: "text/html", body: page("crossing")},
		{ns: 'C', path: "a2.html", title: "Zebra", mime: "text/html", body: page("zebra")},
		{ns: 'C', path: "plain.html", mime: "text/html", body: page("plain")},
		{ns: 'C', path: "favicon", target: "C/favicon.png"},
		{ns: 'C', path: "favicon.png", mime: "image/png", body: []byte("\x89PNG"), cluster: 1},
		{ns: 'M', path: "Title", mime: "text/plain", body: []byte("Titled")},
	}
	if listing {
		f.entries = append(f.entries, bEntry{ns: 'X', path: "listing/titleOrdered/v1",
			mime: "application/octet-stream+zimlisting", cluster: 1, titleListing: true})
	}
	return f
}

func TestTitleFallback(t *testing.T) {
	for _, tc := range []struct {
		name            string
		minor           uint16
		listing, header bool
	}{
		{"6.3 listing v1", 3, true, false},
		{"6.2 header title list", 2, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Open(writeZIM(t, titledFile(tc.minor, tc.listing, tc.header)))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if d.titles == nil {
				t.Fatal("no title index found")
			}
			for q, want := range map[string]string{
				"Main Page":  "main",
				"main page":  "main", // the word-capitalised variant
				"Zebra":      "zebra",
				"plain.html": "plain", // a path still answers first
			} {
				res, err := d.Exact(q, 1)
				if err != nil || len(res) != 1 || !strings.Contains(res[0].Body, want) {
					t.Errorf("Exact(%q) = %v, %v; want the %q article", q, res, err, want)
				}
			}
			if res, _ := d.Exact("Main Page", 1); len(res) == 1 && res[0].Headword != "Main Page" {
				t.Errorf("headword = %q, want the title", res[0].Headword)
			}
			if res, _ := d.Exact("favicon", 1); len(res) != 0 {
				t.Errorf("Exact(favicon) = %v: a redirect to an image is not an article", res)
			}
			res, err := d.Prefix("Zeb", 10)
			var hws []string
			for _, r := range res {
				hws = append(hws, r.Headword)
			}
			if err != nil || strings.Join(hws, "|") != "Zebra|Zebra crossing" {
				t.Errorf("Prefix(Zeb) = %q, %v; want title order", hws, err)
			}
			for _, k := range d.Keywords(0, 0) {
				if k == "favicon" {
					t.Error("Keywords lists the favicon redirect")
				}
			}
		})
	}

	// Without a title order the fallback is absent, not an error.
	d, err := Open(writeZIM(t, titledFile(3, false, false)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if res, err := d.Exact("Main Page", 1); err != nil || len(res) != 0 {
		t.Errorf("Exact without a title index = %v, %v", res, err)
	}
}

// A title list that names entries past the end is a miss, never a failed
// search or a wild read.
func TestTitleIndexDamaged(t *testing.T) {
	b := buildZIM(t, titledFile(2, false, true))
	pos := int(uint64(b[40]) | uint64(b[41])<<8 | uint64(b[42])<<16 | uint64(b[43])<<24)
	for k := 0; k < 4; k++ {
		b[pos+k] = 0xFF
	}
	d, err := Open(writeTruncated(t, b))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{"Main Page", "Zebra", "A", "zzz"} {
		if _, err := d.Exact(q, 1); err != nil {
			t.Errorf("Exact(%q): %v", q, err)
		}
		if _, err := d.Prefix(q, 5); err != nil {
			t.Errorf("Prefix(%q): %v", q, err)
		}
	}
}
