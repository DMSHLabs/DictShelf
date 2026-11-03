// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !sqlite_fts5 || !cgo

// This file exists to fail the build with a NAME rather than with a list of
// missing symbols, and it covers the two combinations that select no driver:
//
//	no tags                          -> this file
//	-tags sqlite_fts5, CGO_ENABLED=0 -> this file
//
// The declarations below are the surface driver_cgo.go provides, each of them
// referring to one undefined name on purpose: the compiler then reports THIS
// file, once per entry, instead of naming the call sites all over the package
// (`undefined: dsnIngest` in ingest.go, and "too many errors" after the tenth).
//
// The fork builds the cgo driver (driver_cgo.go): `-tags "sqlite_fts5
// sqlite_dbstat"` with CGO_ENABLED=1 and the GCC that build-windows.local.bat
// points at. That is the only driver since 2026-11-03: the pure-Go alternative
// (modernc.org/sqlite, `-tags purego`) was removed, because every build this fork
// makes was cgo already, and keeping a second driver meant a second binary whose
// dbstat support, measured index sizes and tests all differed (D4 amendment).
//
// A build that reaches this file does not fail by accident, it fails because it
// asked for what this fork does not build - which is the whole point: it used to
// be answered silently with the pure-Go driver.

package store

const driverName = noSQLiteDriverSelected__buildWith_CGO_ENABLED_1_and_tags_sqlite_fts5_sqlite_dbstat

func cacheClause() string {
	return noSQLiteDriverSelected__buildWith_CGO_ENABLED_1_and_tags_sqlite_fts5_sqlite_dbstat
}

func dsnRO(path string) string {
	return noSQLiteDriverSelected__buildWith_CGO_ENABLED_1_and_tags_sqlite_fts5_sqlite_dbstat
}

func dsnIngest(path string) string {
	return noSQLiteDriverSelected__buildWith_CGO_ENABLED_1_and_tags_sqlite_fts5_sqlite_dbstat
}

func dsnWildcardCache(path string) string {
	return noSQLiteDriverSelected__buildWith_CGO_ENABLED_1_and_tags_sqlite_fts5_sqlite_dbstat
}

func dsnWildcardCacheWAL(path string) string {
	return noSQLiteDriverSelected__buildWith_CGO_ENABLED_1_and_tags_sqlite_fts5_sqlite_dbstat
}
