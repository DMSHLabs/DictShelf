// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// ingestTempInfix is what tells a working file from a database. tempDBName hangs
// it, with a random stem, off the name the file will be published under, so
// "<database>.ingest.<random>" - and the "-articles" and "-media" files that
// share that stem - are never a published state.
const ingestTempInfix = ".ingest."

// isIngestTemp reports whether a file name is one of those working files.
func isIngestTemp(name string) bool { return strings.Contains(name, ingestTempInfix) }

// SweepIngestTemps clears the working files an interrupted ingest left in a
// prepared folder, and reports how many went and how many bytes they held.
//
// An ingest writes into a temp name and publishes by renaming it over the real
// one, so a file carrying that infix is a build that never finished: a process
// that was killed, or a publish whose rename did not happen (on Windows a rename
// over a database another connection holds open fails outright). The ingest
// clears its own temp on its error paths; a process that is killed cannot, and
// nothing looked for the leftovers afterwards - one sat in this library for
// sixteen hours, 151 MB, beside a database that had been published within the
// hour.
//
// Safe whenever no ingest is in flight, which is why the caller runs it once, at
// startup: the application is single-instance (one mutex over the whole
// install), so nothing can be indexing yet. Nothing else ever reads a name with
// that infix either - isTextDB accepts text.db and *.text.db, and every walker
// uses the published names - so a file this leaves is a file no one can use.
func SweepIngestTemps(dir string) (removed int, freed int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, entry := range entries {
		if entry.IsDir() || !isIngestTemp(entry.Name()) {
			continue
		}
		var size int64
		if info, err := entry.Info(); err == nil {
			size = info.Size()
		}
		path := filepath.Join(dir, entry.Name())
		if err := os.Remove(path); err != nil {
			logx.V("ingest: could not clear the abandoned %s: %v", path, err)
			continue
		}
		removed++
		freed += size
	}
	return removed, freed
}
