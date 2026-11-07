// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// Snapshot pins one manifest revision until Close. Its content-addressed
// objects remain live while this lease is held, even after a newer revision is
// published.
type Snapshot struct {
	store  *Store
	rev    string
	data   manifest
	lease  *fileLock
	has    map[string]struct{}
	mu     sync.RWMutex
	closed bool
}

func (s *Store) OpenSnapshot() (*Snapshot, error) {
	if err := os.MkdirAll(filepath.Join(s.root, "leases"), 0o700); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		rev, err := s.Current()
		if err != nil {
			return nil, err
		}
		leasePath := filepath.Join(s.root, "leases", rev+".lock")
		lease, ok, err := openFileLock(leasePath, lockShared, true)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if !ok {
			continue
		}
		m, err := s.loadRevision(rev)
		current, currentErr := s.Current()
		if err == nil && currentErr == nil && current == rev && m.Revision == rev {
			return &Snapshot{store: s, rev: rev, data: m, lease: lease, has: buildHasIndex(m.Articles)}, nil
		}
		_ = lease.Close()
		if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
			return nil, currentErr
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return nil, errors.New("favourites changed repeatedly while opening a snapshot")
}

func (s *Store) loadRevision(rev string) (manifest, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cacheRev == rev {
		return s.cacheManifest, nil
	}
	b, err := readManifestBytes(filepath.Join(s.root, "versions", rev, "manifest.json"))
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return manifest{}, err
	}
	if m.Version != Version || m.Revision != rev {
		return manifest{}, errors.New("unsupported or mismatched favourites manifest")
	}
	s.cacheRev, s.cacheManifest, s.cacheHas = rev, m, buildHasIndex(m.Articles)
	return m, nil
}

func (s *Store) currentManifest() (manifest, error) {
	for attempt := 0; attempt < 8; attempt++ {
		rev, err := s.Current()
		if err != nil {
			return manifest{}, err
		}
		m, err := s.loadRevision(rev)
		current, currentErr := s.Current()
		if err == nil && currentErr == nil && current == rev {
			return m, nil
		}
		if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
			return manifest{}, currentErr
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return manifest{}, err
		}
	}
	return manifest{}, errors.New("favourites changed repeatedly while reading a snapshot")
}

func buildHasIndex(articles []Article) map[string]struct{} {
	index := make(map[string]struct{}, len(articles))
	for _, a := range articles {
		index[starKey(a.Provenance.DictionaryID, a.Headword)] = struct{}{}
	}
	return index
}

func starKey(dictionaryID, headword string) string {
	return dictionaryID + "\x00" + headword
}

func (snap *Snapshot) Revision() string { return snap.rev }

func (snap *Snapshot) Articles() []Article {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	if snap.closed {
		return nil
	}
	return cloneArticles(snap.data.Articles)
}

func cloneArticles(articles []Article) []Article {
	out := make([]Article, len(articles))
	for i, a := range articles {
		out[i] = a
		if a.Assets != nil {
			out[i].Assets = make(map[string]Asset, len(a.Assets))
			for k, asset := range a.Assets {
				out[i].Assets[k] = asset
			}
		}
	}
	return out
}

func (snap *Snapshot) Name() string {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	if snap.closed || snap.data.Name == "" {
		return "Favourites"
	}
	return snap.data.Name
}

func (snap *Snapshot) Has(dictionaryID, headword string) bool {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	if snap.closed {
		return false
	}
	_, ok := snap.has[starKey(dictionaryID, headword)]
	return ok
}

func (snap *Snapshot) Body(a Article) (string, error) {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	if snap.closed {
		return "", os.ErrClosed
	}
	b, err := readObject(snap.store.Objects(), a.BodyHash, MaxArticle)
	return string(b), err
}

func (snap *Snapshot) AssetToken(entryID int64, rawURL string) (string, bool) {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	if snap.closed || entryID <= 0 || entryID > int64(len(snap.data.Articles)) {
		return "", false
	}
	return assetToken(snap.data.Articles[entryID-1], rawURL)
}

func assetToken(a Article, rawURL string) (string, bool) {
	if asset, ok := a.Assets[rawURL]; ok {
		return asset.Token, true
	}
	name := snapshotAssetName(rawURL)
	if name == "" {
		return "", false
	}
	for key, asset := range a.Assets {
		if strings.EqualFold(key, name) || strings.EqualFold(asset.Name, name) || strings.EqualFold(snapshotAssetName(asset.URL), name) {
			return asset.Token, true
		}
	}
	return "", false
}

func (snap *Snapshot) OpenAsset(token string) (io.ReadCloser, string, error) {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	if snap.closed {
		return nil, "", os.ErrClosed
	}
	for _, a := range snap.data.Articles {
		for _, asset := range a.Assets {
			if asset.Token == token {
				if !validObjectHash(asset.Hash) {
					return nil, "", os.ErrNotExist
				}
				f, err := os.Open(filepath.Join(snap.store.Objects(), asset.Hash))
				return f, asset.MIME, err
			}
		}
	}
	return nil, "", os.ErrNotExist
}

func (snap *Snapshot) ResourceTokens() []string {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	if snap.closed {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range snap.data.Articles {
		for _, asset := range a.Assets {
			if !seen[asset.Token] {
				seen[asset.Token] = true
				out = append(out, asset.Token)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (snap *Snapshot) Close() error {
	snap.mu.Lock()
	defer snap.mu.Unlock()
	if snap.closed {
		return nil
	}
	snap.closed = true
	return snap.lease.Close()
}

func readManifestBytes(path string) ([]byte, error) {
	return fsx.ReadBounded(path, 64<<20)
}

func readObject(dir, hash string, max int64) ([]byte, error) {
	if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
		return nil, os.ErrNotExist
	}
	return fsx.ReadBounded(filepath.Join(dir, hash), max)
}
