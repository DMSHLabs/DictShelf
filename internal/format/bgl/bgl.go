// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package bgl

import (
	"bytes"
	"io"
	"mime"
	"path"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/resource"
	"github.com/wuweidict/wudict/internal/store"
)

// ReaderVersion is the behaviour version of this format's Reader (see
// dict.RegisterReaderVersion). Bump it in the same commit as any change to
// what the Reader yields, and update the golden in reader_golden_test.go.
const ReaderVersion = 1

func init() {
	dict.RegisterFormat(".bgl", func(path string) (dict.Dictionary, error) { return Open(path) })
	dict.RegisterReader(".bgl", func(path string) (dict.Reader, error) { return NewReader(path) })
	dict.RegisterReaderVersion("bgl", ReaderVersion)
}

// Dict is the BGL "direct" backend. BGL has no native index, so Open ingests
// into a cached text.db on first use (SPEC §1); the cache name embeds a
// source-content hash, so a changed source re-ingests automatically. Embedded
// resources are scanned from the source lazily on first request.
type Dict struct {
	*store.Store
	srcPath string
	// src is the source's own header, re-read at every open (the reader is
	// opened anyway). Its description and header win over the text.db copies,
	// which are frozen at preparation: a folder prepared before either was
	// read in full still reports what the file says now.
	src dict.Meta

	resOnce sync.Once
	res     *Embedded
	resErr  error
}

func Open(path string) (*Dict, error) {
	r, err := NewReader(path)
	if err != nil {
		return nil, err
	}
	// The reader is opened whatever happens: the source's own header is
	// read at every open.
	defer r.Close()
	src := r.Meta() // before the reader is consumed by an ingest
	s, err := store.OpenSelfPrepared(path, "bgl", func() (dict.Reader, error) { return r, nil })
	if err != nil {
		return nil, err
	}
	return &Dict{Store: s, srcPath: path, src: src}, nil
}

func (d *Dict) Meta() dict.Meta {
	m := d.Store.Meta()
	m.Format = "bgl"
	m.Path = d.srcPath
	m.Description, m.Header = d.src.Description, d.src.Header
	return m
}

func (d *Dict) Close() error { return d.Store.Close() }

// loadRes scans the source BGL for its embedded resource blocks. Kept lazy so
// a dictionary that never serves an image never pays the decompression.
func (d *Dict) loadRes() {
	d.res, d.resErr = ReadEmbedded(d.srcPath)
}

// Resource streams one embedded resource (image/HTML) by name,
// case-insensitively.
func (d *Dict) Resource(name string) (io.ReadCloser, string, error) {
	d.resOnce.Do(d.loadRes)
	if d.resErr != nil {
		return nil, "", d.resErr
	}
	rc, err := d.res.Open(name)
	if err != nil {
		return nil, "", err
	}
	return rc, mime.TypeByExtension(path.Ext(name)), nil
}

// Resources lists the embedded resource names (for full-ingest media packing).
func (d *Dict) Resources() []string {
	d.resOnce.Do(d.loadRes)
	if d.resErr != nil {
		return nil
	}
	return d.res.List()
}

// Embedded is the resources a BGL file carries inside it, read into memory by
// one pass over the file. It is a resource.Source, and reading it opens no
// dictionary and prepares nothing: `wudict dump` writes a BGL's resources
// through it.
type Embedded struct {
	data  map[string][]byte // by lowercased name
	names []string          // sorted, in the file's spelling
}

var _ resource.Source = (*Embedded)(nil)

// ReadEmbedded reads the resources embedded in the BGL file at path.
func ReadEmbedded(path string) (*Embedded, error) {
	data, names, err := scanResources(path)
	if err != nil {
		return nil, err
	}
	return &Embedded{data: data, names: names}, nil
}

// Open returns one resource by name, case-insensitively, or dict.ErrNotFound.
func (e *Embedded) Open(name string) (io.ReadCloser, error) {
	norm := strings.ToLower(strings.TrimLeft(path.Clean(name), "/"))
	if norm == "" || norm == "." || strings.HasPrefix(norm, "..") {
		return nil, dict.ErrNotFound
	}
	if b, ok := e.data[norm]; ok {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return nil, dict.ErrNotFound
}

// List returns the resource names, sorted.
func (e *Embedded) List() []string { return append([]string(nil), e.names...) }

func (e *Embedded) Close() error { return nil }
