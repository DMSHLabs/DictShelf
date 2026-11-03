// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build sqlite_fts5 && cgo

package store

// The fast driver: mattn/go-sqlite3 (cgo - decision D4), selected by
// -tags sqlite_fts5 as `make build` and `make install` pass.
//
// BOTH conditions are required, and neither is optional (D29). mattn
// compiles SQLite *without* FTS5 unless the sqlite_fts5 tag sets
// -DSQLITE_ENABLE_FTS5 in its own cgo CFLAGS, and store's base schema
// always creates an FTS5 table (see ingest.go) - so a mattn build
// missing that tag fails at runtime with "no such module: fts5". The
// && cgo half keeps CGO_ENABLED=0 away from mattn's !cgo stub, whose
// registered driver refuses every Open.
//
// It is the ONLY driver: the pure-Go one (modernc.org/sqlite, `-tags purego`)
// was removed on 2026-11-03, when every build this fork makes - Windows, Android
// through the NDK, and the cgo release workflow - was already cgo. Anything this
// constraint rejects lands on driver_missing.go, which fails the build by name.

import (
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

const driverName = "sqlite3"

// cacheClause sizes the per-connection page cache when this platform asks for
// something other than the driver default (see pageCacheKiB).
func cacheClause() string {
	if pageCacheKiB <= 0 {
		return ""
	}
	return "&_cache_size=-" + strconv.Itoa(pageCacheKiB)
}

// dsnRO is a read-only, query-only connection string.
func dsnRO(path string) string {
	return "file:" + uriPath(path) + "?mode=ro&_query_only=1&_busy_timeout=5000" + cacheClause()
}

// dsnIngest is a throwaway-safe bulk-write connection string (the ingest
// target is a temp file renamed on success, so durability is pointless).
func dsnIngest(path string) string {
	return "file:" + uriPath(path) + "?_journal_mode=OFF&_synchronous=OFF&_cache_size=-" + strconv.Itoa(ingestCacheKiB)
}

func dsnWildcardCache(path string) string {
	return "file:" + uriPath(path) + "?mode=rwc&_journal_mode=DELETE&_synchronous=NORMAL&_busy_timeout=5000"
}

func dsnWildcardCacheWAL(path string) string {
	return "file:" + uriPath(path) + "?mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"
}
