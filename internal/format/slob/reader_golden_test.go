// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package slob

import (
	"testing"

	"github.com/wuweidict/wudict/internal/store/goldentest"
)

// The prepared content of this package's fixture, pinned. A failure here
// means what an ingest writes has changed: bump ReaderVersion (or the version
// of whichever layer changed) so existing libraries are told to rebuild, then
// update the golden to the value the failure prints.
var readerGolden = goldentest.Golden{
	Versions: "reader=1 ingest=7 markup=2 fold=1",
	Hash:     "1760a3f52c7e8daf5b66863ad0df22ca0c023300333c249e37747635a094beab",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "slob", buildSlob(t), readerGolden)
}
