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
	Versions: "reader=1 ingest=5 markup=2 fold=1",
	Hash:     "dd98e64268290ad834ba5abc042b97a9832853d7db2c9b3bc5d239307dc28299",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "bgl", writeBGL(t, t.TempDir()), readerGolden)
}
