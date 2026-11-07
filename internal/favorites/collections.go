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

const collectionCatalogVersion = 2

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

// Collections owns the available stores. Every collection gets a directory
// directly below the favourites root; "fav" is reserved for the default one.
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
	if ids == nil {
		ids = []string{ID}
	}
	if len(ids) == 0 {
		ids = []string{ID}
		if err := c.saveCatalog(ids); err != nil {
			return nil, err
		}
	}
	c.ids = append([]string(nil), ids...)
	for _, id := range ids {
		if (id != ID && !validCollectionID(id)) || c.stores[id] != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidCollection, id)
		}
		var store *Store
		if id == ID {
			store = NewAt(filepath.Join(root, "fav"), "favourites.favs")
		} else {
			store = NewAt(filepath.Join(root, id), id+".favs")
		}
		if err := store.EnsureNamed(defaultName); err != nil {
			return nil, fmt.Errorf("open favourites collection %q: %w", id, err)
		}
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
	if err := json.Unmarshal(data, &catalog); err != nil || (catalog.Version != 1 && catalog.Version != collectionCatalogVersion) || catalog.IDs == nil {
		return nil, fmt.Errorf("%w: cannot read collection catalogue", ErrInvalidCollection)
	}
	if catalog.Version == 1 {
		return append([]string{ID}, catalog.IDs...), nil
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
		m, err := set.currentManifest()
		if err != nil {
			return nil, err
		}
		name := m.Name
		if name == "" {
			name = "Favourites"
		}
		out = append(out, CollectionInfo{ID: id, Name: name, About: m.About, Path: set.Path(), Articles: len(m.Articles), Size: m.TotalSize})
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
	data := filepath.Join(c.root, collectionDirectory(id))
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
	if (id != ID && !validCollectionID(id)) || c.stores[id] == nil {
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
	set := c.stores[id]
	if err := set.SetName(name); err != nil {
		return CollectionInfo{}, err
	}
	m, err := set.currentManifest()
	if err != nil {
		return CollectionInfo{}, err
	}
	return CollectionInfo{ID: id, Name: name, About: m.About, Path: set.Path(), Articles: len(m.Articles), Size: m.TotalSize}, nil
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
	m, err := set.currentManifest()
	if err != nil {
		return CollectionInfo{}, err
	}
	return CollectionInfo{ID: id, Name: m.Name, About: m.About, Path: set.Path(), Articles: len(m.Articles), Size: m.TotalSize}, nil
}

func (c *Collections) Store(id string) (*Store, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	store, ok := c.stores[id]
	return store, ok
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
	root := filepath.Join(c.root, collectionDirectory(id))
	store := NewAt(root, id+".favs")
	if err := store.EnsureNamed(name); err != nil {
		return CollectionInfo{}, err
	}
	next := append(append([]string(nil), c.ids...), id)
	if err := c.saveCatalog(next); err != nil {
		_ = os.RemoveAll(root)
		return CollectionInfo{}, err
	}
	c.ids, c.stores[id] = next, store
	return CollectionInfo{ID: id, Name: name, Path: store.Path()}, nil
}

func collectionDirectory(id string) string {
	if id == ID {
		return "fav"
	}
	return id
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
