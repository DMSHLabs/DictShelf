// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Package indexconst holds shared sizes and intervals for prepared indexes and their views.
package indexconst

const (
	// SearchDebounceMS is the idle interval after typing before Browse and result-filter searches.
	SearchDebounceMS = 500
	// BrowsePageProgressDelayMS delays the page-loading indicator to avoid flashes on quick reads.
	BrowsePageProgressDelayMS = 500
	// BrowsePageSize is the default number of headwords shown on a Browse page.
	BrowsePageSize = 300
	// BrowsePageLastOffset is the last row of a Browse page relative to its start.
	BrowsePageLastOffset = BrowsePageSize - 1
	// BrowseAnchorStride stores one ordinal seek point per this many headword rows.
	BrowseAnchorStride = 100
	// WildcardCacheBlockSize keeps SQL result blocks aligned with Browse pages.
	WildcardCacheBlockSize = BrowsePageSize
	// ResultCacheLimit is the maximum number of recent SQL result caches on disk.
	ResultCacheLimit = 5
	// ResultCacheMaxAgeDays is the startup cleanup age for unused SQL result caches.
	ResultCacheMaxAgeDays = 30
	// ResultCacheAndroidDefaultGB is the default disk-cache limit on Android.
	ResultCacheAndroidDefaultGB = 16
	// ResultCacheWindowsDefaultGB is the default disk-cache limit on Windows.
	ResultCacheWindowsDefaultGB = 16
	// ResultCacheDesktopDefaultGB is the default disk-cache limit on desktop platforms.
	ResultCacheDesktopDefaultGB = 8
	// CombinedDictionaryLimitDefault is the default number of dictionaries allowed in a combined list.
	CombinedDictionaryLimitDefault = 500
	// CombinedDictionaryLimitMax bounds the configurable combined-list size.
	CombinedDictionaryLimitMax = 5000
	// BrowseStripExactWidthCount is the number of Browse strip labels measured exactly.
	BrowseStripExactWidthCount = 200
	// IngestBatchSize is the number of entries written per ingest batch.
	IngestBatchSize = 5000
)

// There are no search clocks. A search ends when the reader stops it or asks
// something else, and that is the whole of it: a clock cannot tell a slow disk
// from a wedged backend, and the one that measured the FAN-OUT measured how long
// a dictionary waited for its turn, so it failed dictionaries that were working -
// the user's 388-dictionary 'contains' search, where every row said "timed out"
// while the same log showed them indexed (2026-11-03).
//
// What replaces it, for the wedged case, is a watchdog on PROGRESS: where the
// loop is ours - headwords, article ids, archive records - a thread that stops
// moving for a long while is a bug or a corrupt file, and it is reported with the
// dictionary, its size and the record it stopped on. Where the call is opaque
// (one SQLite MATCH) there is no progress to watch, so there is no limit there
// either: the reader has Stop. Resource bounds stay where they belong and in
// their own currency: bytes for memory (PREVIEW_MEMORY) and lanes for indexing
// work (IndexDictionaryLimit, the search fan-out width).
