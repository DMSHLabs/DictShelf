// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package favorites stores saved articles in the same SQLite databases used
// to search and serve them. Article HTML belongs to text.db; binary resources
// belong to media.db; provenance and collection metadata are SQLite rows.
package favorites

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/store"
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
	Token string `json:"token"`
	Size  int64  `json:"size,omitempty"`
}

type Article struct {
	ID         string           `json:"id"`
	EntryID    int64            `json:"entry_id"`
	Headword   string           `json:"headword"`
	BodyHash   string           `json:"-"`
	BodySize   int64            `json:"body_size"`
	Provenance Provenance       `json:"provenance"`
	Assets     map[string]Asset `json:"assets,omitempty"`
}

type Store struct {
	root string
}

func New(root string) *Store { return &Store{root: root} }

// NewAt keeps the collection directory as its identity; the main dictionary
// file is always text.db inside that directory.
func NewAt(root, _ string) *Store { return New(root) }

func (s *Store) Path() string    { return store.TextDBPath(s.root) }
func (s *Store) Objects() string { return "" }

func (s *Store) Ensure() error { return s.EnsureNamed("Favourites") }

func (s *Store) EnsureNamed(name string) error {
	if strings.TrimSpace(name) == "" {
		name = "Favourites"
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(s.Path()); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	reader := &emptyReader{name: name, path: s.Path()}
	contains, fullText := true, true
	_, err := store.Reconcile("", store.Target{Out: s.Path(), Contains: &contains, FullText: &fullText}, store.Hooks{
		Reader: func() (dict.Reader, error) { return reader, nil },
		FinalizeText: func(textDB string) error {
			return store.WriteFavouriteRecords(textDB, name, "", nil)
		},
	})
	return err
}

func (s *Store) Name() (string, error) {
	meta, err := store.ReadMeta(s.Path())
	if err != nil {
		return "", err
	}
	if meta["name"] == "" {
		return "Favourites", nil
	}
	return meta["name"], nil
}

func (s *Store) About() (string, error) {
	meta, err := store.ReadMeta(s.Path())
	if err != nil {
		return "", err
	}
	return meta["favourites_about"], nil
}

func (s *Store) SetName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
		return fmt.Errorf("favourites name must contain 1 to 100 characters")
	}
	return store.SetFavouriteName(s.Path(), name)
}

func (s *Store) SetAbout(about string) error {
	if !utf8.ValidString(about) || len(about) > 65536 {
		return fmt.Errorf("favourites description is too long or invalid")
	}
	return store.SetFavouriteAbout(s.Path(), about)
}

