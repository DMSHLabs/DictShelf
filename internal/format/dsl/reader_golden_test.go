// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import (
	"testing"

	"github.com/wuweidict/wudict/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=5 ingest=1 markup=2",
	Hash:     "122de9fef6e7111ca73b458051198c23f323e5916e820f2507cc8c7bfdc36d19",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "dsl", writeDSL(t, "mini.dsl", []byte(sampleDSL)), readerGolden)
}
