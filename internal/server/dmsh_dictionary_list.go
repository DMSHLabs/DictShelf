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
	"time"

	"github.com/DMSHLabs/DictShelf/internal/facet"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
	"github.com/DMSHLabs/DictShelf/internal/logx"
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
	listStarted := time.Now()
	if emit == nil {
		return dictionaryListResult{}, errors.New("dictionary list event callback is required")
	}
	entries := s.reg.all()
	groups := s.groupsNow()
	problems := facet.CountProblems(groups.problems)
	global := s.rowsGlobal(groups)
	// A validator has to be judged over the WHOLE list before a byte of the body
	// (a 304 carries none). Without one there is nothing to decide up front, so
	// the per-row keys are derived while the list streams: the first row leaves
	// at once instead of after every dictionary has been stat-ed. That up-front
	// pass is what showed "checking ... N of M" and held the first row back for
	// seconds on a phone's FUSE mount - and a Browse word click is a page load
	// that waits for that first row before it can search.
	_ = showProgress
	keys := make([]string, len(entries))
	if ifNoneMatch != "" {
		keys = rowKeys(entries, global)
		if matchesTag(ifNoneMatch, s.listTag(entries, keys, problems)) {
			return dictionaryListResult{notModified: true}, nil
		}
	}
	ids := make(map[string]bool, len(entries))
	for _, e := range entries {
		ids[e.ID] = true
	}
	s.dictRows.keep(ids)

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

	workers := search.Workers()
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var keepable atomic.Bool
	var rowWorkNanos atomic.Int64
	var firstRowNanos atomic.Int64
	keepable.Store(true)
	rowPhaseStarted := time.Now()
	for i, e := range entries {
		wg.Add(1)
		go func(i int, e *entry) {
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
			key := keys[i]
			if key == "" {
				key = rowKey(e, global)
				keys[i] = key
			}
			rowStarted := time.Now()
			info, held := s.dictInfoFor(e, global, key)
			noteArchiveFullText(&info)
			s.attachEstimate(&info)
			rowWorkNanos.Add(time.Since(rowStarted).Nanoseconds())
			firstRowNanos.CompareAndSwap(0, time.Since(rowPhaseStarted).Nanoseconds())
			if !held || info.Error != "" || info.Job != nil {
				keepable.Store(false)
			}
			_ = write(dictMsg{T: "dict", Dict: &info})
		}(i, e)
	}
	wg.Wait()
	rowWall := time.Since(rowPhaseStarted)
	emitMu.Lock()
	defer emitMu.Unlock()
	if emitErr != nil {
		return dictionaryListResult{}, emitErr
	}
	end := dictMsg{T: "end"}
	if ctx.Err() == nil && keepable.Load() {
		// Every key is in by now, so the validator describes the list that was
		// streamed. It rides on the end message for the next request, which may
		// carry it (the web path does; the Android bridge has no headers).
		end.ETag = s.listTag(entries, keys, problems)
	}
	// Free space rides on the list rather than on a route of its own: it is one
	// number, the page that needs it already has this response in hand, and a
	// separate call would only add a way for the two to disagree.
	freeSpaceStarted := time.Now()
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
	firstRow := firstRowNanos.Load()
	logx.SystemStartupTimingOnce("dictionary_list",
		"stage=dictionary_list_backend dictionaries=%d workers=%d prepare_ms=%d first_row_ms=%d row_wall_ms=%d row_work_sum_ms=%d free_space_ms=%d total_ms=%d",
		len(entries), workers, rowPhaseStarted.Sub(listStarted).Milliseconds(), time.Duration(firstRow).Milliseconds(),
		rowWall.Milliseconds(), time.Duration(rowWorkNanos.Load()).Milliseconds(),
		time.Since(freeSpaceStarted).Milliseconds(), time.Since(listStarted).Milliseconds())
	logx.FlushSystemStartupTimings()
	return dictionaryListResult{}, nil
}
