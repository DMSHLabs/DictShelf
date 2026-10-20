// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Package indexconst holds shared sizes and intervals for prepared indexes and their views.
package indexconst

const (
	// SearchDebounceMS is the idle interval after typing before Browse and result-filter searches.
	SearchDebounceMS = 500
	// BrowsePageSize is the default number of headwords shown on a Browse page.
	BrowsePageSize = 300
	// BrowsePageLastOffset is the last row of a Browse page relative to its start.
	BrowsePageLastOffset = BrowsePageSize - 1
	// BrowseAnchorStride stores one ordinal seek point per this many headword rows.
	BrowseAnchorStride = 100
	// BrowseStripExactWidthCount is the number of Browse strip labels measured exactly.
	BrowseStripExactWidthCount = 200
	// IngestBatchSize is the number of entries written per ingest batch.
	IngestBatchSize = 5000
)
