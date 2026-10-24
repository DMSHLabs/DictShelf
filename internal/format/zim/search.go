// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"strings"
	"sync/atomic"
)

// Lookup over the path pointer list, which is sorted in raw BYTE order over
// (namespace, path). That is what makes this a binary search over the file
// instead of over an in-memory map, and it is why the ban in docs/SPEC.md §6c
// on "binary-search / collation work" in a direct backend does not apply: that
// rule exists to stop a backend GUESSING a collation and returning silently
// wrong results. Byte order is not a guess - it is exact, it is what the file
// is written with, and it is verified against every entry of a real
// wiktionary.

// probeLevels is how many of the top levels of a binary search over the file
// keep the keys they compared against. Every search starts at the same entry,
// the middle one, then goes on to one of the same two, one of the same four:
// the top of the tree is the same few entries for every word, read again on
// every lookup. Kept, they cost no read - 11 of the ~20 levels of a
// million-entry file, 2047 keys - and the rest of a search lands in a narrow
// stretch of the file.
const probeLevels = 11

// keyCache holds the keys of the top probeLevels levels of the binary search
// over one sorted list, filled in as searches reach them. A slot is a node of
// the search tree, numbered breadth-first from 1, and a node identifies the
// entry it probes: the midpoint depends only on the range, and the range only
// on the path taken to it.
type keyCache [1 << probeLevels]atomic.Pointer[string]

// search is sort.Search over n ascending keys - the first m whose key is >=
// target, or n - probing in the same order; key reads the m-th key, and an
// error from it ends the search.
func (kc *keyCache) search(n int, target string, key func(m int) (string, error)) (int, error) {
	lo, hi, node := 0, n, 1
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		var k string
		if p := kc.at(node); p != nil {
			k = *p
		} else {
			s, err := key(m)
			if err != nil {
				return 0, err
			}
			k = s
			if node < len(kc) {
				kc[node].Store(&s)
			}
		}
		if k >= target {
			hi, node = m, node<<1
		} else {
			lo, node = m+1, node<<1|1
		}
	}
	return lo, nil
}

func (kc *keyCache) at(node int) *string {
	if node < len(kc) {
		return kc[node].Load()
	}
	return nil
}

// lowerBound returns the first entry index whose key is >= key, or entryCount
// when none is. ~20 probes on a million-entry file, two preads each - the
// pointer, then the dirent - of which the top levels are held (keyCache).
func (c *container) lowerBound(key string) (int, error) {
	return c.probes.search(c.entries(), key, func(m int) (string, error) {
		d, err := c.entry(m)
		return d.key(), err
	})
}

// find locates an exact (namespace, path) entry.
func (c *container) find(ns byte, path string) (int, dirent, bool) {
	key := searchKey(ns, path)
	i, err := c.lowerBound(key)
	if err != nil || i >= c.entries() {
		return 0, dirent{}, false
	}
	d, err := c.entry(i)
	if err != nil || d.key() != key {
		return 0, dirent{}, false
	}
	return i, d, true
}

// nsRange returns the half-open entry range of one namespace. Two binary
// searches, no scan: the namespace is the first byte of every key, so its
// entries are contiguous.
func (c *container) nsRange(ns byte) (int, int) {
	lo, err := c.lowerBound(string([]byte{ns}))
	if err != nil {
		return 0, 0
	}
	if ns == 0xFF { // no successor byte to bound with
		return lo, c.entries()
	}
	hi, err := c.lowerBound(string([]byte{ns + 1}))
	if err != nil || hi < lo {
		return lo, lo
	}
	return lo, hi
}

// prefixScan walks entries in a namespace whose path starts with prefix,
// calling fn until it returns false or the prefix runs out.
//
// The read budget is bounded independently of the caller's limit, because a
// prefix can sit in front of thousands of entries the caller rejects (a
// non-article MIME, a resource path) and an unbounded scan would turn one
// keystroke into a walk of the namespace.
func (c *container) prefixScan(ns byte, prefix string, budget int, fn func(int, dirent) bool) error {
	key := searchKey(ns, prefix)
	i, err := c.lowerBound(key)
	if err != nil {
		return err
	}
	w := c.walk()
	for ; i < c.entries() && budget > 0; i, budget = i+1, budget-1 {
		d, err := w.entry(i)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(d.key(), key) {
			return nil
		}
		if !fn(i, d) {
			return nil
		}
	}
	return nil
}

// pathVariants expands a user's word into the spellings a ZIM path may use,
// most likely first. Paths are the wiki page titles with spaces written as
// '_', MediaWiki capitalises the first letter of every title, and a proper
// noun carries that capital on every word - so "new york" must still find
// "New_York" without any resident folded index. Each variant costs one binary
// search (~21 probes) and is only tried after the plainer ones miss, which is
// why a short closed list of spellings is cheaper here than the folded map it
// replaces. It is not case folding: a title that capitalises nothing a user
// would guess stays a preview miss, and is found once the dictionary is
// prepared.
func pathVariants(word string) []string {
	out := make([]string, 0, 5)
	add := func(s string) {
		if s == "" {
			return
		}
		for _, v := range out {
			if v == s {
				return
			}
		}
		out = append(out, s)
	}
	add(word)
	underscored := strings.ReplaceAll(word, " ", "_")
	add(underscored)
	add(upperFirst(word))
	add(upperFirst(underscored))
	add(titleWords(underscored))
	return out
}

// titleWords upper-cases the first letter of every '_'-separated word, which
// is how a multiword proper noun is spelled as a path ("New_York").
func titleWords(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		parts[i] = upperFirst(p)
	}
	return strings.Join(parts, "_")
}

// upperFirst upper-cases the first rune only, leaving the rest of the string
// exactly as it was: "new york" -> "New york". Capitalising the later words
// too is titleWords' job, and a separate variant on purpose.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	u := []rune(strings.ToUpper(string(r[0])))
	return string(u) + string(r[1:])
}
