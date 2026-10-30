// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// RebuildContainsIndex replaces the trigram and token indexes from the prepared
// headwords. It does not rescan the source or rewrite the base/full-text data.
func RebuildContainsIndex(path string, progress Progress) (err error) {
	return RebuildContainsIndexContext(context.Background(), path, progress)
}

func RebuildContainsIndexContext(ctx context.Context, path string, progress Progress) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	db, err := sql.Open(driverName, dsnIngest(path)+"&mode=rw")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=DELETE"); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ingestVersion int
	_ = tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='ingest_version'").Scan(&ingestVersion)
	if _, err = tx.ExecContext(ctx, "DROP TABLE IF EXISTS entry_trigram"); err != nil {
		return err
	}
	if ingestVersion >= 6 {
		if _, err = tx.ExecContext(ctx, "DROP TABLE IF EXISTS entry_fts"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `CREATE VIRTUAL TABLE entry_trigram USING fts5(
		w, content='', detail=none, columnsize=0, tokenize='trigram')`); err != nil {
		return err
	}
	if ingestVersion >= 6 {
		if _, err = tx.ExecContext(ctx, `CREATE VIRTUAL TABLE entry_fts USING fts5(
			w, content='', columnsize=0, tokenize='unicode61 remove_diacritics 2')`); err != nil {
			return err
		}
	}
	var total int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM entry").Scan(&total); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id, w FROM entry ORDER BY id")
	if err != nil {
		return err
	}
	insert, err := tx.PrepareContext(ctx, "INSERT INTO entry_trigram(rowid, w) VALUES(?, ?)")
	if err != nil {
		rows.Close()
		return err
	}
	var insertFTS *sql.Stmt
	if ingestVersion >= 6 {
		insertFTS, err = tx.PrepareContext(ctx, "INSERT INTO entry_fts(rowid, w) VALUES(?, ?)")
		if err != nil {
			rows.Close()
			insert.Close()
			return err
		}
	}
	var done int
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			break
		}
		var id int64
		var word string
		if err = rows.Scan(&id, &word); err != nil {
			break
		}
		if _, err = insert.ExecContext(ctx, id, dict.Fold(word)); err != nil {
			break
		}
		if insertFTS != nil {
			if _, err = insertFTS.ExecContext(ctx, id, word); err != nil {
				break
			}
		}
		done++
		if progress != nil && (done%batchSize == 0 || done == total) {
			progress(done, total)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if closeErr := insert.Close(); err == nil {
		err = closeErr
	}
	if insertFTS != nil {
		if closeErr := insertFTS.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO entry_trigram(entry_trigram) VALUES('optimize')"); err != nil {
		return err
	}
	if insertFTS != nil {
		if _, err = tx.ExecContext(ctx, "INSERT INTO entry_fts(entry_fts) VALUES('optimize')"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE meta SET value='1' WHERE key='has_trigram'"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf("UPDATE meta SET value='%d' WHERE key='fold_version'", dict.FoldVersion)); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "VACUUM")
	if ctx.Err() != nil {
		return nil // the index is committed and valid; VACUUM is optional cleanup
	}
	return err
}

// ClearSearchIndexes removes optional indexes without reading the source again.
// The caller must close serving handles and serialize with ingestion.
func ClearSearchIndexes(path string, contains, fullText bool) error {
	return ClearSearchIndexesContext(context.Background(), path, contains, fullText)
}

func ClearSearchIndexesContext(ctx context.Context, path string, contains, fullText bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	db, err := sql.Open(driverName, dsnIngest(path)+"&mode=rw")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=DELETE"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{}
	var ingestVersion int
	_ = tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='ingest_version'").Scan(&ingestVersion)
	if contains {
		statements = append(statements, "DROP TABLE IF EXISTS entry_trigram", "UPDATE meta SET value='0' WHERE key='has_trigram'")
		if ingestVersion >= 6 {
			statements = append(statements, "DROP TABLE IF EXISTS entry_fts")
		}
	}
	if fullText {
		if ingestVersion >= 6 {
			statements = append(statements, "DROP TABLE IF EXISTS article_fts")
		} else {
			statements = append(statements, "DROP TABLE IF EXISTS entry_fts",
				"CREATE VIRTUAL TABLE entry_fts USING fts5(w, txt, content='', columnsize=0, tokenize='unicode61 remove_diacritics 2')",
				"INSERT INTO entry_fts(rowid,w,txt) SELECT id,w,'' FROM entry")
		}
		statements = append(statements, "UPDATE meta SET value='headwords' WHERE key='ingest_level'")
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "VACUUM")
	if ctx.Err() != nil {
		return nil // the index deletion is committed; VACUUM is optional cleanup
	}
	return err
}
