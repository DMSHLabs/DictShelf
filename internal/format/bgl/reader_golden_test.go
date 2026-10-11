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
	Versions: "reader=1 ingest=1 markup=2",
	Hash:     "9a1978f9bd48ed862b94048c485d6397a9e69153e5b73498d4be0e032d45d9bd",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "bgl", writeBGL(t, t.TempDir()), readerGolden)
}
