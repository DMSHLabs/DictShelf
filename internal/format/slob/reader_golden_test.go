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
	Versions: "reader=1 ingest=2 markup=2 fold=1",
	Hash:     "fc7d42572652fda4baa9e7a97ac54d38685480e2d699aef6f102db4685976cc0",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "slob", buildSlob(t), readerGolden)
}
