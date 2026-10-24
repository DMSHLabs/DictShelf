// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
)

// Lookup by title, the fallback when a word is not a path (D96 Am. 1).
//
// A path is the title with '_' for space only in files written from MediaWiki
// page names. zimwriterfs keys entries by file name ("Main_Page.html", titled
// "Main Page"), zimit by captured URL, and every one of those titles is a
// headword Keywords lists and a path lookup cannot find. The file carries its
// own title order: u32 entry indices sorted by title in raw byte order, in one
// of three places, preferred in this order:
//
//   - the header's title pointer list (before 6.3): every entry, keyed by
//     namespace and title;
//   - X/listing/titleOrdered/v0 (6.1, 6.2): the same list as a blob;
//   - X/listing/titleOrdered/v1 (6.1+, the only one in 6.3): the front articles
//     only, all in the content namespace, so its keys are titles alone. It
//     leaves out redirects - wikibooks' "Main Page" -> "Галоўная старонка" -
//     which is why it is the last resort here. libzim prefers it (FileImpl's
//     constructor), because its findByTitle lists front articles; this
//     fallback finds a headword, and a redirect is one.
//
// Every real file checked (wiktionary_ro 2026-07, the suite's Wikipedia,
// wikibooks) stores the listing in an uncompressed cluster, so a probe is
// three preads - listing slot, path pointer, dirent - and nothing is
// decompressed. A listing in a compressed cluster is read through the cluster
// cache instead.
type titleIndex struct {
	c  *container
	n  int
	ns bool // keys carry the namespace byte

	off    int64 // file offset of a stored list; -1 when it is in a compressed cluster
	cl, bl uint32

	probes keyCache // the top of the title binary search (scan)
}

// titles finds the file's title order, or nil when it has none this reader
// can use. A damaged title list is not a damaged file: lookup by path still
// works, so nothing here refuses the open.
func (c *container) titles() *titleIndex {
	if c.titlePtrPos > 0 {
		return &titleIndex{c: c, n: c.entries(), ns: true, off: c.titlePtrPos}
	}
	if t := c.listing("listing/titleOrdered/v0", true); t != nil {
		return t
	}
	return c.listing("listing/titleOrdered/v1", false)
}

func (c *container) listing(name string, ns bool) *titleIndex {
	_, de, ok := c.find('X', name)
	if !ok || !de.isContent() {
		return nil
	}
	cd, lo, hi, err := c.blobSpan(de.cluster, de.blob)
	if err != nil || (hi-lo)%4 != 0 || hi == lo {
		return nil
	}
	t := &titleIndex{c: c, n: int((hi - lo) / 4), ns: ns, off: -1, cl: de.cluster, bl: de.blob}
	if cd.direct {
		t.off = cd.body + int64(lo)
	}
	return t
}

// at is the m-th entry in title order: its index in the path list, and the
// entry itself.
func (t *titleIndex) at(m int) (int, dirent, error) {
	var slot [4]byte
	if t.off >= 0 {
		if _, err := t.c.f.ReadAt(slot[:], t.off+int64(m)*4); err != nil {
			return 0, dirent{}, err
		}
	} else {
		b, err := t.c.blob(t.cl, t.bl)
		if err != nil {
			return 0, dirent{}, err
		}
		if len(b) < (m+1)*4 {
			return 0, dirent{}, fmt.Errorf("title listing shorter than %d entries", m+1)
		}
		copy(slot[:], b[m*4:])
	}
	i := int(binary.LittleEndian.Uint32(slot[:]))
	if i >= t.c.entries() {
		return 0, dirent{}, fmt.Errorf("title listing entry %d names entry %d of %d", m, i, t.c.entries())
	}
	d, err := t.c.entry(i)
	return i, d, err
}

func (t *titleIndex) key(d dirent) string {
	if t.ns {
		return searchKey(d.namespace, d.headword())
	}
	return d.headword()
}

func (t *titleIndex) keyFor(ns byte, title string) string {
	if t.ns {
		return searchKey(ns, title)
	}
	return title
}

// scan calls fn on each entry whose title starts with prefix in namespace ns,
// in title order, until fn returns false, the prefix runs out or budget
// entries have been read. A read error ends the scan: the title order is a
// fallback, and a damaged one is a miss rather than a failed search.
func (t *titleIndex) scan(ns byte, prefix string, budget int, fn func(int, dirent) bool) {
	key := t.keyFor(ns, prefix)
	m, err := t.probes.search(t.n, key, func(m int) (string, error) {
		_, d, err := t.at(m)
		return t.key(d), err
	})
	if err != nil {
		return
	}
	for ; m < t.n && budget > 0; m, budget = m+1, budget-1 {
		i, d, err := t.at(m)
		if err != nil || !strings.HasPrefix(t.key(d), key) {
			return
		}
		if !fn(i, d) {
			return
		}
	}
}

// titleVariants are pathVariants' spellings with spaces kept, because a title
// is the page name as written: "new york" also tries "New york" and
// "New York".
func titleVariants(word string) []string {
	out := make([]string, 0, 3)
	for _, v := range []string{word, upperFirst(word), strings.ReplaceAll(titleWords(strings.ReplaceAll(word, " ", "_")), "_", " ")} {
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
