// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// searchOpen opens one dictionary for a search, refusing one whose index is not
// there yet.
//
// A dictionary with no index does not take part in search at all - that is the
// rule, and it is why "index this dictionary" is never advice a search gives
// (2026-11-03). What it replaced was a preview: an unprepared dictionary was
// opened from its own source and searched through the format's reader, bounded by
// a materialisation budget and reported as DEFERRED when the budget refused it.
// That path could take minutes on a large source, reported every dictionary of a
// wide search as timed out, and existed only because the app used to search
// before it indexed. The queue prepares every dictionary that is added; until it
// is prepared the dictionary answers nothing and says so (errIndexNotReady, which
// the client translates).
//
// Self-indexed formats are the exception and not a preview: their index is the
// archive's own (ZIM's title index), it needs nothing prepared, and searching
// them costs no more than searching a prepared one.
func (e *entry) searchOpen() (dict.Dictionary, error) {
	if !entryHasSelfIndex(e) {
		// Search asks this once per dictionary on every lookup. Prefer the
		// prepared folder's receipt so the hot path can validate the source
		// without opening text.db just to read its meta; keep the full check as
		// a fallback for older libraries whose receipt is incomplete.
		prepared := store.IsTextDB(e.Path)
		if !prepared {
			_, prepared = store.FindPreparedFromReceipt(e.Path)
			if !prepared {
				_, prepared = validPrepared(e.Path)
			}
		}
		if !prepared {
			return nil, errIndexNotReady
		}
	}
	return e.open()
}
