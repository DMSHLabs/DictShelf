// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package stardict

import (
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=1 ingest=9 markup=2 fold=1",
	Hash:     "fe9d9a82426291e201f12ccd0bd21098a1cbd1e31e87806f14299f455895e34f",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "stardict", buildStarDict(t, false), readerGolden)
}
