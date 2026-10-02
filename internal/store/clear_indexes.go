// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import "database/sql"

// ClearSearchIndexes removes optional indexes without reading the source again.
// The caller must close serving handles and serialize with ingestion.
func ClearSearchIndexes(path string, contains, fullText bool) error {
	db, err := sql.Open(driverName, dsnIngest(path)+"&mode=rw")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=DELETE"); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA synchronous=FULL"); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{}
	if contains {
		statements = append(statements, "DROP TABLE IF EXISTS entry_trigram", "UPDATE meta SET value='0' WHERE key='has_trigram'")
	}
	if fullText {
		// Base accent-insensitive searches use the headword column in FTS too.
		// Remove article text while retaining that always-present headword index.
		statements = append(statements, "DROP TABLE IF EXISTS entry_fts",
			"CREATE VIRTUAL TABLE entry_fts USING fts5(w, txt, content='', columnsize=0, tokenize='unicode61 remove_diacritics 2')",
			"INSERT INTO entry_fts(rowid,w,txt) SELECT id,w,'' FROM entry",
			"UPDATE meta SET value='headwords' WHERE key='ingest_level'")
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = db.Exec("VACUUM")
	return err
}
