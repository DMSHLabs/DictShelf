// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/search"
)

// streamSimpleWildcardCache materializes masks containing only '*' and '?'
// from the prepared Browse pages into the same 300-word SQLite blocks used by
// every other selective Wildcards search.
func (s *Server) streamSimpleWildcardCache(ctx context.Context, request browseFindStreamRequest, send func(browseFindStreamEvent) error) error {
	browser, ok := request.dict.(dict.Browser)
	if !ok {
		return errors.New("dictionary does not support Browse paging")
	}
	matcher, err := search.CompileMask(request.query)
	if err != nil {
		return err
	}
	entry, token, err := s.wildcardPages.begin(request.cacheID, wildcardCacheSignature{
		query: request.query, group: request.cacheGroup, wildcards: request.mask,
		fts: request.cacheFTS, trigram: request.cacheTrigram,
	}, browsePageSize, request.cacheCount, request.entry.ID, request.entry.probeName(), request.cacheOrder,
		"", false, s.wildcardCacheDir(), request.diagnostics)
	if err != nil {
		return err
	}
	key := wildcardStreamKey{dictID: request.entry.ID}
	defer s.wildcardPages.finish(entry, &key, token)
	letters, err := browser.Alphabet()
	if err != nil {
		return err
	}
	scanTotal := 0
	for _, letter := range letters {
		scanTotal += letter.Count
	}
	var sections dict.MaskSections
	words := make([]string, 0, browsePageSize)
	positions := make([]int, 0, browsePageSize)
	found, scanned := 0, 0
	nextProgress := time.Now().Add(250 * time.Millisecond)
	streamBatchSize := browsePageSize * 10
	emit := func(done bool) error {
		event := browseFindStreamEvent{Type: "progress", Words: words, Positions: positions,
			Total: found, Scanned: scanned, ScanTotal: scanTotal, Size: browsePageSize, DiskCache: true, Done: done}
		if len(words) > 0 {
			event.Type = "block"
		}
		if done {
			event.Type = "done"
			event.Alphabet = sections.Finish()
		}
		if err := entry.append(request.entry.ID, "", false, token, event); err != nil {
			return err
		}
		words = make([]string, 0, browsePageSize)
		positions = make([]int, 0, browsePageSize)
		return send(event)
	}
	for offset := 0; offset < scanTotal; offset += browsePageSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := browser.Page(offset, min(browsePageSize, scanTotal-offset))
		if err != nil {
			return err
		}
		for _, word := range page {
			scanned++
			if matcher.MatchString(word) {
				words = append(words, word)
				positions = append(positions, found)
				sections.Add(word)
				found++
				if found == 1 || len(words) >= streamBatchSize {
					if err := emit(false); err != nil {
						return err
					}
				}
			}
		}
		if len(words) >= streamBatchSize || time.Now().After(nextProgress) {
			if err := emit(false); err != nil {
				return err
			}
			nextProgress = time.Now().Add(250 * time.Millisecond)
		}
	}
	return emit(true)
}
