// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package favorites stores immutable article snapshots independently from
// their source dictionaries. A small atomic pointer publishes complete
// manifests; article and resource bytes live in a content-addressed object
// directory and are never rewritten when the index changes.
package favorites

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

const (
	Format     = "favs"
	ID         = "favourites"
	Version    = 1
	MaxArticle = 64 << 20
	MaxAsset   = 512 << 20
	MaxTotal   = 8 << 30
)

var ErrDuplicate = errors.New("article is already in favourites")

// Provenance describes where the saved article came from; none of these
// values is consulted when serving the saved copy.
type Provenance struct {
	DictionaryID   string    `json:"dictionary_id"`
	DictionaryName string    `json:"dictionary_name"`
	DictionaryUUID string    `json:"dictionary_uuid,omitempty"`
	SourcePath     string    `json:"source_path,omitempty"`
	Headword       string    `json:"headword"`
	Headwords      int       `json:"headwords,omitempty"`
	Articles       int       `json:"articles,omitempty"`
	EntryID        string    `json:"entry_id,omitempty"`
	SavedAt        time.Time `json:"saved_at"`
}

type Asset struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	MIME  string `json:"mime,omitempty"`
	Hash  string `json:"hash"`
	Token string `json:"token"`
}

type Article struct {
	ID         string           `json:"id"`
	Headword   string           `json:"headword"`
	BodyHash   string           `json:"body_hash"`
	BodySize   int64            `json:"body_size"`
	Provenance Provenance       `json:"provenance"`
	Assets     map[string]Asset `json:"assets,omitempty"`
}

type manifest struct {
	Version   int       `json:"version"`
	Revision  string    `json:"revision"`
	Name      string    `json:"name,omitempty"`
	TotalSize int64     `json:"total_size"`
	Articles  []Article `json:"articles"`
}

type Store struct {
	root          string
	fileName      string
	mu            sync.Mutex
	cacheMu       sync.Mutex
	cacheRev      string
	cacheManifest manifest
	cacheHas      map[string]struct{}
}

func New(root string) *Store { return NewAt(root, "favourites.favs") }

// NewAt opens a collection whose manifest uses fileName within root.
// fileName is reduced to a basename so a dictionary path cannot escape root.
func NewAt(root, fileName string) *Store {
	fileName = filepath.Base(fileName)
	if !strings.HasSuffix(strings.ToLower(fileName), ".favs") {
		fileName = "favourites.favs"
	}
	return &Store{root: root, fileName: fileName}
}

func (s *Store) Path() string    { return filepath.Join(s.root, s.fileName) }
func (s *Store) Objects() string { return filepath.Join(s.root, "objects") }

func (s *Store) Name() (string, error) {
	m, err := s.currentManifest()
	if err != nil {
		return "", err
	}
	if m.Name == "" {
		return "Favourites", nil
	}
	return m.Name, nil
}

func (s *Store) SetName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("favourites name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writer, err := s.lockWriter()
	if err != nil {
		return err
	}
	defer writer.Close()
	m, err := s.readManifestLocked()
	if err != nil {
		return err
	}
	if m.Name == name {
		return nil
	}
	m.Name = name
	return s.publishLocked(m, false)
}

// Ensure provisions the empty source pointer and its first manifest. Repeated
// calls are idempotent and never replace snapshots that already exist.
func (s *Store) Ensure() error {
	return s.EnsureNamed("Favourites")
}

func (s *Store) EnsureNamed(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Favourites"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writer, err := s.lockWriter()
	if err != nil {
		return err
	}
	defer writer.Close()
	if _, err := os.Stat(s.Path()); errors.Is(err, os.ErrNotExist) {
		return s.publishLocked(manifest{Version: Version, Name: name}, false)
	} else if err != nil {
		return err
	}
	if _, err := s.readManifestLocked(); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.publishLocked(manifest{Version: Version, Name: name}, false)
}

func (s *Store) Has(dictionaryID, headword string) bool {
	m, err := s.currentManifest()
	if err != nil {
		return false
	}
	s.cacheMu.Lock()
	index := s.cacheHas
	if s.cacheRev != m.Revision {
		index = nil
	}
	s.cacheMu.Unlock()
	if index == nil {
		index = buildHasIndex(m.Articles)
	}
	_, ok := index[starKey(dictionaryID, headword)]
	return ok
}

