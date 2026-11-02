// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

// An ingest writes into a working file and publishes by renaming it over the
// real one, so a killed process leaves that file in the prepared folder and
// nothing looked for it afterwards. The startup pass clears them, in the
// background, before anything can be indexing - the application is
// single-instance, so every such file at that moment is abandoned.
func TestStartupSweepClearsAbandonedIngestTemps(t *testing.T) {
	reg, e, _ := preparedForServer(t, "dsl")
	dir, ok := store.LookupDir(e.Path)
	if !ok {
		t.Fatal("the fixture prepared no folder")
	}
	abandoned := filepath.Join(dir, store.TextDBName+".ingest.deadbeef")
	if err := os.WriteFile(abandoned, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(dir, store.TextDBName+".ingest.deadbeef-articles")
	if err := os.WriteFile(sibling, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(dir, store.TextDBName)
	before, err := os.Stat(kept)
	if err != nil {
		t.Fatal(err)
	}

	reg.startTempSweep()
	reg.jobs.wg.Wait() // the pass is a goroutine; this is how a test waits for it

	for _, gone := range []string{abandoned, sibling} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("the abandoned %s was not cleared", filepath.Base(gone))
		}
	}
	after, err := os.Stat(kept)
	if err != nil {
		t.Fatalf("the published database was removed: %v", err)
	}
	if after.Size() != before.Size() {
		t.Errorf("the published database changed: %d bytes, was %d", after.Size(), before.Size())
	}
}
