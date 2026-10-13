// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"testing"

	"github.com/wuweidict/wudict/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=3 ingest=3 markup=2 fold=1",
	Hash:     "87cdcb75fb2909fe1016573f12f96e0234f370b7228db926864926037e6ce3ec",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "zim", writeZIM(t, sample()), readerGolden)
}
