// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build (!sqlite_fts5 || !cgo) && (!purego || sqlite_fts5)

// This file exists to fail the build with a NAME rather than with a missing
// symbol, and it covers exactly the combinations that select no driver:
//
//	neither tag            -> this file
//	-tags sqlite_fts5 with CGO_ENABLED=0 -> this file
//	-tags "purego sqlite_fts5" without cgo -> this file
//
// The fork builds the cgo driver (driver_cgo.go): `-tags "sqlite_fts5
// sqlite_dbstat"` with CGO_ENABLED=1 and the GCC that build-windows.local.bat
// points at. The pure-Go driver is opt-in (`-tags purego`) because it is a
// different binary, not a fallback: dbstat is compiled into it and tagged out of
// the cgo one, so the measured index sizes and the tests differ between them -
// see driver_purego.go.
//
// A build that reaches this file has selected neither, which is the accident
// this whole arrangement is for: it used to be answered silently with the
// pure-Go driver.

package store

var _ = noSQLiteDriverSelected__buildWith_CGO_ENABLED_1_and_tags_sqlite_fts5_sqlite_dbstat__or_purego_on_purpose
