// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package lsd

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/format/dsl"
	"github.com/wuweidict/wudict/internal/logx"
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
	source     *source
	page       pageCursor
	seen       uint32
	look       *heading
	pending    []dict.Entry
	done       bool
	abbrevOnce sync.Once
	abbrevs    *dsl.Abbreviations
}

func NewReader(path string) (*Reader, error) {
	s, err := openSource(path)
	if err != nil {
		return nil, err
	}
	return &Reader{source: s}, nil
}

func (r *Reader) Meta() dict.Meta { return r.source.meta }

func (r *Reader) abbreviations() *dsl.Abbreviations {
	r.abbrevOnce.Do(func() {
		path, ok := dict.AbbrevCompanion(r.source.path)
		if !ok {
			return
		}
		st, err := os.Stat(path)
		if err != nil || st.Size() > 8<<20 {
			if err == nil {
				logx.V("lsd abbreviations: %s is %d bytes - too large for a glossary, ignored", path, st.Size())
			}
			return
		}
		glossary, err := NewReader(path)
		if err != nil {
			logx.V("lsd abbreviations: %s: %v", path, err)
			return
		}
		defer glossary.Close()
		entries := make([]dict.Entry, 0, min(int(st.Size()/64), 20000))
		for len(entries) < 20000 {
			e, err := glossary.Next()
			if err != nil {
				if err != io.EOF {
					logx.V("lsd abbreviations: %s: %v", path, err)
				}
				break
			}
			entries = append(entries, e)
		}
		r.abbrevs = dsl.NewAbbreviations(entries)
	})
	return r.abbrevs
}

// ExtraMeta records the companion stamp used when article markup was prepared.
func (r *Reader) ExtraMeta() map[string]string {
	path, ok := dict.AbbrevCompanion(r.source.path)
	if !ok {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	meta := map[string]string{
		"abbrev_path":  path,
		"abbrev_size":  strconv.FormatInt(st.Size(), 10),
		"abbrev_mtime": st.ModTime().UTC().Truncate(time.Second).Format(time.RFC3339),
		"abbrev_count": "0",
	}
	if a := r.abbreviations(); a != nil {
		meta["abbrev_count"] = strconv.Itoa(a.Count())
	}
	return meta
}

func (r *Reader) nextHeading() (*heading, error) {
	for uint64(r.page.page)*pageSize < uint64(r.source.pagesEnd-r.source.header.pagesOffset) ||
		((r.source.header.version == 0x70000 || r.source.header.version == 0x90000 || r.source.header.version == 0x100000) && r.seen < r.source.header.entryCount) {
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
	if r.source.header.version == 0x70000 || r.source.header.version == 0x90000 || r.source.header.version == 0x100000 {
		// Legacy last pages may be truncated at EOF. Zero padding lets the
		// declared entries decode without changing the page bitstream.
		if base >= r.source.fileSize {
			return fmt.Errorf("lsd: read index page %d: EOF", r.page.page)
		}
		_, _ = r.source.f.ReadAt(buf[:], base)
	} else if _, err := r.source.f.ReadAt(buf[:], base); err != nil {
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
			if r.source.decoder.xorArticles {
				// Each X6 system article starts a fresh XOR block, including its
				// encoded length prefix.
				x6XOR(data)
			}
			size, payload, err := articleSize(data)
			if err != nil {
				return dict.Entry{}, err
			}
			body, err = r.source.decoder.decodeArticle(payload, size)
			if err == nil {
				body, err = dsl.NormalizeArticleWithAbbreviations(body, group[0], r.abbreviations())
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
		for _, word := range group[1:] {
			r.pending = append(r.pending, dict.Entry{Headwords: []string{word}, Body: body, Kind: dict.BodyHTML})
		}
		return dict.Entry{Headwords: group[:1], Body: body, Kind: dict.BodyHTML}, nil
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
func (d *Dict) Icon() (io.ReadCloser, string, error) {
	if len(d.source.icon) == 0 {
		return nil, "", dict.ErrNotFound
	}
	mime := iconMIME(d.source.icon)
	if mime == "" {
		return nil, "", dict.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(d.source.icon)), mime, nil
}
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
var _ dict.IconProvider = (*Dict)(nil)
