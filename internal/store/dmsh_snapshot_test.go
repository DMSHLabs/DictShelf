// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSnapshotFileIncludesWAL(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	dest := filepath.Join(dir, "copy.db")
	db, err := sql.Open(driverName, source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; CREATE TABLE sample (value TEXT); INSERT INTO sample VALUES ('committed')"); err != nil {
		t.Fatal(err)
	}
	if err = SnapshotFile(context.Background(), source, dest); err != nil {
		t.Fatal(err)
	}
	copy, err := sql.Open(driverName, dest)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	var value string
	if err = copy.QueryRow("SELECT value FROM sample").Scan(&value); err != nil || value != "committed" {
		t.Fatalf("snapshot value %q: %v", value, err)
	}
}
