// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=12 ingest=8 markup=2 fold=1",
	Hash:     "158c000acfb04420b31b5c20d2f2ed9a2793b3239d5d643d7c09e5ccdcba491d",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "dsl", writeDSL(t, "mini.dsl", []byte(sampleDSL)), readerGolden)
}
