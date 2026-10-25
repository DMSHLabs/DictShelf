// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestIngestBulk256FiveColumnRows(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open(driverName, dsnIngest(filepath.Join(t.TempDir(), "bulk.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE rows(a, b, c, d, e)"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	bulk := newIngestBulk(ctx, tx, "INSERT INTO rows(a, b, c, d, e) VALUES", 5)
	defer bulk.close()
	for i := 0; i < ingestBulkRows; i++ {
		bulk.add(i, i, i, i, i)
	}
	if err := bulk.flush(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM rows").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != ingestBulkRows {
		t.Fatalf("rows = %d, want %d", count, ingestBulkRows)
	}
}
