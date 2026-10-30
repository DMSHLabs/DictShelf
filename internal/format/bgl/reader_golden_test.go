// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package bgl

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
	Hash:     "aab41291f9153226d4aff0fda9aed94f7ca251744bab3df3697143f0e281c2f5",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "bgl", writeBGL(t, t.TempDir()), readerGolden)
}
