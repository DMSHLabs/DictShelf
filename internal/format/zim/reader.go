// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"io"
	"sort"

	"github.com/wuweidict/wudict/internal/dict"
)

// Reader is the sequential ingest scan.
//
// Entries are visited in CLUSTER order rather than path order, so each cluster
// is decompressed exactly once - path order would revisit the same cluster
// thousands of times, and a cluster is megabytes. slob's Reader pre-sorts by
// bin/item for the same reason.
//
// Redirects come last, as a second pass: they hold no content, so they cost a
// single dirent read each, and by then every target headword they name has
// already been emitted.
type Reader struct {
	d             *Dict
	headwordsOnly bool
	scanPos       int

	articles  []articleRef
	pos       int
	redirects []int
	rpos      int
}

type articleRef struct {
	cluster uint32
	blob    uint32
	entry   int
}

func NewReader(path string) (*Reader, error) {
	return newReader(path, false)
}

// NewHeadwordReader prepares the searchable title list without decompressing
// article clusters. The prepared database is only an index; article bodies
// continue to come from the original ZIM file.
func NewHeadwordReader(path string) (*Reader, error) {
	return newReader(path, true)
}

func newReader(path string, headwordsOnly bool) (*Reader, error) {
	d, err := Open(path)
	if err != nil {
		return nil, err
	}
	r := &Reader{d: d, headwordsOnly: headwordsOnly}
	if headwordsOnly {
		// The path-pointer list is already sorted and the title index needs no
		// cluster reads. Walk it in place instead of retaining millions of
		// article references or sorting them in memory.
		r.scanPos = d.lo
		return r, nil
	}
	for i := d.lo; i < d.hi; i++ {
		de, err := d.c.entry(i)
		if err != nil {
			d.Close()
			return nil, err
		}
		switch {
		case de.isRedirect():
			r.redirects = append(r.redirects, i)
		case d.c.htmlMIME[de.mimetype]:
			r.articles = append(r.articles, articleRef{de.cluster, de.blob, i})
		}
		// Anything else in the content namespace is a resource: it stays in
		// the source file and is packed by the media pass, not ingested here.
	}
	sort.Slice(r.articles, func(a, b int) bool {
		if r.articles[a].cluster != r.articles[b].cluster {
			return r.articles[a].cluster < r.articles[b].cluster
		}
		return r.articles[a].blob < r.articles[b].blob
	})
	return r, nil
}

func (r *Reader) Meta() dict.Meta { return r.d.Meta() }

// ExtraMeta marks the prepared database as a title-only ZIM index. The server
// uses this to reopen the native source for article bodies after searching it.
func (r *Reader) ExtraMeta() map[string]string {
	if r.headwordsOnly {
		return map[string]string{"index_kind": "headwords"}
	}
	return nil
}

func (r *Reader) Next() (dict.Entry, error) {
	if r.headwordsOnly {
		for r.scanPos < r.d.hi {
			i := r.scanPos
			r.scanPos++
			de, err := r.d.c.entry(i)
			if err != nil {
				return dict.Entry{}, err
			}
			if de.isRedirect() {
				_, tgt, err := r.d.c.resolve(i)
				if err != nil || !r.d.c.htmlMIME[tgt.mimetype] {
					continue
				}
				hw, to := de.headword(), tgt.headword()
				if hw == "" || hw == to {
					continue
				}
				return dict.Entry{Headwords: []string{hw}, LinkTo: to}, nil
			}
			if r.d.c.htmlMIME[de.mimetype] {
				return dict.Entry{Headwords: []string{de.headword()}, Kind: dict.BodyHTML}, nil
			}
		}
		return dict.Entry{}, io.EOF
	}
	for r.pos < len(r.articles) {
		a := r.articles[r.pos]
		r.pos++
		de, err := r.d.c.entry(a.entry)
		if err != nil {
			return dict.Entry{}, err
		}
		body, err := r.d.c.content(de)
		if err != nil {
			return dict.Entry{}, err
		}
		if len(body) == 0 {
			continue // real Kiwix files carry empty content entries
		}
		return dict.Entry{
			Headwords: []string{de.headword()},
			Body:      r.d.articleHTML(body),
			Kind:      dict.BodyHTML,
		}, nil
	}
	for r.rpos < len(r.redirects) {
		i := r.redirects[r.rpos]
		r.rpos++
		src, err := r.d.c.entry(i)
		if err != nil {
			return dict.Entry{}, err
		}
		_, tgt, err := r.d.c.resolve(i)
		if err != nil {
			continue // a redirect into nothing is one lost alias, not a failure
		}
		if !r.d.c.htmlMIME[tgt.mimetype] {
			continue
		}
		hw, to := src.headword(), tgt.headword()
		if hw == "" || hw == to {
			continue
		}
		return dict.Entry{Headwords: []string{hw}, LinkTo: to}, nil
	}
	return dict.Entry{}, io.EOF
}

func (r *Reader) Close() error { return r.d.Close() }
