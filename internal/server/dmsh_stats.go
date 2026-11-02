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

// errMeasurementDeferred says a measurement did not happen because a writer owns
// the dictionary's files right now: not a failure, and nothing was published.
// The write that took them schedules the measurement when it is done
// (measureLater), so the dictionary is not left without numbers.
var errMeasurementDeferred = errors.New("the dictionary is being written; measure later")

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
				if errors.Is(err, errMeasurementDeferred) {
					logx.V("stats: %s is being written; leaving it for the writer to schedule", e.Path)
					return
				}
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
	// The index sizes are a page walk of the database (dbstat) - minutes on a
	// large one - and NOTHING has to pay it any more: the build measures them on
	// the private file it is writing and records them in the database itself
	// (store.IndexSizesRecorded). The walk stays for a library prepared before
	// that, where the numbers are simply unknown until it runs once.
	//
	// It yields while it runs (entry.walk): a writer of the same file cancels it
	// rather than waiting for it, and claims the files until it is done. A walk
	// that did not run, or was cancelled, has NOT measured anything - so nothing
	// is published, and the writer that interrupted it schedules it again
	// (measureLater). Publishing a zero here would be worse than publishing
	// nothing: the cache would answer as measured, and the dictionary would show
	// empty indexes for ever.
	if !store.IndexSizesRecorded(prepared.Meta) {
		ctx, ok := e.walk.begin()
		if !ok {
			return store.Stats{}, errMeasurementDeferred
		}
		s.ContainsIndexBytes, s.FullTextIndexBytes = store.MeasureIndexSizesContext(ctx, prepared.TextDB, prepared.Plan)
		cancelled := ctx.Err() != nil
		e.walk.end()
		if cancelled {
			return store.Stats{}, errMeasurementDeferred
		}
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

// measureLater schedules the measurement a writer interrupted, in the
// background.
//
// The walk yields to a writer (entry.walk), and a walk that gave up leaves the
// dictionary unmeasured; the pass that would have measured it runs once per
// process, so without this the numbers would be missing until the next start -
// which is the thing the fence exists to avoid. It runs off the request path, on
// the file as the writer left it.
func (r *Registry) measureLater(e *entry) {
	go func() {
		if !r.statsNeeded(e) {
			return
		}
		if _, err := r.measureStats(e); err != nil && !errors.Is(err, errMeasurementDeferred) {
			logx.V("stats: could not measure %s: %v", e.Path, err)
		}
	}()
}

func isZIMFormat(format string) bool { return format == "zim" }
