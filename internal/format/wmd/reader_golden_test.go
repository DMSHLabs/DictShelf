// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

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
	Hash:     "346704d0f481cdb117aa66426d478611db7d5a5e5d745159c0a8537d0177e1ad",
}

const goldenDoc = `# Golden
wudict: 1
from: en
to: en
author: fixture

A fixture for the reader golden.

## run
## runs

*v.* to move quickly: [walk](entry://walk), [C#](entry://C%23)

| form | tense |
| --- | :-: |
| ran | past |

## ran
see: run

## gone
see: nowhere

## html

<div class="entry"><span class="pos">n.</span> text</div>
`

func TestReaderGolden(t *testing.T) {
	goldentest.Check(t, Format, writeTemp(t, "golden"+Ext, goldenDoc), readerGolden)
}
