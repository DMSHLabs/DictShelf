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
	Versions: "reader=1 ingest=7 markup=2 fold=1",
	Hash:     "b29b895fcc799f2125ee2943df1395f9842806aabb804b212f45b2c057189c8d",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "stardict", buildStarDict(t, false), readerGolden)
}
