// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package bgl

import (
	"testing"

	"github.com/wuweidict/wudict/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=1 ingest=6 markup=2 fold=1",
	Hash:     "fb8a658f1ceb584264f4998eeb03c15fe94ee8337b44623b590d9a3ee9d0af27",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "bgl", writeBGL(t, t.TempDir()), readerGolden)
}
