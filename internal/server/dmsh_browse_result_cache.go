// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"fmt"
	"os"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/indexconst"
)

// Browse result blocks share the bounded, on-disk result store used by
// Wildcards. The signature includes the prepared database's file identity so
// a rebuilt dictionary cannot serve blocks from an older index.
func browseResultSignature(e *entry, query, mode string) (wildcardCacheSignature, error) {
	path := backingDB(e.Path)
	if path == "" {
		return wildcardCacheSignature{}, fmt.Errorf("Browse index is unavailable")
	}
	info, err := os.Stat(path)
	if err != nil {
		return wildcardCacheSignature{}, err
	}
	return wildcardCacheSignature{
		query: query,
		group: fmt.Sprintf("browse-v1:%s:%s:%s:%d:%d", e.ID, mode, path, info.Size(), info.ModTime().UnixNano()),
	}, nil
}

// streamBrowseResultCache keeps title blocks in SQLite while sending only
// progress and one ribbon marker per block to the WebView. Article bodies are
// never stored in this cache.
func (s *Server) streamBrowseResultCache(ctx context.Context, request browseFindStreamRequest,
	send func(browseFindStreamEvent) error) error {
	signature, err := browseResultSignature(request.entry, request.query, request.matchMode)
	if err != nil {
		return err
	}
	const pageSize = indexconst.BrowsePageSize
	entry, token, err := s.wildcardPages.begin(request.browseCacheID, signature, pageSize, 1,
		request.entry.ID, request.dict.Meta().Name, 0, "", false, s.wildcardCacheDir())
	if err != nil {
		return err
	}
	key := wildcardStreamKey{dictID: request.entry.ID}
	defer s.wildcardPages.finish(entry, &key, token)
	return s.streamBrowseFind(ctx, request, func(event browseFindStreamEvent) error {
		start := event.Total - len(event.Words)
		if start < 0 {
			return fmt.Errorf("Browse result block has invalid offset")
		}
		cached := event
		cached.Alphabet = nil
		cached.Positions = nil
		markers := make([]dict.Letter, 0, 2)
		for i, word := range event.Words {
			position := start + i
			if position%pageSize == 0 {
				markers = append(markers, dict.Letter{Letter: word, Offset: position, Count: pageSize})
			}
		}
		if err := entry.append(request.entry.ID, "", false, token, cached); err != nil {
			return err
		}
		event.Words = nil
		event.Positions = nil
		event.Alphabet = markers
		event.DiskCache = true
		event.Size = pageSize
		return send(event)
	})
}
