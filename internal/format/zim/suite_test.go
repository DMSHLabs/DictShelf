// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// openZIM's zim-testing-suite (github.com/openzim/zim-testing-suite), the files
// libzim tests against: valid archives in three layouts (nons: ZIM 6.1+
// namespaces, withns: the old A/I/- namespaces and xz clusters,
// noTitleListingV0) and ~25 copies of small.zim, each broken in one place.
// The repository has no licence, so it is not vendored: `make test-zim-suite`
// fetches it and points WUDICT_TEST_ZIM_SUITE at the checkout.
//
// The rule for every file: no panic. Beyond that, a damaged file is refused at
// open, or it opens and the damage surfaces as an error where it is read -
// never as different bytes.

// suiteCase is what one file must do. The same names recur in each layout.
type suiteCase int

const (
	valid      suiteCase = iota
	refused              // Open fails
	badBlobs             // opens; one cluster is broken - its offset table, its pointer: those blobs error, nothing reads differently
	unread               // damage outside the content (title order, checksum): content reads like small.zim
	tolerated            // opens and reads without a panic; what it returns is not specified
	notOneFile           // a later split part or an embedded copy: not a .zim on its own
	split                // the first part of a split archive: reads as the unsplit file
)

// splitWhole is the file the suite's split archive was cut from.
const splitWhole = "wikibooks_be_all_nopic_2017-02.zim"

var suiteCases = map[string]suiteCase{
	"small.zim":                                    valid,
	"wikibooks_be_all_nopic_2017-02.zim":           valid,
	"wikipedia_en_climate_change_mini_2024-06.zim": valid,

	"invalid.smaller_than_header.zim":         refused,
	"invalid.outofbounds_urlptrpos.zim":       refused,
	"invalid.outofbounds_clusterptrpos.zim":   refused,
	"invalid.invalid_mimelistpos.zim":         refused,
	"invalid.outofbounds_first_direntptr.zim": refused,
	"invalid.outofbounds_last_direntptr.zim":  refused,
	"invalid.bad_mimetype_list.zim":           refused,

	"invalid.too_small_offset_of_first_blob_in_cluster_0.zim":   badBlobs,
	"invalid.too_small_offset_of_first_blob_in_cluster_4.zim":   badBlobs,
	"invalid.too_small_offset_of_first_blob_in_cluster_7.zim":   badBlobs,
	"invalid.misaligned_offset_of_first_blob_in_cluster_9.zim":  badBlobs,
	"invalid.misaligned_offset_of_first_blob_in_cluster_10.zim": badBlobs,
	"invalid.misaligned_offset_of_first_blob_in_cluster_11.zim": badBlobs,
	"invalid.too_large_offset_of_first_blob_in_cluster.zim":     badBlobs,
	"invalid.offset_in_cluster.zim":                             badBlobs,
	// A cluster pointer past the file's end costs that cluster's entries:
	// libzim opens the file and reads the rest, as this does.
	"invalid.outofbounds_first_clusterptr.zim": badBlobs,

	"invalid.invalid_checksumpos.zim":           unread,
	"invalid.nonsorted_title_index.zim":         unread,
	"invalid.outofbounds_first_title_entry.zim": unread,
	"invalid.outofbounds_last_title_entry.zim":  unread,
	"invalid.outofbounds_titleptrpos.zim":       unread,

	// A MIME index past the list makes that one entry neither article nor
	// typed resource. The swapped first two path pointers break the sort the
	// binary search relies on; the direct backend may then miss entries, as
	// libzim does until Archive::check.
	"invalid.bad_mimetype_in_dirent.zim": tolerated,
	"invalid.nonsorted_dirent_table.zim": tolerated,

	"small.zim.embedded":                            notOneFile,
	"small.zim.embedded.multi":                      notOneFile,
	"wikibooks_be_all_nopic_2017-02_splitted.zimaa": split,
	"wikibooks_be_all_nopic_2017-02_splitted.zimab": notOneFile,
	"wikibooks_be_all_nopic_2017-02_splitted.zimac": notOneFile,
}

func suiteDir(t *testing.T) string {
	t.Helper()
	root := os.Getenv("WUDICT_TEST_ZIM_SUITE")
	if root == "" {
		t.Skip("WUDICT_TEST_ZIM_SUITE not set (make test-zim-suite)")
	}
	dir := filepath.Join(root, "data")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("WUDICT_TEST_ZIM_SUITE has no data directory: %v", err)
	}
	return dir
}

