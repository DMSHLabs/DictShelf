// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Rebuilding the article index without rebuilding the dictionary.
//
// The article index is a function of what text.db already holds: its key is
// entry.id, its headword column is entry.w, and its text column is
// htmlref.PlainText of the body in entry.m - the same normalized body the
// ingest indexed, kept compressed in that column. So it can be built from the
// database alone, with the ids coming from the very table every reader uses.
// That is what makes the alignment between the two files structural rather than
// a promise: this cannot index the wrong rows, whatever changed in a Reader
// since (see dmsh_articles_split.go for why the two files are one thing).
//
// It is the other half of the split. The split made REMOVING the index a file
// deletion instead of a VACUUM over the whole dictionary, and this makes
// REBUILDING it a second pass over text.db instead of a full ingest that reads
// the source and rebuilds every headword table - a rebuild whose headword half
// would be written bit for bit as it already is. Two shapes cannot use it, and
// both are the same fact - the article TEXT is not in this database: one whose
// reader drops body columns (a ZIM that keeps its articles in the archive), and
// one that still keeps the index inside text.db (moving it out is the migration
// a full rebuild performs, VACUUM included).

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/DMSHLabs/DictShelf/internal/htmlref"
)

// ErrArticleNeedsReingest reports that the article index cannot be built from
// the prepared database alone: the caller rebuilds from the source instead.
var ErrArticleNeedsReingest = errors.New("the article index cannot be built from the database alone")

// articleIndexBatch is how many rows are handed to the index per statement,
// and how often progress is reported.
const articleIndexBatch = 2000

// ArticleIndexRebuildable reports whether the article index of textDB can be
// rebuilt from the database itself (RebuildArticleIndexContext), without the
// source. It is the cheap question a caller asks BEFORE it chooses the path.
func ArticleIndexRebuildable(textDB string) bool {
	p := Inspect(textDB)
	if !p.Usable() || p.Meta["dict_uuid"] == "" {
		return false
	}
	// The index inside text.db: taking it out is the migration, and that one
	// VACUUMs the whole file - which is what the split exists to avoid, so it
	// stays where it has always been, in the full rebuild.
	if p.Plan.FullText && p.Plan.ArticlesInside {
		return false
	}
	// A rebuild writes article_fts, and the reader takes that name from THIS
	// database's meta (Store.articleFTSTable): a database old enough to predate
	// the name is old enough to be rebuilt whole.
	if stampOf(p.Meta, "ingest_version", 1) < 6 {
		return false
	}
	// Bodies are not stored: the article text lives only in the source (a ZIM
	// whose reader drops them), so there is nothing in this file to index. An
	// absent key is not a "0": only a build that knew the answer records one.
	return p.Meta["stored_body_bytes"] != "0"
}

// RebuildArticleIndexContext builds the article index of textDB from the
// database's own columns and publishes it as its own file, leaving every
// headword table and the rest of text.db untouched.
//
// The work is the indexing itself - tokenizing each article, which is what a
// rebuild costs in either path - and what it does not do is everything around
// it: no source is read, no headword table is written, text.db is not replaced.
// Callers hold the files for the whole operation (the server's walk fence) and
// are expected to have let go of the old index file, which on Windows cannot be
// renamed over while anything has it open.
//
// It answers ErrArticleNeedsReingest - before writing anything - when
// ArticleIndexRebuildable says this database cannot do it, and again if the
// rows turn out to carry no text at all (a database built by a reader that
// drops bodies but that does not record stored_body_bytes).
func RebuildArticleIndexContext(ctx context.Context, textDB string, progress Progress) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ArticleIndexRebuildable(textDB) {
		return fmt.Errorf("rebuild the article index of %s from the source: %w", textDB, ErrArticleNeedsReingest)
	}
	p := Inspect(textDB)
	articles := ArticlesFilePath(textDB)
	tmp := tempDBName(articles)
	_ = os.Remove(tmp)

	write, err := sql.Open(driverName, dsnIngest(tmp))
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			write.Close()
			_ = os.Remove(tmp)
		}
	}()
	// The same schema the ingest writes for an index that is a file of its own
	// (ingest.go): a rebuild has to leave a database this build could have made.
	if _, err = write.ExecContext(ctx, `CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT);
		CREATE VIRTUAL TABLE article_fts USING fts5(w, txt, content='', columnsize=0, tokenize='unicode61 remove_diacritics 2')`); err != nil {
		return err
	}
	// The sibling belongs to the dictionary beside it, and says so - the same
	// dict_uuid check attachArticles and the reader both make.
	if _, err = write.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES('dict_uuid', ?)`, p.Meta["dict_uuid"]); err != nil {
		return err
	}

	src, err := openRO(textDB)
	if err != nil {
		return err
	}
	defer src.Close()
	var total int
	if err = src.QueryRowContext(ctx, "SELECT count(*) FROM entry").Scan(&total); err != nil {
		return err
	}
	tx, err := write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	bulk := newIngestBulk(ctx, tx, "INSERT INTO article_fts(rowid, w, txt) VALUES", 3)

	rows, err := src.QueryContext(ctx, "SELECT id, w, m FROM entry ORDER BY id")
	if err != nil {
		return err
	}
	done, indexed := 0, 0
	for rows.Next() {
		var id int64
		var headword string
		var body []byte
		if err := rows.Scan(&id, &headword, &body); err != nil {
			rows.Close()
			return err
		}
		text := htmlref.PlainText(decodeBody(body))
		if text != "" {
			indexed++
		}
		bulk.add(id, headword, text)
		done++
		if done%articleIndexBatch == 0 {
			if err := bulk.flush(); err != nil {
				rows.Close()
				return err
			}
			if progress != nil {
				progress(done, total)
			}
			if err := ctx.Err(); err != nil {
				rows.Close()
				return err
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if err := bulk.flush(); err != nil {
		return err
	}
	if indexed == 0 {
		// Every row carries an empty body: there is no article text in this
		// database to index. Publishing an empty index over a working one would
		// be worse than doing nothing, so the caller rebuilds from the source -
		// the answer ArticleIndexRebuildable gives, here read from the data
		// instead of from a meta key an older library may not carry.
		return fmt.Errorf("%s: no article text to index: %w", textDB, ErrArticleNeedsReingest)
	}
	// Merge the index's segments, as the ingest does once the rows are in.
	if _, err := tx.ExecContext(ctx, "INSERT INTO article_fts(article_fts) VALUES('optimize')"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := write.Close(); err != nil {
		return err
	}
	syncFile(tmp)
	if err := os.Rename(tmp, articles); err != nil {
		return err
	}
	published = true
	if progress != nil {
		progress(done, total)
	}
	// What says the index is a file of its own, and that the text was indexed,
	// is this database's meta - and the reader takes both from there. Written
	// after the file is in place, so a reader arriving in between sees either
	// the old shape or the new one, and both are usable.
	meta, err := liveWrite(ctx, textDB)
	if err != nil {
		return err
	}
	defer meta.Close()
	if _, err := meta.ExecContext(ctx, `INSERT OR REPLACE INTO meta(key, value) VALUES('ingest_level', 'text'), ('articles_split', '1')`); err != nil {
		return err
	}
	// The recorded sizes are what the dictionary list shows, and the full-text
	// one has just changed: measure them from the files that are there now.
	recordIndexSizes(meta, textDB)
	_ = writeStatsFromMeta(textDB)
	IndexDiagnostic("article index rebuilt from the database database=%q rows=%d indexed=%d", textDB, done, indexed)
	return nil
}
