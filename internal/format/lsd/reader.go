// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/format/dsl"
)

type pageCursor struct {
	page        uint32
	count       uint32
	position    int
	known       []uint16
	isLeaf      bool
	initialized bool
	bits        *bitReader
}

type heading struct {
	title string
	ref   uint32
}

// Reader walks index pages and decodes one distinct article per Next call.
// It keeps only the current 512-byte page, adjacent aliases and one lookahead
// heading in memory.
type Reader struct {
	source  *source
	page    pageCursor
	seen    uint32
	look    *heading
	refs    map[uint32]string
	pending []dict.Entry
	done    bool
}

func NewReader(path string) (*Reader, error) {
	s, err := openSource(path)
	if err != nil {
		return nil, err
	}
	return &Reader{source: s, refs: make(map[uint32]string)}, nil
}

func (r *Reader) Meta() dict.Meta { return r.source.meta }

func (r *Reader) nextHeading() (*heading, error) {
	for uint64(r.page.page)*pageSize < uint64(r.source.pagesEnd-r.source.header.pagesOffset) {
		if !r.page.initialized {
			if err := r.loadPage(); err != nil {
				return nil, err
			}
			if !r.page.isLeaf || r.page.count == 0 {
				r.page.page++
				r.page.initialized = false
				continue
			}
		}
		if r.page.position >= int(r.page.count) {
			r.page.page++
			r.page.initialized = false
			continue
		}
		title, sorted, ref, err := r.source.decoder.readHeading(r.page.bits, r.page.known)
		if err != nil {
			return nil, fmt.Errorf("lsd: page %d heading %d: %w", r.page.page, r.page.position, err)
		}
		r.page.known = sorted
		r.page.position++
		r.seen++
		return &heading{title: title, ref: ref}, nil
	}
	if r.seen != r.source.header.entryCount {
		return nil, fmt.Errorf("lsd: index contains %d headings, header declares %d", r.seen, r.source.header.entryCount)
	}
	r.done = true
	return nil, io.EOF
}

func (r *Reader) loadPage() error {
	base := int64(r.source.header.pagesOffset) + int64(r.page.page)*pageSize
	var buf [pageSize]byte
	if _, err := r.source.f.ReadAt(buf[:], base); err != nil {
		return fmt.Errorf("lsd: read index page %d: %w", r.page.page, err)
	}
	b := &bitReader{data: buf[:]}
	leaf, err := b.read(1)
	if err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		if _, err := b.read(16); err != nil {
			return err
		}
	}
	n, err := b.read(16)
	if err != nil {
		return err
	}
	b.align()
	r.page.count = n
	r.page.position = 0
	r.page.known = nil
	r.page.isLeaf = leaf != 0
	r.page.initialized = true
	r.page.bits = b
	return nil
}