func (s *Store) Current() (string, error) {
	b, err := fsx.ReadBounded(s.Path(), 128)
	if err != nil {
		return "", err
	}
	rev := strings.TrimSpace(string(b))
	if len(rev) != 32 {
		return "", fmt.Errorf("invalid favourites revision")
	}
	if _, err := hex.DecodeString(rev); err != nil {
		return "", fmt.Errorf("invalid favourites revision: %w", err)
	}
	return rev, nil
}

func (s *Store) Manifest() ([]Article, error) {
	m, err := s.currentManifest()
	if err != nil {
		return nil, err
	}
	return cloneArticles(m.Articles), nil
}

func (s *Store) Article(id string) (Article, error) {
	articles, err := s.Manifest()
	if err != nil {
		return Article{}, err
	}
	for _, a := range articles {
		if a.ID == id {
			return a, nil
		}
	}
	return Article{}, os.ErrNotExist
}

func (s *Store) Body(a Article) (string, error) {
	snap, err := s.OpenSnapshot()
	if err != nil {
		return "", err
	}
	defer snap.Close()
	for _, current := range snap.data.Articles {
		if current.ID == a.ID && current.BodyHash == a.BodyHash {
			return snap.Body(a)
		}
	}
	return "", os.ErrNotExist
}

func (s *Store) Asset(token string) (Asset, error) {
	m, err := s.currentManifest()
	if err != nil {
		return Asset{}, err
	}
	for _, a := range m.Articles {
		for _, asset := range a.Assets {
			if asset.Token == token {
				return asset, nil
			}
		}
	}
	return Asset{}, os.ErrNotExist
}

// AssetToken resolves a source URL in one prepared entry. The favourite Reader
// yields records in manifest order, and the prepared writer assigns the same
// stable, one-based article ID because snapshots contain no redirects.
func (s *Store) AssetToken(entryID int64, url string) (string, bool) {
	m, err := s.currentManifest()
	if err != nil || entryID <= 0 || entryID > int64(len(m.Articles)) {
		return "", false
	}
	return assetToken(m.Articles[entryID-1], url)
}

