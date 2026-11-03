// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"os"
	"testing"
)

// TestRealFileRedirectDepth answers the one question that decides whether
// redirectHops is a bound or a handicap: how deep do the chains in real
// archives actually go? It walks every directory entry, follows each
// redirect to its end and reports the deepest chain found - and fails only if
// a chain is deeper than this reader will follow, which would mean a real
// archive we cannot read.
//
// Opt-in like TestRealFile, because it reads the whole pointer list: measured
// 2026-11-04 on the archives in D:\Dictionaries\wuDict\Dictionaries\zim - 21 s
// for the 2.0 GiB Russian Wiktionary, 60 s for the 5.7 GiB Wikipedia,
// 8 s for the 3.6 GiB Wikisource. All three: 8.2M entries, 3.97M of them
// redirects (every inflected form and every alternate spelling in a
// Wiktionary), and the deepest chain is ONE hop.
func TestRealFileRedirectDepth(t *testing.T) {
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
	deepest := -1
	for i := 0; i < n; i++ {
		de, err := c.entry(i)
		if err != nil {
			unreadable++
			continue
		}
		if !de.isRedirect() {
			continue
		}
		redirects++
		seen := map[int]bool{i: true}
		cur, depth := int(de.redirect), 1
		for {
			if cur < 0 || cur >= n {
				unreadable++
				break
			}
			if seen[cur] {
				cycles++
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
				// Deeper than this reader will follow: name it.
				break
			}
		}
		if depth > maxDepth {
			maxDepth, deepest = depth, i
		}
	}
	if maxDepth > 1 {
		if de, err := c.entry(deepest); err == nil {
			t.Logf("deepest chain starts at entry %d, namespace %q, path %q", deepest, de.namespace, de.path)
		}
	}
	t.Logf("%d entries, %d redirects, deepest chain %d hop(s), %d cycles, %d unreadable",
		n, redirects, maxDepth, cycles, unreadable)
	if unreadable > 0 {
		t.Errorf("%d entries could not be read", unreadable)
	}
	if maxDepth > redirectHops {
		t.Errorf("deepest chain is %d hops, deeper than the %d this reader follows", maxDepth, redirectHops)
	}
}
