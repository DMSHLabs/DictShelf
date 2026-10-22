// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

// Layouts and entries libzim reads and a reader written from the common case
// does not: each test is one way a valid file departs from the files the
// reader was first checked against.

func openBuilt(t *testing.T, f bFile) *Dict {
	t.Helper()
	d, err := Open(writeZIM(t, f))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// readEvery reads every entry that stores content, by its path, and fails on
// the first that does not read.
func readEvery(t *testing.T, d *Dict) int {
	t.Helper()
	n := 0
	w := d.c.walk()
	for i := 0; i < d.c.entries(); i++ {
		de, err := w.entry(i)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if !de.isContent() {
			continue
		}
		if _, err := d.c.content(de); err != nil {
			t.Errorf("%c/%s: %v", de.namespace, de.path, err)
		}
		n++
	}
	return n
}

// The deprecated linktarget and deleted entries have an 8-byte dirent: no
// redirect index, no cluster, no blob. Read as a blob's 16 bytes, the path
// came out garbled - so the entry was not found by its own path, and a binary
// search probing it could be sent the wrong way for any other word.
func TestMarkerEntries(t *testing.T) {
	f := sample()
	f.entries = append(f.entries,
		bEntry{ns: 'C', path: "cattle", marker: mimeLinkTarget},
		bEntry{ns: 'C', path: "deleted", title: "Gone", marker: mimeDeleted},
		bEntry{ns: 'C', path: "to_nothing", target: "C/cattle"})
	d := openBuilt(t, f)
	w := d.c.walk()
	for i := 0; i < d.c.entries(); i++ {
		de, err := w.entry(i)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if j, _, ok := d.c.find(de.namespace, de.path); !ok || j != i {
			t.Errorf("%c/%q: found at %d, %v; it is entry %d", de.namespace, de.path, j, ok, i)
		}
		if de.path == "deleted" && de.title != "Gone" {
			t.Errorf("deleted entry's title = %q", de.title)
		}
	}
	for _, w := range []string{"cat", "catalog", "New York", "kitty"} {
		if res, err := d.Exact(w, 1); err != nil || len(res) != 1 {
			t.Errorf("Exact(%q) = %v, %d results", w, err, len(res))
		}
	}
	for _, w := range []string{"cattle", "deleted", "to_nothing"} {
		if res, _ := d.Exact(w, 1); len(res) != 0 {
			t.Errorf("Exact(%q) found a marker", w)
		}
		if _, _, err := d.Resource(w); !errors.Is(err, dict.ErrNotFound) {
			t.Errorf("Resource(%q) = %v, want ErrNotFound", w, err)
		}
	}
	for _, name := range d.Resources() {
		if name == "cattle" || name == "deleted" {
			t.Errorf("Resources lists the marker %q", name)
		}
	}
}

// After the last cluster of a 6.1+ file come the dirents and the pointer
// lists, not the checksum. Bounded by the checksum, a compressed last cluster
// handed them to the decoder as a second stream, which xz and zstd refuse:
// every blob in it failed. With a gap, the extent is still not the stream's
// length - only decoding that stops at the stream's end reads the cluster.
func TestCompressedLastCluster(t *testing.T) {
	for _, codec := range []byte{2, 4, 5} {
		for _, gap := range [][]byte{nil, []byte(strings.Repeat("not a cluster\x00", 9))} {
			t.Run(fmt.Sprintf("codec %d gap %d", codec, len(gap)), func(t *testing.T) {
				f := sample()
				for i := range f.clusters {
					f.clusters[i].codec = codec
				}
				f.gap = gap
				d := openBuilt(t, f)
				rc, mime, err := d.Resource("f.png") // in the last cluster
				if err != nil {
					t.Fatal(err)
				}
				b, _ := io.ReadAll(rc)
				rc.Close()
				if string(b) != "\x89PNG\r\n\x1a\n" || mime != "image/png" {
					t.Errorf("Resource(f.png) = %q, %q", b, mime)
				}
				readEvery(t, d)
			})
		}
	}
}

// No writer leaves the cluster pointer list out of file order, but libzim
// never asks where a cluster ends, so such a file reads there.
func TestClustersOutOfFileOrder(t *testing.T) {
	for _, codec := range []byte{1, 5} {
		f := sample()
		for i := range f.clusters {
			f.clusters[i].codec = codec
		}
		f.reverseClusters = true
		d := openBuilt(t, f)
		if d.c.nextStart == nil {
			t.Fatal("pointer list taken for file order")
		}
		if n := readEvery(t, d); n == 0 {
			t.Fatal("nothing read")
		}
		if res, err := d.Exact("cat", 1); err != nil || len(res) != 1 {
			t.Errorf("codec %d: Exact(cat) = %v, %d results", codec, err, len(res))
		}
	}
}

// The first ZIM headers were 72 bytes, from before the checksum: the MIME
// list starts at 72, where the checksum position is now (libzim's
// Fileheader::hasChecksum).
func TestHeader72(t *testing.T) {
	f := sample()
	f.header72 = true
	d := openBuilt(t, f)
	if d.c.checksumPos != 0 {
		t.Errorf("checksum position %d read from the MIME list", d.c.checksumPos)
	}
	if d.Meta().Name != "Test ZIM" {
		t.Errorf("Name = %q", d.Meta().Name)
	}
	readEvery(t, d)
	if res, err := d.Exact("cat", 1); err != nil || len(res) != 1 {
		t.Errorf("Exact(cat) = %v, %d results", err, len(res))
	}
}

// A redirect chain is followed as far as libzim follows one - 50 hops, its
// first and then a watchdog of 50 counted down before each further one
// (Entry::getRedirect) - and a longer one, a cycle in practice, fails rather
// than spinning.
func TestRedirectChains(t *testing.T) {
	chain := func(hops int) bFile {
		f := sample()
		for k := 0; k < hops; k++ {
			target := fmt.Sprintf("C/hop%03d", k+1)
			if k == hops-1 {
				target = "C/cat"
			}
			f.entries = append(f.entries, bEntry{ns: 'C', path: fmt.Sprintf("hop%03d", k), target: target})
		}
		return f
	}
	if res, err := openBuilt(t, chain(50)).Exact("hop000", 1); err != nil || len(res) != 1 {
		t.Errorf("50 hops: %v, %d results", err, len(res))
	}
	if res, _ := openBuilt(t, chain(51)).Exact("hop000", 1); len(res) != 0 {
		t.Error("51 hops resolved")
	}
}

// A cluster's extent ends at the next thing in the file. After the last
// cluster that is a structure, not the file's end: here the path pointer
// list, the gap after the clusters taken in, not the lists after it.
func TestClusterExtent(t *testing.T) {
	f := sample()
	f.gap = []byte("gap")
	d := openBuilt(t, f)
	last := d.c.clusterCount - 1
	if _, end := d.c.extent(last); end != uint64(d.c.pathPtrPos) {
		t.Errorf("last cluster ends at %d, the path pointer list starts at %d", end, d.c.pathPtrPos)
	}
	if _, end := d.c.extent(0); end != d.c.clusterPtr[1] {
		t.Errorf("cluster 0 ends at %d, cluster 1 starts at %d", end, d.c.clusterPtr[1])
	}
}

// A metadata entry may be a redirect, which libzim follows
// (FileImpl::findxMetadata).
func TestMetadataRedirect(t *testing.T) {
	f := sample()
	for i, e := range f.entries {
		if e.ns == 'M' && e.path == "Title" {
			f.entries[i] = bEntry{ns: 'M', path: "Title", target: "M/Title_long"}
		}
	}
	f.entries = append(f.entries, bEntry{ns: 'M', path: "Title_long", mime: "text/plain", body: []byte("Long Title"), cluster: 0})
	if d := openBuilt(t, f); d.Meta().Name != "Long Title" {
		t.Errorf("Name = %q, want the redirect's target", d.Meta().Name)
	}
}

// An M namespace too large to hold is searched per key instead.
func TestMetadataUnheld(t *testing.T) {
	f := sample()
	for k := 0; k <= maxMetaEntries; k++ {
		f.entries = append(f.entries, bEntry{ns: 'M', path: fmt.Sprintf("X-%04d", k), mime: "text/plain", body: []byte("x"), cluster: 0})
	}
	d := openBuilt(t, f)
	if d.c.meta != nil {
		t.Fatal("held a namespace past maxMetaEntries")
	}
	if d.Meta().Name != "Test ZIM" {
		t.Errorf("Name = %q", d.Meta().Name)
	}
}

// A path that misses is looked up once more without a namespace prefix, as
// libzim's getEntryByPath does: a pre-6.1 link carried into a later file.
func TestExactNamespacePrefix(t *testing.T) {
	d := openSample(t)
	if res, err := d.Exact("C/cat", 1); err != nil || len(res) != 1 {
		t.Errorf("Exact(C/cat) = %v, %d results", err, len(res))
	}
	if res, _ := d.Exact("C/nothing", 1); len(res) != 0 {
		t.Errorf("Exact(C/nothing) found %d", len(res))
	}
}

// The probe cache answers exactly what sort.Search over the keys answers,
// cold and warm, for keys present, absent, and between.
func TestLowerBoundCached(t *testing.T) {
	f := bFile{major: 6, minor: 3, clusters: []bCluster{{codec: 1}}}
	words := map[string]bool{}
	r := rand.New(rand.NewPCG(1, 2))
	for len(words) < 5000 {
		b := make([]byte, 1+r.IntN(12))
		for i := range b {
			b[i] = byte('a' + r.IntN(26))
		}
		words[string(b)] = true
	}
	for w := range words {
		f.entries = append(f.entries, bEntry{ns: 'C', path: w, mime: "text/html", body: []byte(w), cluster: 0})
	}
	f.entries = append(f.entries, bEntry{ns: 'M', path: "Title", mime: "text/plain", body: []byte("T"), cluster: 0})
	d := openBuilt(t, f)
	var keys []string
	w := d.c.walk()
	for i := 0; i < d.c.entries(); i++ {
		de, err := w.entry(i)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, de.key())
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatal("builder wrote unsorted keys")
	}
	targets := []string{"", "\x00", "\xff", "C", "D", "M", "Ca", "Czzzzzzzzzzzzzz"}
	for _, k := range keys {
		targets = append(targets, k, k+"\x00", k[:len(k)-1])
	}
	for pass := 0; pass < 2; pass++ {
		for _, tg := range targets {
			got, err := d.c.lowerBound(tg)
			if err != nil {
				t.Fatal(err)
			}
			if want := sort.SearchStrings(keys, tg); got != want {
				t.Fatalf("pass %d: lowerBound(%q) = %d, want %d", pass, tg, got, want)
			}
		}
	}
}

// Readers of one cluster at one time share one decompression.
func TestClusterLoadShared(t *testing.T) {
	d := openSample(t)
	for round := 0; round < 20; round++ {
		d.c.dropCache()
		var wg sync.WaitGroup
		got := make([]*clusterData, 16)
		start := make(chan struct{})
		for k := range got {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				cd, err := d.c.cluster(1) // zstd
				if err != nil {
					t.Error(err)
				}
				got[k] = cd
			}()
		}
		close(start)
		wg.Wait()
		for _, cd := range got[1:] {
			if cd != got[0] {
				t.Fatal("one cluster decompressed more than once at the same time")
			}
		}
	}
}
