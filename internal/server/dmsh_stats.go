// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"errors"
	"path/filepath"
	"sync"

	zimformat "github.com/DMSHLabs/DictShelf/internal/format/zim"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// warmStats caches the measurements a size estimate reads (store.StatsCacheName)
// for the prepared dictionaries that do not have a valid cache yet, so the
// estimate dialog opens with numbers instead of a progress bar.
//
// Best effort in every direction, because nothing else depends on it: a
// dictionary is measured and committed before the next one is looked at, so
// closing the application in the middle leaves the finished ones measured and
// the rest for the next start - the condition to do the work is the same as the
// condition to have done it, and no state has to remember where the pass got
// to. A dictionary that refuses to be measured is logged and skipped, not
// retried: the dialog will measure that one on demand and say so.
//
// Two workers, not four like Warm: this reads whole archive directories, and it
// has no deadline to meet. It runs once per process - a registry is warmed from
// several places as the application comes up, and this is one pass over the
// library, not one per caller. A dictionary that arrives later is measured when
// the dialog asks for it.
func (r *Registry) warmStatsStart(entries []*entry) { r.statsOnce.Do(func() { r.warmStats(entries) }) }

// warmStats measures them all.
func (r *Registry) warmStats(entries []*entry) {
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, e := range entries {
		if !r.statsNeeded(e) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(e *entry) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := r.measureStats(e); err != nil {
				logx.Warn("could not measure %s: %v", e.Path, err)
			}
		}(e)
	}
	wg.Wait()
}

// statsNeeded answers whether a dictionary has yet to be measured. It reads the
// cache, and ReadStats is what decides validity - a library that was rebuilt, a
// source that was re-downloaded and a measurement format that moved on are all
// "measure again" without any of them being an error.
func (r *Registry) statsNeeded(e *entry) bool {
	if e == nil || r.prefs.Off(e.ID, e.Path) {
		return false
	}
	e.ingestMu.Lock()
	prepared, ok := preparedInfo(e.Path)
	e.ingestMu.Unlock()
	if !ok || prepared.TextDB == "" {
		return false
	}
	_, cached := store.ReadStats(prepared.TextDB)
	return !cached
}

// measureStats builds one dictionary's measurements and writes them beside its
// database. What a build recorded in the meta is the measurement of record;
// what that leaves open is measured here, and only what can be measured without
// decompressing anything: a ZIM's directory (article counts, text and media
// split, cluster bytes) and nothing else. The bodies of a dz/mdx dictionary
// cannot be weighed without inflating them, so those wait for the next rebuild
// - the estimate reads the file's size until then.
func (r *Registry) measureStats(e *entry) (store.Stats, error) {
	e.ingestMu.Lock()
	prepared, ok := preparedInfo(e.Path)
	e.ingestMu.Unlock()
	if !ok || prepared.TextDB == "" || prepared.Meta == nil {
		return store.Stats{}, errors.New("no prepared database")
	}
	s := store.StatsIdentity(prepared.Meta)
	if measured, ok := store.StatsFromMeta(prepared.Meta); ok {
		s = measured
	} else {
		s.ZIMForm = store.ZIMFormOf(prepared.Meta)
	}
	src := prepared.Meta["source_path"]
	if src == "" || !isZIMFormat(prepared.Meta["format"]) {
		// Nothing else can be measured without reading the dictionary itself.
		s.Version = store.StatsVersion
		return s, store.WriteStats(filepath.Dir(prepared.TextDB), s)
	}
	ds, err := zimformat.MeasureDirectory(src)
	if err != nil {
		// A source that is gone is not a defect: the dictionary was prepared
		// from it and the database still describes itself. The identity
		// measurements are worth caching on their own.
		logx.V("stats: not measuring %s: %v", src, err)
		s.Version = store.StatsVersion
		return s, store.WriteStats(filepath.Dir(prepared.TextDB), s)
	}
	s.Version = store.StatsVersion
	s.ZIMArticles = int64(ds.Articles)
	s.ZIMRedirects = int64(ds.Redirects)
	s.ZIMMedia = int64(ds.Media)
	s.ZIMOther = int64(ds.Other)
	s.ZIMClusters = int64(ds.Clusters)
	s.ZIMTextBytes = int64(ds.TextBytes)
	s.ZIMMixedBytes = int64(ds.MixedBytes)
	s.ZIMOtherBytes = int64(ds.OtherBytes)
	return s, store.WriteStats(filepath.Dir(prepared.TextDB), s)
}

func isZIMFormat(format string) bool { return format == "zim" }
