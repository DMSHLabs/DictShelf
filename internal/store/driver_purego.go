// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build purego && !sqlite_fts5

package store

// The pure-Go driver: modernc.org/sqlite (FTS5 always compiled in, no C
// toolchain).
//
// It used to be the silent fallback - any build without -tags sqlite_fts5, and
// any build of that tag on a machine without a C compiler, landed here. It is
// now asked for by NAME, with -tags purego, and a build that selects neither
// driver fails outright (driver_missing.go). The reason is that the fallback was
// not a fallback but a different binary: this driver and the cgo one differ in
// more than speed. dbstat is compiled into this one, so every index size is
// measured here and reads as zero on a cgo build that was not given the
// sqlite_dbstat tag; and a test run that silently used this driver said nothing
// about the cgo binary the fork actually ships.
//
// Nothing is wrong with driving with modernc - it cannot be short of FTS5, it
// cross-compiles from one machine and the tests pass on it - so keep using it
// wherever that is what you mean to test.
//
// (The earlier note here said a plain `go build` and the release cross-builds
// used it; those releases are built per-platform with cgo now - see
// .github/workflows/build-cgo.yml - so nothing ships on this path by default
// any more.)

import (
	"strconv"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

// cacheClause sizes the per-connection page cache when this platform asks for
// something other than the driver default (see pageCacheKiB).
func cacheClause() string {
	if pageCacheKiB <= 0 {
		return ""
	}
	return "&_pragma=cache_size(-" + strconv.Itoa(pageCacheKiB) + ")"
}

// dsnRO is a read-only, query-only connection string (modernc pragma syntax).
func dsnRO(path string) string {
	return "file:" + uriPath(path) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)" + cacheClause()
}

// dsnIngest is a throwaway-safe bulk-write connection string.
func dsnIngest(path string) string {
	return "file:" + uriPath(path) + "?_pragma=journal_mode(OFF)&_pragma=synchronous(OFF)&_pragma=cache_size(-" + strconv.Itoa(ingestCacheKiB) + ")"
}

func dsnWildcardCache(path string) string {
	return "file:" + uriPath(path) + "?mode=rwc&_pragma=journal_mode(DELETE)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
}

func dsnWildcardCacheWAL(path string) string {
	return "file:" + uriPath(path) + "?mode=rwc&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
}