func TestZIMTestingSuite(t *testing.T) {
	dir := suiteDir(t)
	layouts, _ := filepath.Glob(filepath.Join(dir, "*"))
	seen := 0
	for _, layout := range layouts {
		if st, err := os.Stat(layout); err != nil || !st.IsDir() {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(layout, "*"))
		for _, p := range files {
			name := filepath.Base(p)
			want, ok := suiteCases[name]
			if !ok {
				if name != ".gitignore" {
					t.Errorf("%s/%s: no expectation; add it to suiteCases", filepath.Base(layout), name)
				}
				continue
			}
			seen++
			t.Run(filepath.Base(layout)+"/"+name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("PANIC: %v", r)
					}
				}()
				checkSuiteFile(t, p, want, layout)
			})
		}
	}
	if seen == 0 {
		t.Fatal("no suite files found")
	}
}

func checkSuiteFile(t *testing.T, p string, want suiteCase, layout string) {
	small := filepath.Join(layout, "small.zim")
	alloc := allocated(func() {
		d, err := Open(p)
		switch want {
		case refused, notOneFile:
			if err == nil {
				d.Close()
				t.Fatal("opened without error")
			}
			return
		}
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer d.Close()
		blobs, errs := readAll(t, d)
		readThrough(t, p, want == valid || want == split)
		switch want {
		case valid, split:
			if len(errs) > 0 {
				t.Errorf("%d unreadable entries, first: %v", len(errs), errs[0])
			}
			checkLookups(t, d)
			if want == split {
				ref := openRef(t, filepath.Join(layout, splitWhole))
				defer ref.Close()
				refBlobs, _ := readAll(t, ref)
				if len(blobs) != len(refBlobs) {
					t.Errorf("%d entries, the unsplit file has %d", len(blobs), len(refBlobs))
				}
				for k, b := range refBlobs {
					if !bytes.Equal(blobs[k], b) {
						t.Errorf("%s differs from the unsplit file", k)
					}
				}
			}
		case badBlobs, unread, tolerated:
			ref := openRef(t, small)
			defer ref.Close()
			refBlobs, _ := readAll(t, ref)
			for path, b := range blobs {
				// X/listing/titleOrdered/v0 IS the title index of a 6.1+
				// file - the header's title pointer points into it - so
				// its damage is its content; wudict never interprets it.
				if want == unread && strings.HasPrefix(path, "Xlisting/") {
					continue
				}
				if r, ok := refBlobs[path]; ok && !bytes.Equal(b, r) {
					t.Errorf("%s: %d bytes, small.zim has %d: damage read as content", path, len(b), len(r))
				}
			}
			switch want {
			case badBlobs:
				if len(errs) == 0 {
					t.Error("the broken offset table read without an error")
				}
			case unread:
				if len(errs) > 0 || len(blobs) != len(refBlobs) {
					t.Errorf("%d of %d entries read, %d errors (%v): want small.zim's reading", len(blobs), len(refBlobs), len(errs), errs)
				}
			}
		}
	})
	// The file sizes here are 40 kB to 14 MB. A bound per file is what catches
	// a decoder sized by the stream instead of by the data (64 MiB per xz
	// cluster before decodeXZ).
	if limit := uint64(64 << 20); alloc > limit && !strings.Contains(p, "wikipedia_") {
		t.Errorf("allocated %d MB, want under %d MB", alloc>>20, limit>>20)
	}
}

func openRef(t *testing.T, p string) *Dict {
	t.Helper()
	d, err := Open(p)
	if err != nil {
		t.Fatalf("reference %s: %v", p, err)
	}
	return d
}

