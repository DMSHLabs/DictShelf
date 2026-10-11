// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

// maskIndexSpans narrows a literal-leading mask to the relevant slices of
// idx_entry_w. The mask regexp uses Unicode case folding while SQLite NOCASE
// folds only ASCII, so the non-ASCII tail must also be scanned: for example,
// a leading K can match the Kelvin sign. Each span remains in browse order.
// A wildcard or non-ASCII lead conservatively scans the whole index.
type maskIndexSpan struct {
	where string
	args  []any
}

func maskIndexSpans(pattern string) []maskIndexSpan {
	if len(pattern) == 0 || pattern[0] == '*' || pattern[0] == '?' || pattern[0] >= 0x7f {
		return []maskIndexSpan{{}}
	}
	lead := pattern[0]
	if 'A' <= lead && lead <= 'Z' {
		lead += 'a' - 'A'
	}
	return []maskIndexSpan{
		{where: " AND w >= ? COLLATE NOCASE AND w < ? COLLATE NOCASE", args: []any{string(lead), string(lead + 1)}},
		{where: " AND w >= ? COLLATE NOCASE", args: []any{"\x80"}},
	}
}
