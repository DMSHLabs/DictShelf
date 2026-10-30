// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"time"

	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// ReportPrepared prints index outcome details, including counts, duration and
// unresolved redirects. Shared by indexing operations across formats.
func ReportPrepared(name string, rep Report, took time.Duration) {
	logx.ClearLine()
	logx.Status("%s%d entries indexed in %.1fs", logx.Dict(name), rep.Entries, took.Seconds())
	if rep.UnresolvedLinks > 0 {
		logx.V("%s%d link targets not found in the source (skipped)", logx.Dict(name), rep.UnresolvedLinks)
	}
}
