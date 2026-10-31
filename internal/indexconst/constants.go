// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Package indexconst holds shared sizes and intervals for prepared indexes and their views.
package indexconst

import "time"

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

// The two search clocks. They are separate numbers because they answer separate
// questions - a fan-out over the whole library, and one dictionary the reader
// named - and they live here rather than inline in the handlers so the places
// that use each cannot drift apart.
const (
	// SearchFanoutBudget bounds ONE search of many dictionaries. It exists to stop
	// a single slow dictionary from holding a wide fan-out open.
	//
	// It is a REPORTING bound, not a resource bound: the fan-out shares this one
	// clock, so every dictionary still unanswered at the end reports a timeout -
	// including a dictionary that is merely not indexed, which is why the reader
	// is told "index this dictionary" and not "too slow". The real limits are
	// expressed in their own currency elsewhere: bytes for a search's memory
	// (SEARCH_MEMORY, PREVIEW_MEMORY, the fan-out budget) and lanes for indexing
	// work (IndexDictionaryLimit, the search fan-out width).
	SearchFanoutBudget = 30 * time.Second

	// SingleSearchBudget is how long a search of ONE named dictionary may take
	// before it is cut off. Generous rather than unlimited: the request still
	// holds a goroutine and an open backend, so it needs an end, but the end is
	// there to catch a wedged backend, not to police a slow one.
	SingleSearchBudget = 5 * time.Minute
)
