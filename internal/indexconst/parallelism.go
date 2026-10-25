// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package indexconst

// IndexDictionaryLimit leaves one logical processor for the rest of the app
// when possible and budgets a reader, writer, and two preparation workers per
// dictionary. Small machines still admit one dictionary.
func IndexDictionaryLimit(logical int) int {
	usable := max(1, logical-1)
	return max(1, usable/4)
}

// IngestWorkerLimit shares the processors left after active readers and
// writers among all dictionaries. The writer prepares work itself when no
// processor lane remains, which also covers single-core devices.
func IngestWorkerLimit(logical, activeDictionaries int) int {
	usable := max(1, logical-1)
	return max(0, usable-2*max(0, activeDictionaries))
}
