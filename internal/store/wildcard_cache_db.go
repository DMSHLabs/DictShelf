// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import "database/sql"

// OpenWildcardCacheDB opens a writable SQLite database used only for a
// temporary Wildcards result cache. The database is separate from dictionary
// indexes and uses the same platform-selected SQLite driver.
func OpenWildcardCacheDB(path string) (*sql.DB, error) {
	db, err := sql.Open(driverName, dsnWildcardCache(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
