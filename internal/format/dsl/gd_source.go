// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/resource"
	"github.com/wuweidict/wudict/internal/store"
)

const GDReaderVersion = 8

type gdFileStamp struct {
	size  int64
	mtime int64
	value string
}

var gdStamps sync.Map

// Legacy descriptor retained for reading old prepared dictionaries only.
type gdSource struct {
	Source string `json:"source"`
	Stamp  string `json:"stamp"`
	Abbrev string `json:"abbrev,omitempty"`
}

func init() {
	dict.RegisterComparisonRemoval(".dslgd", func(p string) error {
		return os.WriteFile(p+".disabled", []byte("GD comparison disabled\n"), 0o600)
	})
	dict.RegisterSourceInput(".dslgd", func(p string) (string, error) {
		s, err := loadGDSource(p)
		return s.Source, err
	})
	dict.RegisterSourceRevision(".dslgd", func(p string) (string, error) {
		s, err := loadGDSource(p)
		if err != nil {
			return "", err
		}
		stamp, err := gdStamp(s.Source)
		if err != nil {
			return "", err
		}
		if ab, ok := dict.AbbrevCompanion(s.Source); ok {
			v, err := gdStamp(ab)
			if err != nil {
				return "", err
			}
			stamp += ":" + v
		}
		return stamp, nil
	})
	// Read legacy descriptors, but never generate a second runtime dictionary.
	dict.RegisterReader(".dslgd", func(p string) (dict.Reader, error) { return readGDSource(p) })
	dict.RegisterFormat(".dslgd", func(p string) (dict.Dictionary, error) {
		r, err := readGDSource(p)
		if err != nil {
			return nil, err
		}
		// Mirrors Open in dsl.go: the reader is opened whatever happens and its
		// header is read at every open; store.OpenSelfPrepared does the
		// prepare-or-open a self-preparing format needs.
		defer r.Close()
		src := r.Meta() // before the reader is consumed by an ingest
		s, err := store.OpenSelfPrepared(p, "dsl", func() (dict.Reader, error) { return r, nil })
		if err != nil {
			return nil, err
		}
		return &Dict{Store: s, srcPath: r.path, src: src}, nil
	})
	dict.RegisterProber(".dslgd", func(p string) (dict.Meta, error) {
		r, err := readGDSource(p)
		if err != nil {
			return dict.Meta{}, err
		}
		defer r.Close()
		return r.Meta(), nil
	})
	dict.RegisterReaderVersion("dsl-gd", GDReaderVersion)
	resource.Register("dsl-gd", resource.Provider{Sources: func(p string) []resource.Source {
		s, err := loadGDSource(p)
		if err != nil {
			return nil
		}
		return MediaSources(s.Source)
	}})
	dict.RegisterAbout("dsl-gd", func(p string) (dict.About, bool) {
		s, err := loadGDSource(p)
		if err != nil {
			return dict.About{}, false
		}
		return loadAnn(s.Source)
	})
}

func gdStamp(p string) (string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if cached, ok := gdStamps.Load(p); ok {
		stamp := cached.(gdFileStamp)
		if stamp.size == st.Size() && stamp.mtime == st.ModTime().UnixNano() {
			return stamp.value, nil
		}
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err = f.Stat()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, 1<<20)); err != nil {
		return "", err
	}
	value := fmt.Sprintf("%d:%d:%x", st.Size(), st.ModTime().UnixNano(), h.Sum(nil))
	gdStamps.Store(p, gdFileStamp{size: st.Size(), mtime: st.ModTime().UnixNano(), value: value})
	return value, nil
}

func loadGDSource(p string) (gdSource, error) {
	f, err := os.Open(p)
	if err != nil {
		return gdSource{}, err
	}
	defer f.Close()
	var s gdSource
	err = json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(&s)
	if err == nil && (s.Source == "" || strings.EqualFold(filepath.Ext(s.Source), ".dslgd")) {
		err = fmt.Errorf("invalid DSL comparison source")
	}
	return s, err
}

// NewGDReader reads legacy comparison receipts and serves reference tests.
// New DSL sources use NewArticleReader without a generated name or descriptor.
func NewGDReader(p string) (*Reader, error) {
	return NewGDReaderWithOptions(p, GDOptions{Enhance: true, Styles: true})
}

func NewGDReaderWithOptions(p string, options GDOptions) (*Reader, error) {
	r, err := NewReader(p)
	if err != nil {
		return nil, err
	}
	r.gd = true
	r.gdOptions = options
	r.meta.Name += " GD"
	r.meta.Format = "dsl-gd"
	return r, nil
}

func readGDSource(p string) (*Reader, error) {
	s, err := loadGDSource(p)
	if err != nil {
		return nil, err
	}
	r, err := NewGDReader(s.Source)
	if err != nil {
		return nil, err
	}
	r.meta.Path = p
	return r, nil
}
