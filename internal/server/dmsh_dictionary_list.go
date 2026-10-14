// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/wuweidict/wudict/internal/facet"
	"github.com/wuweidict/wudict/internal/search"
)

type dictionaryListResult struct {
	notModified bool
}

// listDictionaries resolves and emits the current list without depending on
// HTTP. Both the HTTP adapter and the Windows bridge use this operation.
// A matching validator returns before emitting any events.
func (s *Server) listDictionaries(parent context.Context, ifNoneMatch string, emit func(dictMsg) error) (dictionaryListResult, error) {
	if emit == nil {
		return dictionaryListResult{}, errors.New("dictionary list event callback is required")
	}
	entries := s.reg.all()
	groups := s.groupsNow()
	problems := facet.CountProblems(groups.problems)
	global := s.rowsGlobal(groups)
	keys := rowKeys(entries, global)
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
	if err := emit(end); err != nil {
		emitErr = err
		cancel()
		return dictionaryListResult{}, err
	}
	return dictionaryListResult{}, nil
}