func (r *Reader) Next() (dict.Entry, error) {
	if len(r.pending) > 0 {
		e := r.pending[0]
		r.pending = r.pending[1:]
		return e, nil
	}
	if r.done {
		return dict.Entry{}, io.EOF
	}
	for {
		first := r.look
		r.look = nil
		if first == nil {
			var err error
			first, err = r.nextHeading()
			if err != nil {
				return dict.Entry{}, err
			}
		}
		group := dsl.HeadwordKeys(first.title)
		if len(group) == 0 {
			group = []string{strings.TrimSpace(first.title)}
		}
		groupSeen := make(map[string]bool, len(group))
		for _, word := range group {
			groupSeen[word] = true
		}
		var displayTitles []string
		addDisplayTitle := func(title string) {
			display := dsl.HeadwordDisplay(title)
			if display != "" && display != html.EscapeString(group[0]) {
				displayTitles = append(displayTitles, "<b>"+display+"</b>")
			}
		}
		addDisplayTitle(first.title)
		for {
			next, err := r.nextHeading()
			if err == io.EOF {
				break
			}
			if err != nil {
				return dict.Entry{}, err
			}
			if next.ref != first.ref {
				r.look = next
				break
			}
			addDisplayTitle(next.title)
			for _, word := range dsl.HeadwordKeys(next.title) {
				if word != "" && !groupSeen[word] {
					group = append(group, word)
					groupSeen[word] = true
				}
			}
		}
		if primary, ok := r.refs[first.ref]; ok {
			aliases := make([]string, 0, len(group))
			for _, word := range group {
				if word != primary {
					aliases = append(aliases, word)
				}
			}
			if len(aliases) == 0 {
				continue
			}
			for _, alias := range aliases {
				r.pending = append(r.pending, dict.Entry{Headwords: []string{alias}, LinkTo: primary})
			}
			entry := r.pending[0]
			r.pending = r.pending[1:]
			return entry, nil
		}
		r.refs[first.ref] = group[0]
		articleRegionEnd := uint64(r.source.header.pagesOffset)
		start := uint64(r.source.header.articlesOffset) + uint64(first.ref)
		if start+2 > articleRegionEnd {
			return dict.Entry{}, fmt.Errorf("lsd: invalid article reference %d", first.ref)
		}
		available := articleRegionEnd - start
		if available > 128<<20 {
			available = 128 << 20
		}
		readSize := min(64<<10, available)
		var body string
		for {
			data := make([]byte, int(readSize))
			if _, err := r.source.f.ReadAt(data, int64(start)); err != nil && err != io.EOF {
				return dict.Entry{}, fmt.Errorf("lsd: read article at %d: %w", start, err)
			}
			size, payload, err := articleSize(data)
			if err != nil {
				return dict.Entry{}, err
			}
			body, err = r.source.decoder.decodeArticle(payload, size)
			if err == nil {
				body, err = dsl.NormalizeArticle(body, group[0])
				if err != nil {
					return dict.Entry{}, fmt.Errorf("lsd: normalize article %q: %w", group[0], err)
				}
				if len(displayTitles) > 0 {
					body = strings.Join(displayTitles, "<br/>") + "<br/>" + body
				}
				break
			}
			if !errors.Is(err, errTruncatedBits) || readSize >= available {
				return dict.Entry{}, fmt.Errorf("lsd: decode article %q: %w", group[0], err)
			}
			readSize = min(readSize*2, available)
		}
		return dict.Entry{Headwords: group, Body: body, Kind: dict.BodyHTML}, nil
	}
}

func (r *Reader) Close() error {
	if r.source == nil || r.source.f == nil {
		return nil
	}
	err := r.source.f.Close()
	r.source.f = nil
	return err
}

// Dict exposes the minimum direct-format surface. Search uses the prepared
// headword index, while Resources and direct article lookup are added with the
// overlay stage.
type Dict struct{ source *source }

func Open(path string) (*Dict, error) {
	s, err := openSource(path)
	if err != nil {
		return nil, err
	}
	return &Dict{source: s}, nil
}

func (d *Dict) Meta() dict.Meta                                     { return d.source.meta }
func (d *Dict) Caps() dict.Caps                                     { return dict.Caps{} }
func (d *Dict) Exact(string, int) ([]dict.Result, error)            { return nil, dict.ErrUnsupported }
func (d *Dict) Prefix(string, int) ([]dict.Result, error)           { return nil, dict.ErrUnsupported }
func (d *Dict) Keywords(int, int) []string                          { return nil }
func (d *Dict) Resource(name string) (io.ReadCloser, string, error) { return d.source.resource(name) }
func (d *Dict) Close() error {
	if d.source == nil || d.source.f == nil {
		return nil
	}
	err := d.source.f.Close()
	d.source.f = nil
	return err
}

var _ dict.Reader = (*Reader)(nil)
var _ dict.Dictionary = (*Dict)(nil)
var _ dict.ResourceLister = (*Dict)(nil)
