// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/DMSHLabs/DictShelf/internal/facet"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
	"github.com/DMSHLabs/DictShelf/internal/search"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

type dictionaryListResult struct {
	notModified bool
}

// listDictionaries resolves and emits the current list without depending on
// HTTP. Both the HTTP adapter and the Windows bridge use this operation.
// A matching validator returns before emitting any events.
func (s *Server) listDictionaries(parent context.Context, ifNoneMatch string, emit func(dictMsg) error, showProgress ...bool) (dictionaryListResult, error) {
	if emit == nil {
		return dictionaryListResult{}, errors.New("dictionary list event callback is required")
	}
	entries := s.reg.all()
	groups := s.groupsNow()
	problems := facet.CountProblems(groups.problems)
	global := s.rowsGlobal(groups)
	wantProgress := len(showProgress) > 0 && showProgress[0] && ifNoneMatch == ""
	var progressErr error
	progress := func(done, total int) {
		if wantProgress && progressErr == nil {
			progressErr = emit(dictMsg{T: "checking", Done: done, Total: total})
		}
	}
	var keys []string
	if wantProgress {
		keys = rowKeysWithProgress(entries, global, progress)
	} else {
		keys = rowKeys(entries, global)
	}
	if progressErr != nil {
		return dictionaryListResult{}, progressErr
	}
	ids := make(map[string]bool, len(entries))
	for _, e := range entries {
		ids[e.ID] = true
	}
	s.dictRows.keep(ids)
	tag := s.listTag(entries, keys, problems)
	if ifNoneMatch != "" && matchesTag(ifNoneMatch, tag) {
		return dictionaryListResult{notModified: true}, nil
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var emitMu sync.Mutex
	var emitErr error
	write := func(message dictMsg) error {
		emitMu.Lock()
		defer emitMu.Unlock()
		if emitErr != nil {
			return emitErr
		}
		if err := emit(message); err != nil {
			emitErr = err
			cancel()
			return err
		}
		return nil
	}
	if err := write(dictMsg{T: "begin", Total: len(entries), GroupsProblems: problems}); err != nil {
		return dictionaryListResult{}, err
	}

	sem := make(chan struct{}, search.Workers())
	var wg sync.WaitGroup
	var keepable atomic.Bool
	keepable.Store(true)
	for i, e := range entries {
		wg.Add(1)
		go func(e *entry, key string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			info, held := s.dictInfoFor(e, global, key)
			noteArchiveFullText(&info)
			s.attachEstimate(&info)
			if !held || info.Error != "" || info.Job != nil {
				keepable.Store(false)
			}
			_ = write(dictMsg{T: "dict", Dict: &info})
		}(e, keys[i])
	}
	wg.Wait()
	emitMu.Lock()
	defer emitMu.Unlock()
	if emitErr != nil {
		return dictionaryListResult{}, emitErr
	}
	end := dictMsg{T: "end"}
	if ctx.Err() == nil && keepable.Load() {
		end.ETag = tag
	}
	// Free space rides on the list rather than on a route of its own: it is one
	// number, the page that needs it already has this response in hand, and a
	// separate call would only add a way for the two to disagree.
	if dir := store.DefaultDBDir(); dir != "" {
		if free, ok := fsx.FreeSpace(dir); ok {
			end.FreeBytes = free
			end.LibraryVolume = filepath.VolumeName(dir) + string(filepath.Separator)
		}
	}
	if err := emit(end); err != nil {
		emitErr = err
		cancel()
		return dictionaryListResult{}, err
	}
	return dictionaryListResult{}, nil
}
