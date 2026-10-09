// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/resource"
	"github.com/wuweidict/wudict/internal/store"
)

// ReaderVersion is the behaviour version of this format's Reader (see
// dict.RegisterReaderVersion). Bump it in the same commit as any change to
// what the Reader yields, and update the golden in reader_golden_test.go.
const ReaderVersion = 11

func init() {
	openFn := func(path string) (dict.Dictionary, error) { return Open(path) }
	readFn := func(path string) (dict.Reader, error) { return NewArticleReader(path) }
	dict.RegisterFormat(".dsl", openFn)
	dict.RegisterReader(".dsl", readFn)
	// Compressed DSL. Registered as the full ".dsl.dz" suffix (not bare ".dz")
	// so a StarDict ".dict.dz" companion is never matched here; Open/NewReader
	// handle the gunzip. matchKey prefers this longest suffix.
	dict.RegisterFormat(".dsl.dz", openFn)
	dict.RegisterReader(".dsl.dz", readFn)
	dict.RegisterReaderVersion("dsl", ReaderVersion)
	// O8: a prepared DSL reaches its media through the path alone. No Fetcher -
	// DSL keeps nothing inside the .dsl itself, so there is no location to
	// record and the Sources below are the complete answer.
	resource.Register("dsl", resource.Provider{Sources: MediaSources})
}

// Dict is the DSL "direct" backend. DSL has no native index, so Open
// transparently prepares a library folder (<db dir>/<source name>/text.db) on
// first use (SPEC §1); a changed source is detected from the recorded
// size/mtime/hash and re-indexed in place. Resources stay lazy in
// `<name>.files.zip`, the matching `.files` folder, or loose beside the .dsl.
type Dict struct {
	*store.Store
	srcPath string
	// src is the source's own header, re-read at every open (the reader is
	// opened anyway). Its description and header win over the text.db copies,
	// which are frozen at preparation: a folder prepared before either was
	// read in full still reports what the file says now.
	src dict.Meta

	resOnce sync.Once
	res     []resource.Source
	resMu   sync.Mutex // guards res against a concurrent Close
}

// NewArticleReader is the application reader. NewReader retains the upstream
// implementation for merge compatibility and reference tests only.
func NewArticleReader(path string) (*Reader, error) {
	r, err := NewReader(path)
	if err == nil {
		r.gd = true
		r.gdOptions = ArticleOptions{Enhance: true, Styles: true}
	}
	return r, err
}

func Open(path string) (*Dict, error) {
	r, err := NewArticleReader(path)
	if err != nil {
		return nil, err
	}
	// The reader is opened whatever happens: the source's own header is
	// read at every open (see src above).
	defer r.Close()
	src := r.Meta() // before the reader is consumed by an ingest
	s, err := store.OpenSelfPrepared(path, "dsl", func() (dict.Reader, error) { return r, nil })
	if err != nil {
		return nil, err
	}
	return &Dict{Store: s, srcPath: r.path, src: src}, nil
}

func (d *Dict) Meta() dict.Meta {
	m := d.Store.Meta()
	m.Format = d.src.Format
	m.Path = d.src.Path
	m.Description, m.Header = d.src.Description, d.src.Header
	return m
}

func (d *Dict) Close() error {
	// Do, not a flag: a Resource call racing Close must find the sources
	// already built and closed, never build a fresh set nothing will close.
	d.resOnce.Do(func() {})
	d.resMu.Lock()
	for _, s := range d.res {
		s.Close()
	}
	d.res = nil
	d.resMu.Unlock()
	return d.Store.Close()
}

// Resource serves from the dictionary's resource containers, in the order
// LingvoDSL itself documents: the `.files.zip` archive first, then the
// `.files` folder, then loose beside the source.
func (d *Dict) Resource(name string) (io.ReadCloser, string, error) {
	for _, src := range d.sources() {
		if rc, err := src.Open(name); err == nil {
			return rc, resource.MIME(name), nil
		}
	}
	return nil, "", dict.ErrNotFound
}

// Resources lists what the containers hold, for media packing. The folder the
// dictionary merely sits in contributes nothing: it holds other dictionaries
// and their assets, and packing them would copy a neighbour's media into this
// dictionary's library folder.
func (d *Dict) Resources() []string {
	return resource.ListAll(d.sources())
}

func (d *Dict) sources() []resource.Source {
	d.resOnce.Do(d.loadSources)
	d.resMu.Lock()
	defer d.resMu.Unlock()
	return d.res
}

func (d *Dict) loadSources() {
	res := MediaSources(d.srcPath)
	d.resMu.Lock()
	d.res = res
	d.resMu.Unlock()
}

// MediaSources builds the resource containers of a DSL from its PATH alone -
// no parsing, no headwords, nothing opened but the archives themselves.
//
// Without it a prepared DSL would reach its images by opening the whole .dsl
// again (the registry's resource fallback), which for a large one means parsing
// hundreds of megabytes of text to serve a thumbnail. Everything below is
// derived from the file name, so the prepared folder - which records the source
// path - can do it directly. Registered as the format's O8 provider; the method
// above is the same call, so the two can never drift apart.
func MediaSources(srcPath string) []resource.Source {
	// A ".dsl.dz" names its resources after either the compressed file or the
	// ".dsl" inside it; both spellings are in the wild, and so is the bare
	// dictionary name with no format suffix at all ("x.files.zip" beside
	// "x.dsl.dz"). All three are tried, most specific first.
	//
	// The bare stem is dict.Stem, the same function removal and the dictionary
	// panel name companions with (internal/dict/companions.go): a spelling
	// listed there but not resolved here is a zip the user is told belongs to
	// this dictionary, and is deleted with it, while every image inside it
	// 404s.
	bases := []string{srcPath}
	if strings.EqualFold(filepath.Ext(srcPath), ".dz") {
		bases = append(bases, strings.TrimSuffix(srcPath, filepath.Ext(srcPath)))
	}
	if st := dict.Stem(srcPath); st != srcPath {
		bases = append(bases, st)
	}
	var res []resource.Source
	for _, b := range bases {
		if z, err := resource.OpenZip(b + ".files.zip"); err == nil {
			res = append(res, z)
		}
	}
	for _, b := range bases {
		if dir := b + ".files"; resource.IsDir(dir) {
			res = append(res, resource.NewDir(dir))
		}
	}
	// Last: a file lying loose beside the .dsl. Exact paths only - this
	// folder is not the dictionary's own, so it is never walked or listed.
	return append(res, resource.NewDirExact(filepath.Dir(srcPath)))
}
