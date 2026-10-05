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
	Versions: "reader=9 ingest=1 markup=2 fold=1",
	Hash:     "3cfd14e62274c1efcc0972ede5373231681d255b1c0642720306060011581355",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "dsl", writeDSL(t, "mini.dsl", []byte(sampleDSL)), readerGolden)
}