// readAll reads every entry that holds content - every namespace, so a
// damaged blob is found wherever it is - keyed by namespace and path.
func readAll(t *testing.T, d *Dict) (map[string][]byte, []error) {
	t.Helper()
	blobs := map[string][]byte{}
	var errs []error
	w := d.c.walk()
	for i := 0; i < d.c.entries(); i++ {
		de, err := w.entry(i)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !de.isContent() {
			continue
		}
		rc, err := d.c.contentReader(de)
		if err == nil {
			var b []byte
			b, err = io.ReadAll(rc)
			rc.Close()
			if err == nil {
				blobs[de.key()] = b
			}
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	for _, name := range d.Resources() {
		rc, _, err := d.Resource(name)
		if err == nil {
			_, err = io.Copy(io.Discard, rc)
			rc.Close()
		}
		// A resource that fails the way its entry did is not counted twice.
		if err != nil && !slices.ContainsFunc(errs, func(e error) bool { return e.Error() == err.Error() }) {
			errs = append(errs, err)
		}
	}
	return blobs, errs
}

// readThrough runs the ingest Reader to its end.
func readThrough(t *testing.T, p string, strict bool) {
	t.Helper()
	r, err := NewReader(p)
	if err != nil {
		if strict {
			t.Errorf("NewReader: %v", err)
		}
		return
	}
	defer r.Close()
	n := 0
	for {
		_, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if strict {
				t.Errorf("Reader after %d entries: %v", n, err)
			}
			return
		}
		n++
	}
	if strict && n == 0 {
		t.Error("Reader yielded nothing")
	}
}

// checkLookups holds a valid file to the property lookup rests on: every
// article Keywords can list is found by Exact under its headword - by path
// when the path is a spelling of it, else through the file's title order
// (D96 Am. 1: zimwriterfs files key "Main Page" as "Main_Page.html").
//
// One exception, counted and logged: a 6.3 file's only title order is
// titleOrdered/v1, which leaves out redirects, so a redirect whose path is not
// a spelling of its title cannot be found by it - by libzim's findByTitle
// either (noTitleListingV0/wikibooks: "Main Page" -> "Галоўная старонка").
func checkLookups(t *testing.T, d *Dict) {
	t.Helper()
	w := d.c.walk()
	stride := max(1, (d.hi-d.lo)/2000)
	byTitle, unlisted := 0, 0
	for i := d.lo; i < d.hi; i += stride {
		de, err := w.entry(i)
		if err != nil || !d.c.isArticle(i, de) {
			continue
		}
		hw := de.headword()
		if !slices.Contains(pathVariants(hw), de.path) {
			if de.isRedirect() && d.titles != nil && !d.titles.ns {
				unlisted++
				continue
			}
			byTitle++
		}
		res, err := d.Exact(hw, 1)
		if err != nil {
			t.Fatalf("Exact(%q): %v", hw, err)
		}
		if len(res) == 0 {
			t.Errorf("Exact(%q) found nothing (path %q)", hw, de.path)
		}
	}
	if byTitle > 0 && d.titles == nil {
		t.Errorf("%d headwords need the title order, and none was found", byTitle)
	}
	if unlisted > 0 {
		t.Logf("%d sampled redirects are not in titleOrdered/v1, the file's only title order", unlisted)
	}
}

func allocated(fn func()) uint64 {
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	fn()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// One cluster costs its own size, not a multiple of it: zstd frames from
// libzim carry no content size, and DecodeAll without a destination allocated
// 4.6x the output before the decode buffers were reused.
func TestSuiteClusterAllocation(t *testing.T) {
	dir := suiteDir(t)
	d := openRef(t, filepath.Join(dir, "nons", "wikipedia_en_climate_change_mini_2024-06.zim"))
	defer d.Close()
	d.c.cluster(0) // warm the decoder and the decode buffer
	for i := uint32(1); i < d.c.clusterCount; i++ {
		var cd *clusterData
		var err error
		got := allocated(func() { cd, err = d.c.cluster(i) })
		if err != nil {
			t.Fatalf("cluster %d: %v", i, err)
		}
		if cd.direct {
			continue
		}
		if limit := cd.size() + cd.size()/4 + 64<<10; got > limit {
			t.Errorf("cluster %d: %d bytes decoded, %d allocated", i, cd.size(), got)
		}
	}

	// xz clusters of the old layout declare a 64 MiB dictionary.
	old := openRef(t, filepath.Join(dir, "withns", "small.zim"))
	defer old.Close()
	for i := uint32(0); i < old.c.clusterCount; i++ {
		got := allocated(func() {
			if _, err := old.c.cluster(i); err != nil {
				t.Fatalf("xz cluster %d: %v", i, err)
			}
		})
		if got > 2*xzDictCap {
			t.Errorf("xz cluster %d: %d MB allocated", i, got>>20)
		}
	}
}

// A prefix search decompresses each cluster its results come from once,
// however the results are spread over the clusters.
func TestSuitePrefixClusters(t *testing.T) {
	dir := suiteDir(t)
	d := openRef(t, filepath.Join(dir, "nons", "wikipedia_en_climate_change_mini_2024-06.zim"))
	defer d.Close()
	for _, q := range []string{"C", "Climate", "G"} {
		d.c.dropCache()
		var res []string
		clusters := map[uint32]bool{}
		var biggest uint64
		d.c.prefixScan(d.c.contentNS(), q, 20*4+64, func(i int, de dirent) bool {
			if !d.c.isArticle(i, de) {
				return true
			}
			if _, tgt, err := d.c.resolve(i); err == nil {
				clusters[tgt.cluster] = true
			}
			res = append(res, de.path)
			return len(res) < 20
		})
		for cl := range clusters {
			if cd, err := d.c.cluster(cl); err == nil {
				biggest = max(biggest, cd.size())
			}
		}
		d.c.dropCache()
		got := allocated(func() {
			if r, err := d.Prefix(q, 20); err != nil || len(r) == 0 {
				t.Fatalf("Prefix(%q) = %d results, %v", q, len(r), err)
			}
		})
		if limit := uint64(len(clusters)+1)*(biggest+biggest/4) + 4<<20; got > limit {
			t.Errorf("Prefix(%q): %d results from %d clusters of up to %d bytes allocated %d MB", q, len(res), len(clusters), biggest, got>>20)
		}
	}
}
