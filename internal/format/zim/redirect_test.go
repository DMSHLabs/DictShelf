// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"os"
	"testing"
)

// TestRealFileRedirects is the measurement behind two decisions in this
// package: the bound on a redirect chain, and what isArticle requires of a
// redirect. It walks every directory entry, follows each redirect to its end
// and reports what it finds - depth, cycles, unreadable entries, and where the
// targets land - then fails only on the two things that would mean a real
// archive this reader cannot serve: an entry it cannot read, and a chain
// deeper than it will follow.
//
// Opt-in like TestRealFile, because it reads the whole pointer list: measured
// 2026-11-04 on the archives in D:\Dictionaries\wuDict\Dictionaries\zim - 19 s
// for the 2.0 GiB Russian Wiktionary, 48 s for the 5.7 GiB Russian Wikipedia.
// Both: every redirect resolves, every target is an HTML article, the deepest
// chain is ONE hop, no cycles, nothing unreadable - and in the Wiktionary
// 999 440 redirects collapse onto 153 594 articles, 124 221 of which are named
// by more than one path, 23 at the worst. Aliases are therefore the ordinary
// case, not an edge one, which is why `results` resolves and de-duplicates by
// TARGET while everything that lists rows walks paths.
func TestRealFileRedirects(t *testing.T) {
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

	c := d.c
	n := c.entries()
	redirects, maxDepth, cycles, unreadable := 0, 0, 0, 0
	var failed, html, nonHTML, notContent, otherNS, samePath int
	byMIME := map[string]int{}
	targets := map[int]int{}
	deepest, sample := -1, 0
	for i := 0; i < n; i++ {
		src, err := c.entry(i)
		if err != nil {
			continue // one unreadable non-redirect entry is not this test's subject
		}
		if !src.isRedirect() {
			continue
		}
		redirects++
		if src.namespace != c.contentNS() {
			otherNS++
		}
		seen := map[int]bool{i: true}
		cur, depth := int(src.redirect), 1
		for {
			if cur < 0 || cur >= n || seen[cur] {
				if seen[cur] {
					cycles++
				} else {
					unreadable++
				}
				break
			}
			seen[cur] = true
			nd, err := c.entry(cur)
			if err != nil {
				unreadable++
				break
			}
			if !nd.isRedirect() {
				break
			}
			cur, depth = int(nd.redirect), depth+1
			if depth > redirectHops {
				break
			}
		}
		if depth > maxDepth {
			maxDepth, deepest = depth, i
		}
		// Where the chain ends, through the same call the readers use.
		ti, tgt, err := c.resolve(i)
		if err != nil {
			failed++
			continue
		}
		targets[ti]++
		if tgt.path == src.path {
			samePath++
		}
		if !tgt.isContent() {
			notContent++
			continue
		}
		if c.htmlMIME[tgt.mimetype] {
			html++
		} else {
			nonHTML++
			byMIME[c.mimeOf(tgt)]++
		}
		// And a sample of them is reachable by the path a reader searches for.
		// Only redirects whose target really holds an article are sampled: an
		// entry that stores nothing has nothing to find, and that is the
		// archive's business, not a reader's bug.
		if sample < 20 && src.namespace == c.contentNS() {
			if body, err := c.content(tgt); err == nil && len(body) > 0 {
				if res, err := d.Exact(src.path, 1); err != nil || len(res) == 0 {
					t.Errorf("Exact(%q), a redirect's own path, returned %d results, err=%v", src.path, len(res), err)
				}
				sample++
			}
		}
	}
	shared, worst := 0, 0
	for _, k := range targets {
		if k > 1 {
			shared++
			if k > worst {
				worst = k
			}
		}
	}
	if maxDepth > 1 && deepest >= 0 {
		if de, err := c.entry(deepest); err == nil {
			t.Logf("deepest chain starts at entry %d, namespace %q, path %q", deepest, de.namespace, de.path)
		}
	}
	t.Logf("%d entries, %d redirects (%d outside the content namespace), deepest chain %d hop(s), %d cycles, %d unreadable",
		n, redirects, otherNS, maxDepth, cycles, unreadable)
	t.Logf("targets: failed=%d html=%d nonHTML=%d notContent=%d samePath=%d", failed, html, nonHTML, notContent, samePath)
	t.Logf("distinct targets=%d named by >1 path=%d, worst %d paths", len(targets), shared, worst)
	if len(byMIME) > 0 {
		t.Logf("non-HTML targets by mime: %v", byMIME)
	}

	if unreadable > 0 {
		t.Errorf("%d redirect entries could not be read", unreadable)
	}
	if maxDepth > redirectHops {
		t.Errorf("deepest chain is %d hops, deeper than the %d this reader follows", maxDepth, redirectHops)
	}
}
