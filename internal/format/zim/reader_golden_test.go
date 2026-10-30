// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=4 ingest=7 markup=2 fold=1",
	Hash:     "32cca3dca255e419d77fdb614390ea905f2a97d73f99ff5918071242beb112d8",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "zim", writeZIM(t, sample()), readerGolden)
}
