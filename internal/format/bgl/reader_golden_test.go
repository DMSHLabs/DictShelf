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
	Versions: "reader=1 ingest=9 markup=2 fold=1",
	Hash:     "0de57d1971125f028c2dc5807c9b7ef8f8564615717833967809bd8e2ff5c95b",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "bgl", writeBGL(t, t.TempDir()), readerGolden)
}
