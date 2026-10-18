// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Package indexconst holds shared sizes and intervals used by prepared indexes.
package indexconst

const (
	// BrowsePageSize is the default number of headwords shown on a Browse page.
	BrowsePageSize = 300
	// BrowseAnchorStride stores one ordinal seek point per this many headword rows.
	BrowseAnchorStride = 100
	// IngestBatchSize is the number of entries written per ingest batch.
	IngestBatchSize = 5000
)
