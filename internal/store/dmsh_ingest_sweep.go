// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"crypto/rand"
	"encoding/hex"
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

// isIngestTemp reports whether a file name is one of those working files. The
// one rule, shared with clean.go's staleIngests: that one applies an hour's
// grace because `wudict clean` can run while a build is in flight, this one
// none because it runs at startup.
func isIngestTemp(name string) bool {
	return strings.Contains(strings.ToLower(name), ingestTempInfix)
}

// removalInfix marks a folder a removal has renamed out of the way and is about
// to delete (store.RemovePrepared).
//
// Renaming first is what makes a removal all-or-nothing where it matters.
// Deleting a folder file by file can fail in the middle - on Windows a file
// another process holds cannot be unlinked at all - and what is left then is a
// dictionary with its text.db gone and its icon cache not: half a dictionary,
// reported as an error. A rename either happens or does not, so the window in
// which that can arise is the milliseconds it takes, and a crash inside it
// leaves a whole dictionary under this name instead of a broken one under the
// real name.
//
// Everything that walks the library treats the name as debris: the startup sweep
// removes it (a DIRECTORY - an ingest's working name is always a file), `clean`
// reports it as an interrupted removal, and Folders() does not list it, so the
// crash window cannot show a dictionary under a name nobody asked for.
const removalInfix = ".removing."

func isRemovalDebris(name string) bool {
	return strings.Contains(strings.ToLower(name), removalInfix)
}

// removalDebrisName is where a removal renames its folder to before deleting it:
// beside it (same volume, so the rename is atomic) and uniquely named, so a
// previous crash's debris cannot be overwritten by the next removal.
func removalDebrisName(path string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return path + removalInfix + hex.EncodeToString(b)
}

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
// install), so nothing can be indexing yet - and that is also why it needs no
// grace period, unlike clean.go's staleIngests, which can be asked to clean a
// folder while a build is writing into it. Nothing else ever reads a name with
// that infix either - isTextDB accepts text.db and *.text.db, and every walker
// uses the published names - so a file this leaves is a file no one can use.
func SweepIngestTemps(dir string) (removed int, freed int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			// A directory can only be a removal's debris (removalInfix): an
			// ingest writes FILES, and a folder is the user's unit - whatever
			// else they put in one is theirs (D20), so nothing else here is
			// touched. removeFolder, not os.RemoveAll: the same retry and the
			// same reason as a removal's own delete.
			if !isRemovalDebris(entry.Name()) {
				continue
			}
			size := TreeSize(path)
			if err := removeFolder(path); err != nil {
				logx.V("remove: could not clear the debris %s: %v", path, err)
				continue
			}
			removed++
			freed += size
			continue
		}
		if !isIngestTemp(entry.Name()) {
			continue
		}
		var size int64
		if info, err := entry.Info(); err == nil {
			size = info.Size()
		}
		if err := os.Remove(path); err != nil {
			logx.V("ingest: could not clear the abandoned %s: %v", path, err)
			continue
		}
		removed++
		freed += size
	}
	return removed, freed
}
