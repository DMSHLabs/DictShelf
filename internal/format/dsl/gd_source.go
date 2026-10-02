// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

const GDReaderVersion = 2

type gdFileStamp struct {
	size  int64
	mtime int64
	value string
}

var gdStamps sync.Map

// The descriptor gives two views of one file independent library ownership.
// Only this small reference is duplicated; DSL and media remain in place.
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
	dict.RegisterComparison(".dsl", gdComparison)
	dict.RegisterComparison(".dsl.dz", gdComparison)
	dict.RegisterReader(".dslgd", func(p string) (dict.Reader, error) { return readGDSource(p) })
	dict.RegisterFormat(".dslgd", func(p string) (dict.Dictionary, error) {
		r, err := readGDSource(p)
		if err != nil {
			return nil, err
		}
		return openReader(p, r)
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

func gdComparison(p string) (string, error) {
	p = dict.CanonPath(p)
	stamp, err := gdStamp(p)
	if err != nil {
		return "", err
	}
	s := gdSource{Source: p, Stamp: stamp}
	if ab, ok := dict.AbbrevCompanion(p); ok {
		s.Abbrev, err = gdStamp(ab)
		if err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	id := sha256.Sum256([]byte(p))
	name := store.FolderName(p) + " GD-" + hex.EncodeToString(id[:6]) + ".dslgd"
	dir := filepath.Join(store.DefaultDBDir(), ".dsl-gd")
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target + ".disabled"); err == nil {
		return "", nil
	}
	if previous, err := os.ReadFile(target); err == nil && bytes.Equal(previous, data) {
		return target, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".comparison-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", err
	}
	return target, nil
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

// NewGDReader shares file decoding and entry boundaries with the current reader,
// but uses an independent tree parser and heading expansion.
func NewGDReader(p string) (*Reader, error) {
	r, err := NewReader(p)
	if err != nil {
		return nil, err
	}
	r.gd = true
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
