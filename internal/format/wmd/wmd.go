// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package wmd reads WuWeiDict markdown (docs/WUDICT-MARKDOWN.md): standard
// CommonMark with GFM tables, one dictionary per `<name>.wudict.md`, a plain
// `<name>.md` whose line 2 is the `wudict` field, or either one gzip- or
// dictzip-compressed.
package wmd

import (
	"compress/gzip"
	"io"
	"path/filepath"
	"sync"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/resource"
	"github.com/wuweidict/wudict/internal/store"
)

// ReaderVersion is the behaviour version of this format's Reader (see
// dict.RegisterReaderVersion). Bump it in the same commit as any change to
// what the Reader yields, and update the golden in reader_golden_test.go.
const ReaderVersion = 1

// Format is the format name recorded in a prepared library folder.
const Format = "wmd"

func init() {
	open := func(p string) (dict.Dictionary, error) { return Open(p) }
	read := func(p string) (dict.Reader, error) { return NewReader(p) }
	// Every spelling is claimed by content alike: the `wudict` field on line
	// 2 is the gate, whatever the file is called (R2.2).
	for _, ext := range []string{Ext, ExtGz, ExtDz, ExtPlain} {
		dict.RegisterSniffed(ext, Claim, open, read)
	}
	dict.RegisterReaderVersion(Format, ReaderVersion)
	resource.Register(Format, resource.Provider{Sources: MediaSources})
}

// claimHead bounds what Claim reads: the title line and the line after it.
const claimHead = 4 << 10

// Claim decides whether a file is a WuWeiDict markdown dictionary, from the
// start of its content r - a `.wudict.md`, a plain `.md`, or either one
// gzip- or dictzip-compressed, which name tells apart. The gate is the same
// for every spelling (R2.2, R3.1): line 1 is `# ` and a title, and line 2 is
// the `wudict` field. Nothing else qualifies, so a README or a changelog with
// `## ` sections is never taken for one. A misspelt field (`Wudict:1`) still
// qualifies: reading it then reports the E-version that tells the author what
// to fix, instead of ignoring the file.
func Claim(name string, r io.Reader) error {
	if compressed(name) {
		zr, err := gzip.NewReader(r)
		if err != nil {
			return formatErr("not a gzip stream: %v", err)
		}
		r = zr
	}
	head, err := io.ReadAll(io.LimitReader(r, claimHead))
	if err != nil {
		return formatErr("cannot be read: %v", err)
	}
	return claim(head)
}

func claim(head []byte) error {
	line1, rest := cutLine(decode(head)) // R2.1: BOM, CRLF and CR
	line2, _ := cutLine(rest)
	switch {
	case !titleLine(line1):
		return formatErr("line 1 must be `# ` and the dictionary title")
	case !versionLine(string(line2)):
		return formatErr("line 2 must be `wudict: 1`, the version of the format")
	}
	return nil
}

// Dict is the direct backend. Like DSL, the format has no index of its own:
// the first open prepares a library folder and every open serves from it. A
// prepared dictionary is opened from its folder alone, since the folder keeps
// the title, description and header fields; the source is read again only
// when it changed and has to be prepared again.
type Dict struct {
	*store.Store
	srcPath string

	resOnce sync.Once
	res     []resource.Source
	resMu   sync.Mutex // guards res against a concurrent Close
}

func Open(path string) (*Dict, error) {
	var r *Reader
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	s, err := store.OpenSelfPrepared(path, Format, func() (dict.Reader, error) {
		var err error
		r, err = NewReader(path)
		if err != nil {
			return nil, err
		}
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	return &Dict{Store: s, srcPath: path}, nil
}

func (d *Dict) Meta() dict.Meta {
	m := d.Store.Meta()
	m.Format = Format
	m.Path = d.srcPath
	return m
}

func (d *Dict) Close() error {
	d.resOnce.Do(func() {})
	d.resMu.Lock()
	for _, s := range d.res {
		s.Close()
	}
	d.res = nil
	d.resMu.Unlock()
	return d.Store.Close()
}

func (d *Dict) Resource(name string) (io.ReadCloser, string, error) {
	for _, src := range d.sources() {
		if rc, err := src.Open(name); err == nil {
			return rc, resource.MIME(name), nil
		}
	}
	return nil, "", dict.ErrNotFound
}

// Resources lists what the resource containers hold, for media packing; the
// folder the file sits in contributes nothing, since an exact-path source
// lists nothing (see dsl.Dict.Resources).
func (d *Dict) Resources() []string {
	return resource.ListAll(d.sources())
}

func (d *Dict) sources() []resource.Source {
	d.resOnce.Do(func() {
		res := MediaSources(d.srcPath)
		d.resMu.Lock()
		d.res = res
		d.resMu.Unlock()
	})
	d.resMu.Lock()
	defer d.resMu.Unlock()
	return d.res
}

// MediaSources builds the resource containers from the path alone (R2.3):
// `<stem>.files.zip`, then the `<stem>.files` folder, then loose files beside
// the source (exact paths only, and always last). The stem of `x.wudict.md`
// (or `.gz`, `.dz`) is `x.wudict`; that of a plain `x.md` is `x`.
func MediaSources(srcPath string) []resource.Source {
	base := dict.Stem(srcPath)
	var res []resource.Source
	if z, err := resource.OpenZip(base + ".files.zip"); err == nil {
		res = append(res, z)
	}
	if dir := base + ".files"; resource.IsDir(dir) {
		res = append(res, resource.NewDir(dir))
	}
	return append(res, resource.NewDirExact(filepath.Dir(srcPath)))
}
