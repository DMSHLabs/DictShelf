// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/resource"
)

// ReaderVersion is the behaviour version of this format's Reader (see
// dict.RegisterReaderVersion). Bump it in the same commit as any change to
// what the Reader yields, and update the golden in reader_golden_test.go.
const ReaderVersion = 12

var titleRepairsEnabled atomic.Bool

func init() { titleRepairsEnabled.Store(true) }

// SetTitleRepairs switches the app-specific recovery rules for malformed
// human-written titles. Its version is part of prepared-index staleness.
func SetTitleRepairs(enabled bool) {
	titleRepairsEnabled.Store(enabled)
	version := ReaderVersion
	if !enabled {
		version++
	}
	dict.RegisterReaderVersion("dsl", version)
}

func init() {
	openFn := func(path string) (dict.Dictionary, error) { return Open(path) }
	readFn := func(path string) (dict.Reader, error) { return NewArticleReader(path) }
	dict.RegisterFormat(".dsl", openFn)
	dict.RegisterReader(".dsl", readFn)
	// Compressed DSL spellings are registered as full suffixes (not bare .dz/.dz2)
	// so StarDict companions are never matched here; the reader detects their
	// encoding from contents. matchKey prefers these over shorter suffixes.
	dict.RegisterFormat(".dsl.dz", openFn)
	dict.RegisterReader(".dsl.dz", readFn)
	dict.RegisterFormat(".dsl.dz2", openFn)
	dict.RegisterReader(".dsl.dz2", readFn)
	dict.RegisterReaderVersion("dsl", ReaderVersion)
	// O8: a prepared DSL reaches its media through the path alone. No Fetcher -
	// DSL keeps nothing inside the .dsl itself, so there is no location to
	// record and the Sources below are the complete answer.
	resource.Register("dsl", resource.Provider{Sources: MediaSources})
}

// Dict is the DSL direct backend. It reads the source without creating a
// prepared index; library indexes are built by the add queue or explicit
// indexing actions. Resources stay lazy in `<name>.files.zip`, the matching
// `.files` folder, or loose beside the .dsl.
type Dict struct {
	reader  *Reader
	resOnce sync.Once
	resMu   sync.Mutex
	res     []resource.Source
}

// NewArticleReader is the application reader. NewReader retains the upstream
// implementation for merge compatibility and reference tests only.
func NewArticleReader(path string) (*Reader, error) {
	r, err := NewReader(path)
	if err == nil {
		r.gd = true
		r.gdOptions = ArticleOptions{Enhance: true, Styles: true}
		r.repairTitles = titleRepairsEnabled.Load()
	}
	return r, err
}

func Open(path string) (*Dict, error) {
	r, err := NewArticleReader(path)
	if err != nil {
		return nil, err
	}
	return &Dict{reader: r}, nil
}

func (d *Dict) Meta() dict.Meta                           { return d.reader.Meta() }
func (d *Dict) Caps() dict.Caps                           { return dict.Caps{} }
func (d *Dict) Exact(string, int) ([]dict.Result, error)  { return nil, dict.ErrUnsupported }
func (d *Dict) Prefix(string, int) ([]dict.Result, error) { return nil, dict.ErrUnsupported }
func (d *Dict) Keywords(int, int) []string                { return nil }

func (d *Dict) Close() error {
	d.resOnce.Do(func() {})
	d.resMu.Lock()
	for _, src := range d.res {
		src.Close()
	}
	d.res = nil
	d.resMu.Unlock()
	return d.reader.Close()
}

func (d *Dict) sources() []resource.Source {
	d.resOnce.Do(func() {
		sources := MediaSources(d.reader.path)
		d.resMu.Lock()
		d.res = sources
		d.resMu.Unlock()
	})
	d.resMu.Lock()
	defer d.resMu.Unlock()
	return d.res
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

func (d *Dict) PreviewBytes() int64 { return int64(d.reader.Meta().EntryCount) * 350 }

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
	if strings.EqualFold(filepath.Ext(srcPath), ".dz") || strings.EqualFold(filepath.Ext(srcPath), ".dz2") {
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