// snapshotAssetName resolves a URL to the same once-decoded resource path the
// snapshot writer records. ZIM paths can contain a literal percent escape, so
// URL-decoding the path exactly once is essential: %252C names a file whose
// stored path contains the literal text %2C.
func snapshotAssetName(raw string) string {
	for _, prefix := range []string{"sound://", "file://"} {
		if strings.HasPrefix(strings.ToLower(raw), prefix) {
			raw = raw[len(prefix):]
			break
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return ""
	}
	name := strings.TrimPrefix(strings.TrimPrefix(u.Path, "./"), "/")
	if name == "" || strings.HasPrefix(name, "../") {
		return ""
	}
	return path.Clean(name)
}

func (s *Store) OpenAsset(token string) (io.ReadCloser, string, error) {
	snap, err := s.OpenSnapshot()
	if err != nil {
		return nil, "", err
	}
	defer snap.Close()
	return snap.OpenAsset(token)
}

func (s *Store) ResourceTokens() ([]string, error) {
	m, err := s.currentManifest()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range m.Articles {
		for _, asset := range a.Assets {
			if !seen[asset.Token] {
				seen[asset.Token] = true
				out = append(out, asset.Token)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// Add publishes one snapshot. Callers supply the already-rendered article
// bytes and each relative resource's bytes. Publication changes one pointer
// only after every immutable object and the complete new manifest exist.
func (s *Store) Add(p Provenance, body string, assets []AssetInput) (Article, error) {
	if len(body) > MaxArticle {
		return Article{}, fmt.Errorf("article exceeds %d bytes", MaxArticle)
	}
	if strings.TrimSpace(p.DictionaryID) == "" || strings.TrimSpace(p.Headword) == "" {
		return Article{}, fmt.Errorf("source dictionary and headword are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writer, err := s.lockWriter()
	if err != nil {
		return Article{}, err
	}
	defer writer.Close()
	old, err := s.readManifestLocked()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Article{}, err
	}
	for _, have := range old.Articles {
		if have.Provenance.DictionaryID == p.DictionaryID && have.Headword == p.Headword {
			return Article{}, ErrDuplicate
		}
	}
	if p.SavedAt.IsZero() {
		p.SavedAt = time.Now().UTC()
	}
	articleID, err := randomID()
	if err != nil {
		return Article{}, err
	}
	objDir := s.Objects()
	if err := os.MkdirAll(objDir, 0o700); err != nil {
		return Article{}, err
	}
	bodyHash, err := putObject(objDir, []byte(body))
	if err != nil {
		return Article{}, err
	}
	article := Article{ID: articleID, Headword: p.Headword, BodyHash: bodyHash, BodySize: int64(len(body)), Provenance: p, Assets: map[string]Asset{}}
	var total int64 = int64(len(body))
	for i, input := range assets {
		if len(input.Data) > MaxAsset {
			return Article{}, fmt.Errorf("resource %q exceeds %d bytes", input.URL, MaxAsset)
		}
		total += int64(len(input.Data))
		if total > MaxTotal {
			return Article{}, fmt.Errorf("snapshot exceeds %d bytes", MaxTotal)
		}
		if input.URL == "" {
			continue
		}
		hash, err := putObject(objDir, input.Data)
		if err != nil {
			return Article{}, err
		}
		token := articleID + "/" + fmt.Sprintf("%04x-%x", i, sha256.Sum256([]byte(input.URL)))
		assetKey := input.Name
		if assetKey == "" {
			assetKey = input.URL
		}
		article.Assets[assetKey] = Asset{URL: input.URL, Name: input.Name, MIME: input.MIME, Hash: hash, Token: token}
	}
	if old.TotalSize+total > MaxTotal {
		return Article{}, fmt.Errorf("favourites exceed %d bytes", MaxTotal)
	}
	old.TotalSize += total
	old.Articles = append(old.Articles, article)
	if err := s.publishLocked(old, false); err != nil {
		return Article{}, err
	}
	return article, nil
}

type AssetInput struct {
	URL, Name, MIME string
	Data            []byte
}

func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	writer, err := s.lockWriter()
	if err != nil {
		return err
	}
	defer writer.Close()
	m, err := s.readManifestLocked()
	if err != nil {
		return err
	}
	out := m.Articles[:0]
	found := false
	for _, a := range m.Articles {
		if a.ID == id {
			found = true
			continue
		}
		out = append(out, a)
	}
	if !found {
		return os.ErrNotExist
	}
	m.Articles = out
	m.TotalSize = 0
	for _, a := range out {
		m.TotalSize += a.BodySize
		for _, asset := range a.Assets {
			fi, err := os.Stat(filepath.Join(s.Objects(), asset.Hash))
			if err != nil {
				return err
			}
			m.TotalSize += fi.Size()
		}
	}
	return s.publishLocked(m, true)
}

func (s *Store) readManifestLocked() (manifest, error) {
	rev, err := s.Current()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifest{Version: Version}, nil
		}
		return manifest{}, err
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
		return manifest{}, fmt.Errorf("unsupported or mismatched favourites manifest")
	}
	return m, nil
}

func (s *Store) publishLocked(m manifest, sweepObjects bool) error {
	rev, err := randomID()
	if err != nil {
		return err
	}
	m.Version, m.Revision = Version, rev
	for i := range m.Articles {
		if m.Articles[i].Assets == nil {
			m.Articles[i].Assets = map[string]Asset{}
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	versionDir := filepath.Join(s.root, "versions", rev)
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		return err
	}
	if err := fsx.WriteAtomic(filepath.Join(versionDir, "manifest.json"), b, 0o600); err != nil {
		return err
	}
	leasesDir := filepath.Join(s.root, "leases")
	if err := os.MkdirAll(leasesDir, 0o700); err != nil {
		return err
	}
	lease, err := os.OpenFile(filepath.Join(leasesDir, rev+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	_ = lease.Close()
	if err := fsx.WriteAtomic(s.Path(), []byte(rev+"\n"), 0o600); err != nil {
		return err
	}
	s.cacheMu.Lock()
	s.cacheRev, s.cacheManifest, s.cacheHas = "", manifest{}, nil
	s.cacheMu.Unlock()
	s.reclaimLocked(rev, sweepObjects)
	return nil
}

func putObject(dir string, data []byte) (string, error) {
	h := sha256.Sum256(data)
	name := hex.EncodeToString(h[:])
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err == nil {
		return name, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := fsx.WriteAtomic(p, data, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
