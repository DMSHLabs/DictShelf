// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package favs exposes the user's immutable favourite snapshots as a normal
// DictShelf dictionary source.
package favs

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/favorites"
)

const ReaderVersion = 1

func init() {
	dict.RegisterFormat(".favs", func(path string) (dict.Dictionary, error) { return Open(path) })
	dict.RegisterReader(".favs", func(path string) (dict.Reader, error) { return NewReader(path) })
	dict.RegisterReaderVersion(favorites.Format, ReaderVersion)
	dict.RegisterProber(".favs", Probe)
}

type Dict struct {
	snapshot *favorites.Snapshot
	meta     dict.Meta
	articles []favorites.Article
}

func Open(path string) (*Dict, error) {
	s := favorites.New(filepath.Dir(path))
	snapshot, err := s.OpenSnapshot()
	if err != nil {
		return nil, err
	}
	articles := snapshot.Articles()
	return &Dict{snapshot: snapshot, articles: articles, meta: dict.Meta{Name: snapshot.Name(), Format: favorites.Format, Path: path, EntryCount: len(articles)}}, nil
}

func Probe(path string) (dict.Meta, error) {
	s := favorites.New(filepath.Dir(path))
	snapshot, err := s.OpenSnapshot()
	if err != nil {
		return dict.Meta{}, err
	}
	defer snapshot.Close()
	return dict.Meta{Name: snapshot.Name(), Format: favorites.Format, Path: path, EntryCount: len(snapshot.Articles())}, nil
}

func (d *Dict) Meta() dict.Meta { return d.meta }
func (d *Dict) Caps() dict.Caps { return dict.Caps{Exact: true, Prefix: true} }
func (d *Dict) Exact(word string, limit int) ([]dict.Result, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []dict.Result
	for i, a := range d.articles {
		if a.Headword != word {
			continue
		}
		body, err := d.snapshot.Body(a)
		if err != nil {
			return nil, err
		}
		out = append(out, dict.Result{Headword: a.Headword, Body: body, EntryID: int64(i + 1), FavoriteID: a.ID, ResourceMap: assetMap(body, a)})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
func (d *Dict) Prefix(word string, limit int) ([]dict.Result, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []dict.Result
	for i, a := range d.articles {
		if !strings.HasPrefix(strings.ToLower(a.Headword), strings.ToLower(word)) {
			continue
		}
		body, err := d.snapshot.Body(a)
		if err != nil {
			return nil, err
		}
		out = append(out, dict.Result{Headword: a.Headword, Body: body, EntryID: int64(i + 1), FavoriteID: a.ID, ResourceMap: assetMap(body, a)})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func assetMap(_ string, a favorites.Article) map[string]string {
	out := make(map[string]string, len(a.Assets))
	for key, asset := range a.Assets {
		name, ok := savedResourceName(key)
		if !ok {
			name, ok = savedResourceName(asset.Name)
		}
		if ok {
			out[strings.ToLower(name)] = asset.Token
		}
	}
	return out
}

func savedResourceName(raw string) (string, bool) {
	for _, prefix := range []string{"sound://", "file://"} {
		if strings.HasPrefix(strings.ToLower(raw), prefix) {
			raw = raw[len(prefix):]
			break
		}
	}
	raw = strings.TrimPrefix(raw, "./")
	raw = strings.TrimPrefix(raw, "/")
	if raw == "" || strings.HasPrefix(raw, "../") {
		return "", false
	}
	return filepath.ToSlash(filepath.Clean(raw)), true
}
func (d *Dict) Keywords(offset, n int) []string {
	words := make([]string, 0, len(d.articles))
	for _, a := range d.articles {
		words = append(words, a.Headword)
	}
	sort.Strings(words)
	lo, hi, ok := dict.KeywordRange(len(words), offset, n)
	if !ok {
		return nil
	}
	return words[lo:hi]
}
func (d *Dict) Resource(name string) (io.ReadCloser, string, error) {
	return d.snapshot.OpenAsset(name)
}
func (d *Dict) AssetToken(entryID int64, url string) (string, bool) {
	return d.snapshot.AssetToken(entryID, url)
}
func (d *Dict) FavoriteID(entryID int64) string {
	if entryID <= 0 || entryID > int64(len(d.articles)) {
		return ""
	}
	return d.articles[entryID-1].ID
}
func (d *Dict) FavoriteSource(entryID int64) *dict.FavoriteSource {
	if entryID <= 0 || entryID > int64(len(d.articles)) {
		return nil
	}
	a := d.articles[entryID-1]
	return &dict.FavoriteSource{
		DictionaryID: a.Provenance.DictionaryID, DictionaryName: a.Provenance.DictionaryName,
		Headword: a.Provenance.Headword, SourcePath: a.Provenance.SourcePath,
		Headwords: a.Provenance.Headwords, Articles: a.Provenance.Articles,
	}
}
func (d *Dict) Resources() []string { return d.snapshot.ResourceTokens() }
func (d *Dict) Close() error        { return d.snapshot.Close() }

type Reader struct {
	path     string
	snapshot *favorites.Snapshot
	articles []favorites.Article
	at       int
	meta     dict.Meta
}

func NewReader(path string) (*Reader, error) {
	s := favorites.New(filepath.Dir(path))
	snapshot, err := s.OpenSnapshot()
	if err != nil {
		return nil, err
	}
	articles := snapshot.Articles()
	return &Reader{path: path, snapshot: snapshot, articles: articles, meta: dict.Meta{Name: snapshot.Name(), Format: favorites.Format, Path: path, EntryCount: len(articles)}}, nil
}
func (r *Reader) Meta() dict.Meta { return r.meta }
func (r *Reader) Close() error    { return r.snapshot.Close() }
func (r *Reader) Next() (dict.Entry, error) {
	for r.at < len(r.articles) {
		a := r.articles[r.at]
		r.at++
		body, err := r.snapshot.Body(a)
		if err != nil {
			return dict.Entry{}, fmt.Errorf("read favourite %q: %w", a.Headword, err)
		}
		return dict.Entry{Headwords: []string{a.Headword}, Body: body, Kind: dict.BodyHTML}, nil
	}
	return dict.Entry{}, io.EOF
}

var _ dict.Dictionary = (*Dict)(nil)
var _ dict.Reader = (*Reader)(nil)
var _ dict.ResourceLister = (*Dict)(nil)
