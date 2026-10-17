// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
)

const (
	wildcardCacheMemoryLimit = 20_000
	wildcardCacheMaxAge      = 30 * time.Minute
	wildcardCacheDirName     = "wildcards"
)

var wildcardCacheIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,80}$`)

type wildcardCacheSignature struct {
	dictionary string
	query      string
	within     string
	wildcards  bool
	withinMask bool
}

type wildcardCachePage struct {
	Words     []string      `json:"words"`
	Positions []int         `json:"positions,omitempty"`
	Alphabet  []dict.Letter `json:"alphabet,omitempty"`
}

type wildcardCacheEntry struct {
	mu        sync.Mutex
	dir       string
	signature wildcardCacheSignature
	pageSize  int
	updated   time.Time
	active    bool
	complete  bool
	total     int
	alphabet  []dict.Letter

	pendingWords     []string
	pendingPositions []int
	memoryPages      []wildcardCachePage

	file    *os.File
	offsets []int64
	fileEnd int64
	failed  error
}

type wildcardPageCache struct {
	mu          sync.Mutex
	dir         string
	entries     map[string]*wildcardCacheEntry
	cleanedTemp bool
}

func (s *Server) wildcardCacheDir() string {
	if s.User != "" {
		return filepath.Join(string(s.User), "cache", wildcardCacheDirName)
	}
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "wudict", wildcardCacheDirName)
	}
	return filepath.Join(os.TempDir(), "wudict", wildcardCacheDirName)
}

func (c *wildcardPageCache) init(dir string) {
	c.mu.Lock()
	c.dir = dir
	c.cleanupLocked(time.Now())
	c.mu.Unlock()
}

func (c *wildcardPageCache) begin(id string, signature wildcardCacheSignature, pageSize int, dir string) (*wildcardCacheEntry, error) {
	if !wildcardCacheIDPattern.MatchString(id) {
		return nil, errors.New("invalid wildcard cache id")
	}
	if pageSize < 1 {
		return nil, errors.New("invalid wildcard cache block size")
	}
	now := time.Now()
	c.mu.Lock()
	c.dir = dir
	c.cleanupLocked(now)
	if c.entries == nil {
		c.entries = make(map[string]*wildcardCacheEntry)
	}
	if old := c.entries[id]; old != nil {
		old.mu.Lock()
		old.closeAndRemoveLocked()
		old.mu.Unlock()
	}
	entry := &wildcardCacheEntry{dir: dir, signature: signature, pageSize: pageSize, updated: now, active: true}
	c.entries[id] = entry
	c.mu.Unlock()
	return entry, nil
}

func (c *wildcardPageCache) page(id string, signature wildcardCacheSignature, page int) (browseFindResp, bool, error) {
	if !wildcardCacheIDPattern.MatchString(id) || page < 1 {
		return browseFindResp{}, false, nil
	}
	c.mu.Lock()
	c.cleanupLocked(time.Now())
	entry := c.entries[id]
	c.mu.Unlock()
	if entry == nil {
		return browseFindResp{}, false, nil
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.signature != signature || entry.failed != nil {
		return browseFindResp{}, false, nil
	}
	entry.updated = time.Now()
	var record wildcardCachePage
	if page <= len(entry.offsets) {
		if entry.file == nil {
			if page > len(entry.memoryPages) {
				return browseFindResp{}, false, nil
			}
			record = entry.memoryPages[page-1]
		} else {
			var err error
			record, err = entry.readPageLocked(page)
			if err != nil {
				return browseFindResp{}, false, err
			}
		}
	} else if page == len(entry.offsets)+1 && len(entry.pendingWords) > 0 {
		record = wildcardCachePage{Words: append([]string(nil), entry.pendingWords...),
			Positions: append([]int(nil), entry.pendingPositions...)}
	} else {
		return browseFindResp{}, false, nil
	}
	pages := (entry.total + entry.pageSize - 1) / entry.pageSize
	return browseFindResp{Words: record.Words, Positions: record.Positions, Alphabet: entry.alphabet,
		Total: entry.total, Page: page, Pages: pages, Size: entry.pageSize, Partial: !entry.complete}, true, nil
}

func (e *wildcardCacheEntry) append(event browseFindStreamEvent) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return false, e.failed
	}
	e.updated = time.Now()
	e.total = event.Total
	if event.Done {
		e.complete = true
		e.alphabet = append([]dict.Letter(nil), event.Alphabet...)
	}
	if len(event.Words) > 0 {
		e.pendingWords = append(e.pendingWords, event.Words...)
		e.pendingPositions = append(e.pendingPositions, event.Positions...)
	}
	for len(e.pendingWords) >= e.pageSize {
		if err := e.commitPageLocked(e.pageSize); err != nil {
			e.failLocked(err)
			return false, err
		}
	}
	if event.Done && len(e.pendingWords) > 0 {
		if err := e.commitPageLocked(len(e.pendingWords)); err != nil {
			e.failLocked(err)
			return false, err
		}
	}
	if e.total > wildcardCacheMemoryLimit && e.file == nil {
		if err := e.spillLocked(); err != nil {
			e.failLocked(err)
			return false, err
		}
	}
	return e.file != nil, nil
}

func (e *wildcardCacheEntry) commitPageLocked(count int) error {
	if count < 1 || count > len(e.pendingWords) {
		return errors.New("invalid wildcard cache block")
	}
	page := wildcardCachePage{Words: append([]string(nil), e.pendingWords[:count]...)}
	if len(e.pendingPositions) >= count {
		page.Positions = append([]int(nil), e.pendingPositions[:count]...)
	}
	e.pendingWords = append(e.pendingWords[:0], e.pendingWords[count:]...)
	if len(e.pendingPositions) >= count {
		e.pendingPositions = append(e.pendingPositions[:0], e.pendingPositions[count:]...)
	} else {
		e.pendingPositions = nil
	}
	if e.file == nil {
		e.memoryPages = append(e.memoryPages, page)
		return nil
	}
	return e.writePageLocked(page)
}

func (e *wildcardCacheEntry) spillLocked() error {
	if err := os.MkdirAll(e.dir, 0o700); err != nil {
		return fmt.Errorf("create wildcard cache directory: %w", err)
	}
	file, err := os.CreateTemp(e.dir, "wildcards-*.cache")
	if err != nil {
		return fmt.Errorf("create wildcard cache file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return fmt.Errorf("secure wildcard cache file: %w", err)
	}
	e.file = file
	for _, page := range e.memoryPages {
		if err := e.writePageLocked(page); err != nil {
			return fmt.Errorf("write wildcard cache file: %w", err)
		}
	}
	e.memoryPages = nil
	return nil
}

func (e *wildcardCacheEntry) writePageLocked(page wildcardCachePage) error {
	data, err := json.Marshal(page)
	if err != nil {
		return err
	}
	if len(data) > int(^uint32(0)) {
		return errors.New("wildcard cache block is too large")
	}
	record := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(record[:4], uint32(len(data)))
	copy(record[4:], data)
	e.offsets = append(e.offsets, e.fileEnd)
	n, err := e.file.WriteAt(record, e.fileEnd)
	if err != nil {
		return err
	}
	if n != len(record) {
		return errors.New("short write to wildcard cache file")
	}
	e.fileEnd += int64(n)
	return nil
}

func (e *wildcardCacheEntry) readPageLocked(page int) (wildcardCachePage, error) {
	if page < 1 || page > len(e.offsets) || e.file == nil {
		return wildcardCachePage{}, errors.New("wildcard cache block is unavailable")
	}
	offset := e.offsets[page-1]
	var prefix [4]byte
	if _, err := e.file.ReadAt(prefix[:], offset); err != nil {
		return wildcardCachePage{}, err
	}
	size := binary.BigEndian.Uint32(prefix[:])
	if size > 64<<20 {
		return wildcardCachePage{}, errors.New("wildcard cache block exceeds limit")
	}
	data := make([]byte, size)
	if _, err := e.file.ReadAt(data, offset+4); err != nil {
		return wildcardCachePage{}, err
	}
	var record wildcardCachePage
	if err := json.Unmarshal(data, &record); err != nil {
		return wildcardCachePage{}, err
	}
	return record, nil
}

func (e *wildcardCacheEntry) closeAndRemoveLocked() {
	if e.file == nil {
		return
	}
	name := e.file.Name()
	_ = e.file.Close()
	_ = os.Remove(name)
	e.file = nil
}

func (e *wildcardCacheEntry) failLocked(err error) {
	e.failed = err
	e.closeAndRemoveLocked()
	e.memoryPages = nil
	e.pendingWords = nil
	e.pendingPositions = nil
}

func (c *wildcardPageCache) finish(id string, entry *wildcardCacheEntry) {
	entry.mu.Lock()
	entry.active = false
	entry.updated = time.Now()
	memoryOnly := entry.file == nil
	if memoryOnly {
		entry.memoryPages = nil
		entry.pendingWords = nil
		entry.pendingPositions = nil
	}
	entry.mu.Unlock()
	if !memoryOnly {
		return
	}
	c.mu.Lock()
	if c.entries[id] == entry {
		delete(c.entries, id)
	}
	c.mu.Unlock()
}

func (c *wildcardPageCache) cleanupLocked(now time.Time) {
	if c.entries == nil {
		c.entries = make(map[string]*wildcardCacheEntry)
	}
	for id, entry := range c.entries {
		entry.mu.Lock()
		expired := !entry.active && now.Sub(entry.updated) > wildcardCacheMaxAge
		if expired {
			entry.closeAndRemoveLocked()
			delete(c.entries, id)
		}
		entry.mu.Unlock()
	}
	if c.cleanedTemp {
		return
	}
	c.cleanedTemp = true
	dir := c.dir
	if dir == "" {
		return
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, item := range items {
		if item.IsDir() || !strings.HasPrefix(item.Name(), "wildcards-") || !strings.HasSuffix(item.Name(), ".cache") {
			continue
		}
		info, err := item.Info()
		if err == nil && now.Sub(info.ModTime()) > wildcardCacheMaxAge {
			_ = os.Remove(filepath.Join(dir, item.Name()))
		}
	}
}