func (s *Store) Manifest() ([]Article, error) {
	if _, err := os.Stat(s.Path()); errors.Is(err, os.ErrNotExist) {
		if err := s.Ensure(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	records, err := store.ReadFavouriteRecords(s.Path())
	if err != nil {
		return nil, err
	}
	out := make([]Article, 0, len(records))
	for _, record := range records {
		article, err := articleFromRecord(record)
		if err != nil {
			return nil, err
		}
		out = append(out, article)
	}
	return out, nil
}

func (s *Store) Article(id string) (Article, error) {
	articles, err := s.Manifest()
	if err != nil {
		return Article{}, err
	}
	for _, article := range articles {
		if article.ID == id {
			return article, nil
		}
	}
	return Article{}, os.ErrNotExist
}

func (s *Store) Body(article Article) (string, error) {
	d, err := store.Open(s.Path())
	if err != nil {
		return "", err
	}
	defer d.Close()
	rows, err := d.ArticlesByIDs(context.Background(), []int64{article.EntryID})
	if err != nil {
		return "", err
	}
	if len(rows) != 1 {
		return "", os.ErrNotExist
	}
	return rows[0].Body, nil
}

func (s *Store) Has(dictionaryID, headword string) bool {
	d, err := store.Open(s.Path())
	if err != nil {
		return false
	}
	defer d.Close()
	return d.HasFavourite(dictionaryID, headword)
}

func (s *Store) AssetToken(entryID int64, rawURL string) (string, bool) {
	d, err := store.Open(s.Path())
	if err != nil {
		return "", false
	}
	defer d.Close()
	return d.AssetToken(entryID, rawURL)
}

func (s *Store) ResourceTokens() []string {
	d, err := store.Open(s.Path())
	if err != nil {
		return nil
	}
	defer d.Close()
	return d.Resources()
}

func (s *Store) OpenAsset(token string) (io.ReadCloser, string, error) {
	d, err := store.Open(s.Path())
	if err != nil {
		return nil, "", err
	}
	r, mime, err := d.Resource(token)
	if err != nil {
		d.Close()
		return nil, "", err
	}
	return &assetCloser{ReadCloser: r, close: d.Close}, mime, nil
}

func (s *Store) AssetSize(token string) (int64, error) {
	d, err := store.Open(s.Path())
	if err != nil {
		return 0, err
	}
	defer d.Close()
	return d.ResourceSize(token)
}

func (s *Store) Add(p Provenance, body string, assets []AssetInput) (Article, error) {
	if len(body) > MaxArticle {
		return Article{}, fmt.Errorf("article exceeds %d bytes", MaxArticle)
	}
	if strings.TrimSpace(p.DictionaryID) == "" || strings.TrimSpace(p.Headword) == "" {
		return Article{}, fmt.Errorf("source dictionary and headword are required")
	}
	articles, err := s.Manifest()
	if err != nil {
		return Article{}, err
	}
	for _, have := range articles {
		if have.Provenance.DictionaryID == p.DictionaryID && have.Headword == p.Headword {
			return Article{}, ErrDuplicate
		}
	}
	if p.SavedAt.IsZero() {
		p.SavedAt = time.Now().UTC()
	}
	id, err := randomID()
	if err != nil {
		return Article{}, err
	}
	article := Article{ID: id, Headword: p.Headword, BodySize: int64(len(body)), Provenance: p, Assets: make(map[string]Asset)}
	var size int64 = int64(len(body))
	for i, input := range assets {
		if len(input.Data) > MaxAsset {
			return Article{}, fmt.Errorf("resource %q exceeds %d bytes", input.URL, MaxAsset)
		}
		size += int64(len(input.Data))
		if size > MaxTotal {
			return Article{}, fmt.Errorf("favourites exceed %d bytes", MaxTotal)
		}
		if input.URL == "" {
			continue
		}
		token := id + "/" + fmt.Sprintf("%04x-%x", i, sha256.Sum256([]byte(input.URL)))
		key := input.Name
		if key == "" {
			key = input.URL
		}
		article.Assets[key] = Asset{URL: input.URL, Name: input.Name, MIME: input.MIME, Token: token, Size: int64(len(input.Data))}
		input.Token = token
		assets[i] = input
	}
	for _, have := range articles {
		size += have.BodySize
		for _, asset := range have.Assets {
			size += asset.Size
		}
	}
	if size > MaxTotal {
		return Article{}, fmt.Errorf("favourites exceed %d bytes", MaxTotal)
	}
	if err := s.rebuild(rebuildChange{add: &article, body: body, assets: assets}); err != nil {
		return Article{}, err
	}
	updated, err := s.Article(article.ID)
	if err == nil {
		return updated, nil
	}
	return article, nil
}

type AssetInput struct {
	URL, Name, MIME string
	Data            []byte
	Token           string
}

func (s *Store) Remove(id string) error {
	articles, err := s.Manifest()
	if err != nil {
		return err
	}
	found := false
	for _, article := range articles {
		found = found || article.ID == id
	}
	if !found {
		return os.ErrNotExist
	}
	return s.rebuild(rebuildChange{removeID: id})
}

func (s *Store) Collect() {}

func articleFromRecord(record store.FavouriteRecord) (Article, error) {
	var source Provenance
	if err := json.Unmarshal([]byte(record.SourceJSON), &source); err != nil {
		return Article{}, fmt.Errorf("read saved article source: %w", err)
	}
	var assets map[string]Asset
	if err := json.Unmarshal([]byte(record.AssetsJSON), &assets); err != nil {
		return Article{}, fmt.Errorf("read saved article resources: %w", err)
	}
	if assets == nil {
		assets = map[string]Asset{}
	}
	return Article{ID: record.ID, EntryID: record.EntryID, Headword: source.Headword, BodySize: record.BodySize, Provenance: source, Assets: assets}, nil
}

type assetCloser struct {
	io.ReadCloser
	close func() error
}

func (c *assetCloser) Close() error {
	err := c.ReadCloser.Close()
	if closeErr := c.close(); err == nil {
		err = closeErr
	}
	return err
}

type rebuildChange struct {
	add      *Article
	body     string
	assets   []AssetInput
	removeID string
}

type rebuildReader struct {
	stream   *store.ArticleStream
	byEntry  map[int64]store.FavouriteRecord
	change   rebuildChange
	name     string
	about    string
	path     string
	newRows  []store.FavouriteRecord
	finished bool
}

func (r *rebuildReader) Meta() dict.Meta {
	return dict.Meta{Name: r.name, Format: Format, Path: r.path, EntryCount: len(r.byEntry) + boolInt(r.change.add != nil) - boolInt(r.change.removeID != "")}
}

func (r *rebuildReader) ExtraMeta() map[string]string {
	return map[string]string{"description": r.about, "favourites_version": "1", "favourites_about": r.about}
}

func (r *rebuildReader) Next() (dict.Entry, error) {
	if r.finished {
		return dict.Entry{}, io.EOF
	}
	for {
		entryID, entry, err := r.stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return dict.Entry{}, err
		}
		record, ok := r.byEntry[entryID]
		if !ok {
			return dict.Entry{}, fmt.Errorf("saved article metadata missing for entry %d", entryID)
		}
		if record.ID == r.change.removeID {
			continue
		}
		record.EntryID = int64(len(r.newRows) + 1)
		record.BodySize = int64(len(entry.Body))
		r.newRows = append(r.newRows, record)
		return entry, nil
	}
	r.finished = true
	if r.change.add == nil {
		return dict.Entry{}, io.EOF
	}
	article := *r.change.add
	article.EntryID = int64(len(r.newRows) + 1)
	sourceJSON, err := json.Marshal(article.Provenance)
	if err != nil {
		return dict.Entry{}, err
	}
	assetsJSON, err := json.Marshal(article.Assets)
	if err != nil {
		return dict.Entry{}, err
	}
	r.newRows = append(r.newRows, store.FavouriteRecord{EntryID: article.EntryID, BodySize: article.BodySize, ID: article.ID,
		SourceID: article.Provenance.DictionaryID, SourceHeadword: article.Headword,
		SourceJSON: string(sourceJSON), AssetsJSON: string(assetsJSON)})
	return dict.Entry{Headwords: []string{article.Headword}, Body: r.change.body, Kind: dict.BodyHTML}, nil
}

func (r *rebuildReader) Close() error {
	if r.stream == nil {
		return nil
	}
	return r.stream.Close()
}

func (s *Store) rebuild(change rebuildChange) (resultErr error) {
	name, err := s.Name()
	if err != nil {
		return err
	}
	about, err := s.About()
	if err != nil {
		return err
	}
	records, err := store.ReadFavouriteRecords(s.Path())
	if err != nil {
		return err
	}
	source, err := store.Open(s.Path())
	if err != nil {
		return err
	}
	defer func() {
		if err := source.Close(); resultErr == nil && err != nil {
			resultErr = err
		}
	}()
	stream, err := source.OpenArticleStream(context.Background())
	if err != nil {
		return err
	}
	byEntry := make(map[int64]store.FavouriteRecord, len(records))
	for _, record := range records {
		byEntry[record.EntryID] = record
	}
	reader := &rebuildReader{stream: stream, byEntry: byEntry, change: change, name: name, about: about, path: s.Path()}
	tmpDir, err := os.MkdirTemp(filepath.Dir(s.root), ".favourites-build-")
	if err != nil {
		stream.Close()
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	textDB := store.TextDBPath(tmpDir)
	contains, fullText := true, true
	_, err = store.Reconcile("", store.Target{Out: textDB, Contains: &contains, FullText: &fullText, Media: store.MediaOn}, store.Hooks{
		Reader: func() (dict.Reader, error) { return reader, nil },
		FinalizeText: func(path string) error {
			return store.WriteFavouriteRecords(path, name, about, reader.newRows)
		},
		Backend: func() (dict.Dictionary, func(), error) {
			assets := map[string]AssetInput{}
			for _, record := range reader.newRows {
				article, err := articleFromRecord(record)
				if err != nil {
					return nil, nil, err
				}
				for _, asset := range article.Assets {
					assets[asset.Token] = AssetInput{URL: asset.URL, Name: asset.Name, MIME: asset.MIME, Token: asset.Token}
				}
			}
			for _, input := range change.assets {
				assets[input.Token] = input
			}
			overlay := &mediaOverlay{Store: source, name: name, path: s.Path(), assets: assets}
			return overlay, func() {}, nil
		},
	})
	if err != nil {
		return err
	}
	if err := source.Close(); err != nil {
		return err
	}
	if err := replaceDirectory(s.root, tmpDir); err != nil {
		return err
	}
	tmpDir = ""
	return nil
}

func replaceDirectory(target, prepared string) error {
	backup := target + ".previous-" + time.Now().UTC().Format("20060102T150405.000000000")
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(prepared, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	return os.RemoveAll(backup)
}

type mediaOverlay struct {
	*store.Store
	name, path string
	assets     map[string]AssetInput
}

func (d *mediaOverlay) Meta() dict.Meta { return dict.Meta{Name: d.name, Format: Format, Path: d.path} }
func (d *mediaOverlay) Resources() []string {
	out := make([]string, 0, len(d.assets))
	for token := range d.assets {
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}
func (d *mediaOverlay) Resource(name string) (io.ReadCloser, string, error) {
	if asset, ok := d.assets[name]; ok && asset.Data != nil {
		return io.NopCloser(bytes.NewReader(asset.Data)), asset.MIME, nil
	}
	return d.Store.Resource(name)
}

func (s *Store) Size() (int64, error) {
	var total int64
	for _, name := range []string{store.TextDBName, filepath.Base(store.ArticlesFilePath(s.Path())), store.MediaDBName} {
		info, err := os.Stat(filepath.Join(s.root, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if !info.IsDir() {
			total += info.Size()
		}
	}
	return total, nil
}

type emptyReader struct{ name, path string }

func (r *emptyReader) Meta() dict.Meta           { return dict.Meta{Name: r.name, Format: Format, Path: r.path} }
func (r *emptyReader) Next() (dict.Entry, error) { return dict.Entry{}, io.EOF }
func (r *emptyReader) Close() error              { return nil }
func (r *emptyReader) ExtraMeta() map[string]string {
	return map[string]string{"description": "", "favourites_version": "1", "favourites_about": ""}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
