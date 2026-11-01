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
//
// The hash below moved without a version bump, and that is deliberate: the only
// change is that the read-through reader now writes index_kind="articles" where
// it used to write nothing. A title-only database - the form every existing
// library holds, since the old code treated anything else as unusable - writes
// exactly what it wrote before, so no library has anything to rebuild. The
// absent marker only ever described the read-through form, which was previously
// refused; it is now a form in its own right.
var readerGolden = goldentest.Golden{
	Versions: "reader=4 ingest=7 markup=2 fold=1",
	Hash:     "f59c94310b9de115a3b699cbd48b7697b6ad5b2f1f723723c96d182c6f6b4434",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "zim", writeZIM(t, sample()), readerGolden)
}
