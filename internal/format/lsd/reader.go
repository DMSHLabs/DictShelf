// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"fmt"
	"io"
	"unicode/utf16"

	"github.com/wuweidict/wudict/internal/dict"
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
	text []uint16
	ref  uint32
}

// Reader walks index pages and decodes one distinct article per Next call.
// It keeps only the current 512-byte page, adjacent aliases and one lookahead
// heading in memory.
type Reader struct {
	source  *source
	page    pageCursor
	seen    uint32
	look    *heading
	lastRef uint32
	hasRef  bool
	done    bool
}

func NewReader(path string) (*Reader, error) {
	s, err := openSource(path)
	if err != nil {
		return nil, err
	}
	return &Reader{source: s}, nil
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
		text, ref, err := r.source.decoder.readHeading(r.page.bits, r.page.known)
		if err != nil {
			return nil, fmt.Errorf("lsd: page %d heading %d: %w", r.page.page, r.page.position, err)
		}
		r.page.known = text
		r.page.position++
		r.seen++
		if r.hasRef && ref < r.lastRef {
			return nil, fmt.Errorf("lsd: article references are out of order (%d after %d)", ref, r.lastRef)
		}
		r.lastRef, r.hasRef = ref, true
		return &heading{text: text, ref: ref}, nil
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
	if r.done {
		return dict.Entry{}, io.EOF
	}
	first := r.look
	r.look = nil
	if first == nil {
		var err error
		first, err = r.nextHeading()
		if err != nil {
			return dict.Entry{}, err
		}
	}
	group := []string{string(utf16.Decode(first.text))}
	endRef := uint32(0)
	for {
		next, err := r.nextHeading()
		if err == io.EOF {
			break
		}
		if err != nil {
			return dict.Entry{}, err
		}
		if next.ref != first.ref {
			endRef = next.ref
			r.look = next
			break
		}
		word := string(utf16.Decode(next.text))
		if word != group[len(group)-1] {
			group = append(group, word)
		}
	}
	articleEnd := uint64(r.source.header.pagesOffset - r.source.header.articlesOffset)
	if r.look != nil {
		articleEnd = uint64(endRef)
	}
	if uint64(first.ref) >= articleEnd {
		return dict.Entry{}, fmt.Errorf("lsd: invalid article reference %d (end %d)", first.ref, articleEnd)
	}
	start := uint64(r.source.header.articlesOffset) + uint64(first.ref)
	bound := articleEnd - uint64(first.ref)
	if start+bound > uint64(r.source.fileSize) || bound > 128<<20 {
		return dict.Entry{}, fmt.Errorf("lsd: invalid or oversized article range at %d (%d bytes)", start, bound)
	}
	data := make([]byte, int(bound))
	if _, err := r.source.f.ReadAt(data, int64(start)); err != nil && err != io.EOF {
		return dict.Entry{}, fmt.Errorf("lsd: read article at %d: %w", start, err)
	}
	size, payload, err := articleSize(data)
	if err != nil {
		return dict.Entry{}, err
	}
	body, err := r.source.decoder.decodeArticle(payload, size)
	if err != nil {
		return dict.Entry{}, fmt.Errorf("lsd: decode article %q: %w", group[0], err)
	}
	return dict.Entry{Headwords: group, Body: body, Kind: dict.BodyDSL}, nil
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

func (d *Dict) Meta() dict.Meta                                { return d.source.meta }
func (d *Dict) Caps() dict.Caps                                { return dict.Caps{} }
func (d *Dict) Exact(string, int) ([]dict.Result, error)       { return nil, dict.ErrUnsupported }
func (d *Dict) Prefix(string, int) ([]dict.Result, error)      { return nil, dict.ErrUnsupported }
func (d *Dict) Keywords(int, int) []string                     { return nil }
func (d *Dict) Resource(string) (io.ReadCloser, string, error) { return nil, "", dict.ErrNotFound }
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
