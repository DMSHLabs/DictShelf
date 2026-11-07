// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

const collectionCatalogVersion = 3

var ErrInvalidCollection = errors.New("invalid favourites collection")

type collectionCatalog struct {
	Version int      `json:"version"`
	IDs     []string `json:"ids"`
}

type CollectionInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	About    string   `json:"about"`
	Path     string   `json:"path"`
	Files    []string `json:"files,omitempty"`
	Articles int      `json:"articles"`
	Size     int64    `json:"size"`
}

// Every collection owns a prepared SQLite folder at one level under root.
// The default collection's stable directory is "fav".
type Collections struct {
	root   string
	mu     sync.RWMutex
	ids    []string
	stores map[string]*Store
}

func OpenCollections(root, defaultName string) (*Collections, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("favourites storage path is required")
	}
	if strings.TrimSpace(defaultName) == "" {
		defaultName = "Favourites"
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	c := &Collections{root: root, stores: map[string]*Store{}}
	ids, err := c.readCatalog()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		ids = []string{ID}
	}
	c.ids = append([]string(nil), ids...)
	for _, id := range ids {
		if (id != ID && !validCollectionID(id)) || c.stores[id] != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidCollection, id)
		}
		collection := New(collectionPath(root, id))
		if err := collection.EnsureNamed(defaultName); err != nil {
			return nil, fmt.Errorf("open favourites collection %q: %w", id, err)
		}
		c.stores[id] = collection
	}
	if err := c.saveCatalog(c.ids); err != nil {
		return nil, err
	}
	return c, nil
}

func collectionPath(root, id string) string {
	if id == ID {
		return filepath.Join(root, "fav")
	}
	return filepath.Join(root, id)
}

func (c *Collections) readCatalog() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(c.root, "collections.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("%w: catalogue is too large", ErrInvalidCollection)
	}
	var catalog collectionCatalog
	if err := json.Unmarshal(data, &catalog); err != nil || catalog.Version != collectionCatalogVersion || catalog.IDs == nil {
		return nil, fmt.Errorf("%w: cannot read collection catalogue", ErrInvalidCollection)
	}
	return catalog.IDs, nil
}

func (c *Collections) saveCatalog(ids []string) error {
	data, err := json.Marshal(collectionCatalog{Version: collectionCatalogVersion, IDs: ids})
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(filepath.Join(c.root, "collections.json"), data, 0o600)
}

func (c *Collections) List() ([]CollectionInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]CollectionInfo, 0, len(c.ids))
	for _, id := range c.ids {
		set := c.stores[id]
		articles, err := set.Manifest()
		if err != nil {
			return nil, err
		}
		name, err := set.Name()
		if err != nil {
			return nil, err
		}
		about, err := set.About()
		if err != nil {
			return nil, err
		}
		size, err := set.Size()
		if err != nil {
			return nil, err
		}
		files := []string{set.Path()}
		for _, file := range []string{filepath.Base(store.ArticlesFilePath(set.Path())), store.MediaDBName} {
			path := filepath.Join(filepath.Dir(set.Path()), file)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				files = append(files, path)
			}
		}
		out = append(out, CollectionInfo{ID: id, Name: name, About: about, Path: set.Path(), Files: files, Articles: len(articles), Size: size})
	}
	return out, nil
}

func (c *Collections) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if (id != ID && !validCollectionID(id)) || c.stores[id] == nil {
		return fmt.Errorf("%w: collection not found", ErrInvalidCollection)
	}
	if len(c.ids) <= 1 {
		return fmt.Errorf("%w: at least one collection must remain", ErrInvalidCollection)
	}
	data := collectionPath(c.root, id)
	trashID, err := randomID()
	if err != nil {
		return err
	}
	trash := filepath.Join(c.root, "."+id+"-removing-"+trashID[:8])
	if err := os.Rename(data, trash); err != nil {
		return err
	}
	next := make([]string, 0, len(c.ids)-1)
	for _, current := range c.ids {
		if current != id {
			next = append(next, current)
		}
	}
	if err := c.saveCatalog(next); err != nil {
		_ = os.Rename(trash, data)
		return err
	}
	c.ids = next
	delete(c.stores, id)
	return os.RemoveAll(trash)
}

func (c *Collections) Rename(id, name string) (CollectionInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
		return CollectionInfo{}, fmt.Errorf("%w: name must contain 1 to 100 characters", ErrInvalidCollection)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	set := c.stores[id]
	if set == nil {
		return CollectionInfo{}, fmt.Errorf("%w: collection not found", ErrInvalidCollection)
	}
	for _, currentID := range c.ids {
		if currentID == id {
			continue
		}
		currentName, err := c.stores[currentID].Name()
		if err != nil {
			return CollectionInfo{}, err
		}
		if strings.EqualFold(currentName, name) {
			return CollectionInfo{}, fmt.Errorf("%w: a collection with that name already exists", ErrInvalidCollection)
		}
	}
	if err := set.SetName(name); err != nil {
		return CollectionInfo{}, err
	}
	return collectionInfo(id, set)
}

func (c *Collections) SetAbout(id, about string) (CollectionInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	set := c.stores[id]
	if set == nil {
		return CollectionInfo{}, fmt.Errorf("%w: collection not found", ErrInvalidCollection)
	}
	if err := set.SetAbout(about); err != nil {
		return CollectionInfo{}, fmt.Errorf("%w: %v", ErrInvalidCollection, err)
	}
	return collectionInfo(id, set)
}

func collectionInfo(id string, set *Store) (CollectionInfo, error) {
	name, err := set.Name()
	if err != nil {
		return CollectionInfo{}, err
	}
	about, err := set.About()
	if err != nil {
		return CollectionInfo{}, err
	}
	articles, err := set.Manifest()
	if err != nil {
		return CollectionInfo{}, err
	}
	size, err := set.Size()
	if err != nil {
		return CollectionInfo{}, err
	}
	return CollectionInfo{ID: id, Name: name, About: about, Path: set.Path(), Articles: len(articles), Size: size}, nil
}

func (c *Collections) Store(id string) (*Store, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	set, ok := c.stores[id]
	return set, ok
}

func (c *Collections) IDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.ids...)
}

func (c *Collections) Create(name string) (CollectionInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
		return CollectionInfo{}, fmt.Errorf("%w: name must contain 1 to 100 characters", ErrInvalidCollection)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, currentID := range c.ids {
		currentName, err := c.stores[currentID].Name()
		if err != nil {
			return CollectionInfo{}, err
		}
		if strings.EqualFold(currentName, name) {
			return CollectionInfo{}, fmt.Errorf("%w: a collection with that name already exists", ErrInvalidCollection)
		}
	}
	var id string
	for range 5 {
		var raw [8]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return CollectionInfo{}, err
		}
		id = ID + "-" + hex.EncodeToString(raw[:])
		if _, exists := c.stores[id]; !exists {
			break
		}
		id = ""
	}
	if id == "" {
		return CollectionInfo{}, errors.New("could not allocate a favourites collection id")
	}
	set := New(collectionPath(c.root, id))
	if err := set.EnsureNamed(name); err != nil {
		return CollectionInfo{}, err
	}
	next := append(append([]string(nil), c.ids...), id)
	if err := c.saveCatalog(next); err != nil {
		_ = os.RemoveAll(set.root)
		return CollectionInfo{}, err
	}
	c.ids, c.stores[id] = next, set
	return collectionInfo(id, set)
}

func validCollectionID(id string) bool {
	if !strings.HasPrefix(id, ID+"-") {
		return false
	}
	encoded := strings.TrimPrefix(id, ID+"-")
	if len(encoded) != 16 {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}
