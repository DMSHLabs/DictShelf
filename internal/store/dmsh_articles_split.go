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
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// articleHandle returns the connection that holds the article index: the
// sibling articles.db when this dictionary has one, and this database when the
// index was built inside it (every database built before the split existed).
//
// It is opened on first use, not in Open, and the file has to say it belongs to
// this dictionary (dict_uuid) before it is used - half of another dictionary's
// index is worse than none of this one's, which is also the rule media.db
// follows. A file that fails either test is remembered as tried, so a broken
// sibling costs one attempt and not one per query.
func (s *Store) articleHandle() (*sql.DB, bool) {
	if !s.ftsOK {
		return nil, false
	}
	s.articleMu.Lock()
	defer s.articleMu.Unlock()
	if s.articleTried {
		return s.articleDB, s.articleDB != nil
	}
	s.articleTried = true
	path, ok := ArticlesSibling(s.meta.Path)
	if !ok {
		return nil, false
	}
	db, err := sql.Open(driverName, dsnRO(path))
	if err != nil {
		return nil, false
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, false
	}
	// The sibling carries its own meta table with the dictionary it belongs to.
	var owner string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='dict_uuid'`).Scan(&owner); err != nil {
		db.Close()
		return nil, false
	}
	if s.uuid == "" || owner != s.uuid {
		db.Close()
		return nil, false
	}
	s.articlePath, s.articleDB = path, db
	return s.articleDB, true
}

// articleHit is one article the article index matched: its id, its headword,
// and its body when the caller asked for one.
type articleHit struct {
	ID int64
	W  string
	M  []byte
}

// articleMatches runs an FTS expression against the article index and returns
// what matched, best first. withBodies decides whether the (large) body column
// is read: the streaming id path pages by id and wants only the headword, and
// carries every match in memory, so asking for bodies there would read the
// whole dictionary.
//
// One query when the index sits in this database, two when it sits in the
// sibling: a JOIN cannot cross files without ATTACH, and ATTACH is a
// per-connection statement while this process pools connections and has no
// limit on their number - the reader deliberately stays away from it
// (dmsh_articles_split.go's header has the whole argument). The second query is
// a batch by primary key, and the rank order of the first is preserved: rank
// comes from the FTS index and from nothing else.
func (s *Store) articleMatches(ctx context.Context, match string, limit int, withBodies bool) ([]articleHit, error) {
	columns := "e.id,e.w"
	if withBodies {
		columns = "e.id,e.w,e.m"
	}
	db, split := s.articleHandle()
	if !split {
		query := `SELECT ` + columns + ` FROM ` + s.articleFTSTable + ` f JOIN entry e ON e.id = f.rowid
			WHERE ` + s.articleFTSTable + ` MATCH ?1` + subEntryFilter("e.w") + `ORDER BY f.rank`
		args := []any{match}
		if limit > 0 {
			query += ` LIMIT ?2`
			args = append(args, clamp(limit))
		}
		rows, err := s.db.QueryContext(ctx, query, args...)
		return scanArticleHits(rows, err, withBodies)
	}
	// The sibling has no entry table, so the sub-entry filter the JOIN carries in
	// its WHERE cannot be part of this query: it is applied when the rows are read
	// (entryRowsByIDs), and the prefix asked for has to grow to pay for it. With
	// LIMIT alone, a dictionary whose rows are mostly hidden @-subentries - 59% of
	// LDOCE6 - would answer with a nearly empty page where the same query on an
	// unsplitted database answers `limit` results, which is what
	// TestSubEntriesHiddenFromBrowsing and TestSplitArticlesAnswersLikeAnUnsplittedOne
	// are for.
	want, asked := 0, 0
	if limit > 0 {
		want = clamp(limit)
		asked = want
	}
	var hits []articleHit
	for {
		idsQuery := `SELECT rowid FROM ` + s.articleFTSTable + ` WHERE ` + s.articleFTSTable + ` MATCH ?1 ORDER BY rank`
		args := []any{match}
		if asked > 0 {
			idsQuery += ` LIMIT ?2`
			args = append(args, asked)
		}
		ids, err := scanIDs(db.QueryContext(ctx, idsQuery, args...))
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		// Rank order, and each batch is a prefix of it, so appending keeps it.
		batch, err := s.entryRowsByIDs(ctx, ids, withBodies)
		if err != nil {
			return nil, err
		}
		hits = append(hits, batch...)
		// Enough real entries, or the index has no more ids to give.
		if want == 0 || len(hits) >= want || len(ids) < asked {
			break
		}
		asked *= 4
	}
	return hits, nil
}

// scanIDs reads a column of rowids, closing the rows either way.
func scanIDs(rows *sql.Rows, err error) ([]int64, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// entryRowsByIDs reads the rows for the ids, keeping the ids' order.
// Unknown ids are dropped, exactly as a JOIN would drop them.
func (s *Store) entryRowsByIDs(ctx context.Context, ids []int64, withBodies bool) ([]articleHit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	columns := "id,w"
	if withBodies {
		columns = "id,w,m"
	}
	found := make(map[int64]articleHit, len(ids))
	const batch = 400
	for start := 0; start < len(ids); start += batch {
		end := min(start+batch, len(ids))
		part := ids[start:end]
		parts := make([]string, len(part))
		args := make([]any, len(part))
		for i, id := range part {
			parts[i], args[i] = "?", id
		}
		rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM entry WHERE id IN (`+strings.Join(parts, ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var h articleHit
			if withBodies {
				if err := rows.Scan(&h.ID, &h.W, &h.M); err != nil {
					rows.Close()
					return nil, err
				}
			} else if err := rows.Scan(&h.ID, &h.W); err != nil {
				rows.Close()
				return nil, err
			}
			// The DELETE-side half of the JOIN's filter: an MDict expandable
			// section is not a result (subEntryFilter, store.go - the same rule in
			// Go, so the two cannot drift).
			if isSubEntry(h.W) {
				continue
			}
			found[h.ID] = h
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	out := make([]articleHit, 0, len(ids))
	for _, id := range ids {
		if h, ok := found[id]; ok {
			out = append(out, h)
		}
	}
	return out, nil
}

// scanArticleHits reads the one-query (unsplit) form.
func scanArticleHits(rows *sql.Rows, err error, withBodies bool) ([]articleHit, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []articleHit
	for rows.Next() {
		var h articleHit
		if withBodies {
			if err := rows.Scan(&h.ID, &h.W, &h.M); err != nil {
				return nil, err
			}
		} else if err := rows.Scan(&h.ID, &h.W); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
