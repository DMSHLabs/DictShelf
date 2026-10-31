// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package search

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

var errSuggestionLimit = errors.New("suggestion candidate limit reached")

type suggestionUpdate struct {
	index int
	meta  dict.Meta
	word  string
	err   error
}

// StreamHeadwordSuggestions forwards each dictionary's first matching headword
// immediately and replaces that partial list as more candidates arrive. The
// UI ranks the cumulative candidates; article bodies are never loaded here.
func StreamHeadwordSuggestions(ctx context.Context, openers []Opener, term string, limit int, emit func(int, Hit)) {
	sem := make(chan struct{}, Workers())
	var workers sync.WaitGroup
	updates := make(chan suggestionUpdate, 256)
	post := func(update suggestionUpdate) {
		select {
		case updates <- update:
		case <-ctx.Done():
		}
	}
	for i, open := range openers {
		workers.Add(1)
		go func(i int, open Opener) {
			defer workers.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if ctx.Err() != nil {
				return
			}
			d, err := safeOpen(open)
			if err != nil {
				if ctx.Err() == nil {
					post(suggestionUpdate{index: i, err: err})
				}
				return
			}
			var meta dict.Meta
			defer func() {
				if failure := recover(); failure != nil && ctx.Err() == nil {
					post(suggestionUpdate{index: i, meta: meta, err: dict.PanicError(meta.Path, failure)})
				}
			}()
			meta = d.Meta()
			streamer, ok := d.(interface {
				StreamBrowseSuggestions(context.Context, string, func(string) error) error
			})
			if !ok {
				hit := query(ctx, d, Wildcards, term, nil, limit)
				if hit.Err != nil {
					post(suggestionUpdate{index: i, meta: hit.Meta, err: hit.Err})
				}
				for _, result := range hit.Results {
					post(suggestionUpdate{index: i, meta: hit.Meta, word: result.Headword})
				}
				return
			}
			count := 0
			err = streamer.StreamBrowseSuggestions(ctx, term, func(word string) error {
				if count >= limit {
					return errSuggestionLimit
				}
				count++
				post(suggestionUpdate{index: i, meta: meta, word: word})
				if count >= limit {
					return errSuggestionLimit
				}
				return ctx.Err()
			})
			if err != nil && !errors.Is(err, errSuggestionLimit) && ctx.Err() == nil {
				post(suggestionUpdate{index: i, meta: meta, err: err})
			}
		}(i, open)
	}
	go func() { workers.Wait(); close(updates) }()
	collected := make(map[int][]dict.Result, len(openers))
	dirty := make(map[int]bool)
	metas := make(map[int]dict.Meta, len(openers))
	var ticker *time.Ticker
	var ticks <-chan time.Time
	flush := func() {
		if ctx.Err() != nil {
			clear(dirty)
			return
		}
		for index := range dirty {
			results := append([]dict.Result(nil), collected[index]...)
			var meta dict.Meta
			if len(results) > 0 {
				// The latest update for each dictionary carries its own metadata.
				meta = metas[index]
			}
			emit(index, Hit{Meta: meta, Term: term, Results: results})
			delete(dirty, index)
		}
	}
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				flush()
				if ticker != nil {
					ticker.Stop()
				}
				return
			}
			if update.err != nil {
				emit(update.index, Hit{Meta: update.meta, Term: term, Err: update.err})
				continue
			}
			if _, exists := metas[update.index]; !exists {
				metas[update.index] = update.meta
			}
			collected[update.index] = append(collected[update.index], dict.Result{Headword: update.word})
			dirty[update.index] = true
			if ticker == nil {
				emit(update.index, Hit{Meta: update.meta, Term: term, Results: append([]dict.Result(nil), collected[update.index]...)})
				delete(dirty, update.index)
				ticker = time.NewTicker(250 * time.Millisecond)
				ticks = ticker.C
			}
		case <-ticks:
			flush()
		}
	}
}
