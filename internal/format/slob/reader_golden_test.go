// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package slob

import (
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=1 ingest=8 markup=2 fold=1",
	Hash:     "2f7061f0e54a516e90a43069e3742dcd832dfdce1f299e476a2a0156f6eb30c5",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "slob", buildSlob(t), readerGolden)
}
