// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
)

// SnapshotFile copies a live SQLite database, including committed WAL data.
func SnapshotFile(ctx context.Context, source, destination string) error {
	db, err := sql.Open(driverName, "file:"+uriPath(source)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, "VACUUM INTO ?", destination)
	return err
}
