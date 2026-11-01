// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// The article index in a file of its own.
//
// A prepared database holds two very different things: the headword index, which
// every lookup, Browse and mask search needs, and the article full-text index,
// which is optional, several times larger, and specific to one dictionary. They
// were always one file, so removing the full text meant DROP plus VACUUM over
// everything - rewriting tens of gigabytes to return a few - and changing any
// FTS setting meant rebuilding the whole dictionary, headwords included.
//
// New builds may therefore write the article index to `articles.db` beside
// `text.db`. The presence of the file is what says so - not a version, not a key
// - which is the same rule media.db follows, and it is what lets old databases
// keep working: a database with the index inside text.db reads exactly as it did
// before, and both shapes answer the same queries.
//
// The pair belongs together, so the file is accepted only when it says it is the
// same dictionary (dict_uuid), exactly as media.db is.

package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// ArticlesDBName is the sibling file holding the article full-text index.
const ArticlesDBName = "articles.db"

// ArticlesFilePath is where the article index of the database at textDB lives.
func ArticlesFilePath(textDB string) string {
	return filepath.Join(filepath.Dir(textDB), ArticlesDBName)
}

// ArticlesSibling reports the sibling file of textDB, when there is one.
func ArticlesSibling(textDB string) (string, bool) {
	p := ArticlesFilePath(textDB)
	if fi, err := os.Stat(p); err != nil || fi.IsDir() || fi.Size() == 0 {
		return "", false
	}
	return p, true
}

// attachArticles brings the sibling file into the connection and reports whether
// it belongs to this database: a file that names another dictionary is ignored,
// because half of another dictionary's index is worse than none of this one's.
func attachArticles(db *sql.DB, textDB, path, uuid string) bool {
	if _, err := db.Exec(`ATTACH DATABASE ? AS articles`, path); err != nil {
		return false
	}
	var theirs string
	if err := db.QueryRow(`SELECT value FROM articles.meta WHERE key='dict_uuid'`).Scan(&theirs); err != nil {
		_, _ = db.Exec(`DETACH DATABASE articles`)
		return false
	}
	if uuid == "" || theirs != uuid {
		_, _ = db.Exec(`DETACH DATABASE articles`)
		return false
	}
	return true
}

// detachArticles lets go of the sibling, for a caller that is about to delete or
// replace it.
func detachArticles(db *sql.DB) {
	_, _ = db.Exec(`DETACH DATABASE articles`)
}

// createArticlesDB makes the sibling file and attaches it to the ingest
// connection, so the article index can be built there while the rest of the
// database is built beside it.
func createArticlesDB(db *sql.DB, path, uuid string) error {
	if _, err := db.Exec(`ATTACH DATABASE ? AS articles`, path); err != nil {
		return fmt.Errorf("attach %s: %w", path, err)
	}
	stmts := []string{
		`CREATE TABLE articles.meta (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO articles.meta(key, value) VALUES('dict_uuid', ?)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt, uuid); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	}
	return nil
}
