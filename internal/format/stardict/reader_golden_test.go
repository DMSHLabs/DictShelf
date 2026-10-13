// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package stardict

import (
	"testing"

	"github.com/wuweidict/wudict/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=1 ingest=3 markup=2 fold=1",
	Hash:     "2c39882f914820994ec8280ad77afb4e4c49c849bc882dbe08d559bedd441de2",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "stardict", buildStarDict(t, false), readerGolden)
}
