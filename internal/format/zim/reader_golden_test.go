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
	Versions: "reader=4 ingest=8 markup=2 fold=1",
	Hash:     "ef4db5e3a4d207fbc4fc1d2384f559bb9c14a184d7efca43e09953450f2ce421",
}

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, "zim", writeZIM(t, sample()), readerGolden)
}
