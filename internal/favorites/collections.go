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
)

const collectionCatalogVersion = 1

var ErrInvalidCollection = errors.New("invalid favourites collection")

type collectionCatalog struct {
	Version int      `json:"version"`
	IDs     []string `json:"ids"`
}

type CollectionInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Collections owns the default snapshot store and any additional collections.
// The default store stays at the legacy path so existing users keep their data.
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
	defaultStore := New(root)
	if err := defaultStore.EnsureNamed(defaultName); err != nil {
		return nil, err
	}
	c := &Collections{root: root, ids: []string{ID}, stores: map[string]*Store{ID: defaultStore}}
	ids, err := c.readCatalog()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if !validCollectionID(id) || id == ID || c.stores[id] != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidCollection, id)
		}
		store := New(filepath.Join(root, "collections", id))
		if err := store.Ensure(); err != nil {
			return nil, fmt.Errorf("open favourites collection %q: %w", id, err)
		}
		c.ids = append(c.ids, id)
		c.stores[id] = store
	}
	return c, nil
}

func (c *Collections) readCatalog() ([]string, error) {
	path := filepath.Join(c.root, "collections.json")
	data, err := os.ReadFile(path)
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
		store := c.stores[id]
		name, err := store.Name()
		if err != nil {
			return nil, err
		}
		out = append(out, CollectionInfo{ID: id, Name: name, Path: store.Path()})
	}
	return out, nil
}

func (c *Collections) Store(id string) (*Store, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	store, ok := c.stores[id]
	return store, ok
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
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return CollectionInfo{}, err
		}
		id = ID + "-" + hex.EncodeToString(random[:])
		if _, exists := c.stores[id]; !exists {
			break
		}
		id = ""
	}
	if id == "" {
		return CollectionInfo{}, errors.New("could not allocate a favourites collection id")
	}
	root := filepath.Join(c.root, "collections", id)
	store := New(root)
	if err := store.EnsureNamed(name); err != nil {
		return CollectionInfo{}, err
	}
	next := append(append([]string(nil), c.ids...), id)
	if err := c.saveCatalog(next[1:]); err != nil {
		_ = os.RemoveAll(root)
		return CollectionInfo{}, err
	}
	c.ids, c.stores[id] = next, store
	return CollectionInfo{ID: id, Name: name, Path: store.Path()}, nil
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
