// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"os"
	"strconv"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
)

// A prepared dictionary is frozen output of the code that built it. When that
// code changes, the data on disk does not follow, and nothing about it fails:
// it keeps answering queries with whatever the old code wrote. The versions
// below are how the running code says what it would write now, and Stale is
// the one place that compares the two.
//
// Each stamp covers one layer, so a bump invalidates exactly what it changed:
//
//	ingest_version   this file's IngestPlan: the schema's use, StripHTML, the
//	                 alias and redirect rules, body normalization
//	reader_version   one format's Reader (dict.RegisterReaderVersion)
//	markup_version   internal/artmark's role vocabulary (MarkupStale)
//	fold_version     dict.Fold, persisted in folded keys and optional trigram (FoldStale)
//	media_version    IngestMedia
//
// A missing stamp means the database predates the stamp, and is read as
// version 1 - the behaviour in force when the stamp was introduced - so the
// day this ships does not declare an entire library outdated.
//
// Stale reports; it never rebuilds. Rebuilding costs minutes to hours on a
// large library and is started only by the user (wudict reindex, or the
// panel's Rebuild), never on anyone's behalf (docs.local/PERF.md).

// IngestVersion identifies IngestPlan's behaviour. Bump it in the same commit
// as any change to what an ingest writes for the same Reader output; the
// golden tests in the format packages fail on such a change and say so.
//
// 8: headword_count is what the ingest counted (articles + aliases) instead of
// the count the source declared, so a library built by 7 shows the old number
// until it is rebuilt.
const IngestVersion = 8

// StatsVersion identifies how the size measurements recorded at ingest
// (body_bytes, headword_bytes) were taken. They are volatile meta - the
// estimate that reads them is a prediction about a build, not prepared
// content - so a change here invalidates them without asking anyone to
// rebuild; a reader that finds a version it does not know treats the
// measurements as absent rather than misread.
// StatsVersion is the measurement format. 2 added FTS5 index sizes; 3 counts
// both headword FTS tables as the Contains size.
const StatsVersion = 3

// MediaVersion identifies IngestMedia's behaviour, bumped the same way.
const MediaVersion = 1

// Reason is why a prepared dictionary no longer matches what this build would
// prepare from its source. The values are stable: the CLI prints them and the
// HTTP API reports them.
type Reason string

const (
	ReasonSchema Reason = "schema" // unreadable, or another schema: not usable at all
	ReasonSource Reason = "source" // the source file was edited or replaced
	ReasonAbbrev Reason = "abbrev" // the DSL abbreviation glossary changed
	ReasonIngest Reason = "ingest" // built by an older IngestPlan
	ReasonReader Reason = "reader" // built by an older Reader for its format
	ReasonMarkup Reason = "markup" // articles written with an older role markup
	ReasonFold   Reason = "fold"   // persisted folded keys built by another dict.Fold
	ReasonMedia  Reason = "media"  // media.db unreadable, unpaired or outdated
)

// stampOf reads an integer version stamp, defaulting to def when the key is
// absent (a database written before the stamp existed) or not a number.
func stampOf(m map[string]string, key string, def int) int {
	if s := m[key]; s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

// Outdated is the code-version half of Stale, for a meta table already in
// hand: the reasons that depend only on which code built the database, not on
// the files beside it. Cheap - no I/O.
//
// Ingest and reader versions are compared as "older than": a library prepared
// by a NEWER build is not something this one can improve by rebuilding it.
// Markup and fold keep their established exact-match rule (MarkupStale,
// FoldStale), since either direction changes what a query or a stylesheet
// meets.
func Outdated(m map[string]string) []Reason {
	var out []Reason
	if stampOf(m, "ingest_version", 1) < IngestVersion {
		out = append(out, ReasonIngest)
	}
	if rv := dict.ReaderVersion(m["format"]); rv > 0 && stampOf(m, "reader_version", 1) < rv {
		out = append(out, ReasonReader)
	}
	if MarkupStale(m) {
		out = append(out, ReasonMarkup)
	}
	if FoldStale(m) {
		out = append(out, ReasonFold)
	}
	return out
}

// Prepared is one prepared database read once. Every question asked about a
// prepared dictionary - can this build use it, what was it built with, is it
// outdated, does its media pair with it - is answered from one of these, so a
// caller asking several of them pays for one read of the meta table, not one
// per question.
type Prepared struct {
	TextDB string
	Meta   map[string]string // nil: missing, or not a readable database
	Schema int               // PRAGMA user_version; meaningful only with Meta
	Plan   Plan              // what it was built with; zero when Meta is nil
	Media  bool              // a media.db lies beside it (paired or not)
	// Optional search-index sizes, measured from SQLite's dbstat virtual table.
	ContainsSize int64
	FullTextSize int64
}

// ZIMForm reports the form of a prepared ZIM database from its meta: whether it
// carries the headword list, and whether it also carries article bodies.
// index_kind says which - "headwords" is the title-only index (SPEC §24) and
// "articles" the read-through one. A database with neither marker predates the
// distinction, and is trusted for neither until it is rebuilt.
func ZIMForm(meta map[string]string) (headwords, articles bool) {
	switch meta["index_kind"] {
	case "headwords":
		return true, false
	case "articles":
		return true, true
	}
	return false, false
}

// Inspect reads textDB's meta table and notes whether a media.db lies beside
// it. A missing or unreadable file is a Prepared with no Meta, never an error:
// to every caller that is simply "not prepared".
func Inspect(textDB string) Prepared {
	p := Prepared{TextDB: textDB}
	if _, err := os.Stat(textDB); err == nil {
		db, openErr := openRO(textDB)
		if openErr == nil {
			defer db.Close()
			var schema int
			if db.QueryRow("PRAGMA user_version").Scan(&schema) == nil {
				if m, metaErr := readMeta(db); metaErr == nil {
					p.Meta, p.Schema, p.Plan = m, schema, PlanFromMeta(m)
					// The same rule Open applies: a split index whose file is
					// gone is no index, so the row must not offer one (see
					// store.go's Open).
					if p.Plan.FullText && !p.Plan.ArticlesInside {
						if _, ok := ArticlesSibling(textDB); !ok {
							p.Plan.FullText = false
						}
					}
					// Deliberately NOT measured here. The index sizes come from
					// dbstat, a page-by-page walk that costs seconds on a large
					// database, and this runs in the path that opens a
					// dictionary - so a library of forty of them took a minute
					// to open, on every start, to draw chips that show sizes.
					// The numbers live in the measurement cache instead
					// (ReadStats, filled by the registry's background pass).
					// A split index is its own file: that is the whole point of
					// splitting it, and its size needs no page accounting.
					if art, ok := ArticlesSibling(textDB); ok {
						if fi, err := os.Stat(art); err == nil {
							p.FullTextSize = fi.Size()
						}
					}
				}
			}
		}
	}
	if sib := MediaSibling(textDB); sib != "" && fsx.FileExists(sib) {
		p.Media = true
	}
	return p
}

// MeasureIndexSizes returns the on-disk bytes used by the optional FTS5
// indexes, by walking the database's pages (dbstat).
//
// It is exported for the one caller that may pay it: the background measurement
// pass, which runs once per process and writes the result into the cache. The
// listing path must not call it (see Inspect).
func MeasureIndexSizes(textDB string, plan Plan) (contains, fullText int64) {
	return MeasureIndexSizesContext(context.Background(), textDB, plan)
}

// MeasureIndexSizesContext is MeasureIndexSizes for a walk that can be called
// off. The walk is a page-by-page pass and holds a read lock on the database for
// as long as it runs, so the writer of that same file - an ingest renaming a new
// database over this one, or an index rebuilt in place - has to be able to stop
// it rather than wait for it. A cancelled walk answers (0, 0), which is what any
// walk that could not read the database answers; the caller is the measurement
// pass, which leaves the dictionary for the writer to schedule again.
func MeasureIndexSizesContext(ctx context.Context, textDB string, plan Plan) (contains, fullText int64) {
	db, err := openRO(textDB)
	if err != nil {
		return 0, 0
	}
	defer db.Close()
	return indexSizesOf(ctx, db, textDB, "", plan)
}

// SearchIndexSizes returns the on-disk bytes used by the optional FTS5 indexes.
// dbstat is absent from some SQLite builds, in which case sizes remain zero.
func SearchIndexSizes(textDB string, plan Plan) (contains, fullText int64) {
	db, err := openRO(textDB)
	if err != nil {
		return 0, 0
	}
	defer db.Close()
	return indexSizesOf(context.Background(), db, textDB, "", plan)
}

// indexSizesOf measures the optional indexes of the database open on db - the
// connection that holds it, so an ingest can ask about the file it is writing
// without opening a second handle for it.
//
// The contains index is always in that database. The article index is not: a
// build keeps it in a file of its own unless the Plan says otherwise, so it is
// read
// from whichever file holds it - the published sibling beside the database
// (articlesFileOf), the temp file a build has not renamed into place yet
// (articlesDB, which the ingest names), or the database itself, which is where
// every database built before the split keeps it.
func indexSizesOf(ctx context.Context, db *sql.DB, textDB, articlesDB string, plan Plan) (contains, fullText int64) {
	if db == nil {
		return 0, 0
	}
	if plan.Contains {
		contains = sizeOfTable(ctx, db, "entry_trigram")
		// ingest_version 6 split the previous shared entry_fts table into
		// headword-token FTS and independent article_fts. Before that, entry_fts
		// was the article index and must not be charged to Contains.
		var ingestVersion int
		_ = db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM meta WHERE key='ingest_version'`).Scan(&ingestVersion)
		if ingestVersion >= 6 {
			contains += sizeOfTable(ctx, db, "entry_fts")
		}
	}
	if !plan.FullText {
		return contains, 0
	}
	if file := articlesFileOf(textDB, articlesDB); file == "" || file == textDB {
		return contains, sizeOfTable(ctx, db, "article_fts")
	}
	side, err := openRO(articlesFileOf(textDB, articlesDB))
	if err != nil {
		return contains, 0
	}
	defer side.Close()
	return contains, sizeOfTable(ctx, side, "article_fts")
}

// articlesFileOf answers which file holds the article index of the database at
// textDB: the one named by the caller (a build's temp file), the sibling the
// build published, or "" for the database itself. An empty textDB means the
// caller knows there is no published sibling to look for - the ingest, whose
// directory may still hold one from an earlier split build.
func articlesFileOf(textDB, articlesDB string) string {
	if articlesDB != "" {
		return articlesDB
	}
	if textDB == "" {
		return ""
	}
	if p, ok := ArticlesSibling(textDB); ok {
		return p
	}
	return textDB
}

// sizeOfTable is one optional index's on-disk bytes: the table and its shadow
// tables, summed over the pages dbstat says they own.
func sizeOfTable(ctx context.Context, db *sql.DB, table string) int64 {
	var size int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(pgsize), 0) FROM dbstat WHERE name = ? OR name LIKE ?`, table, table+"_%").Scan(&size); err != nil {
		return 0
	}
	return size
}

// Usable reports that this build can open it: readable, and this schema.
func (p Prepared) Usable() bool { return p.Meta != nil && textSchemaOK(p.Schema) }

// Current reports that it is usable and was built from srcPath as the file is
// now. A missing source is not a change: the prepared data stands on its own.
func (p Prepared) Current(srcPath string) bool {
	return p.Usable() && !sourceChangedMeta(p.Meta, srcPath)
}

// MediaPaired reports that the media.db beside it serves it: readable, packed
// by the current IngestMedia, and stamped with this text.db's dict_uuid. An
// unpaired media.db is invisible to Store.mediaDB, so for every purpose that
// matters it is not packed. Reads the media.db's meta; ask once.
func (p Prepared) MediaPaired() bool {
	return p.Media && p.Meta != nil && !mediaStale(MediaSibling(p.TextDB), p.Meta["dict_uuid"])
}

// Stale reports every reason it no longer matches what this build would
// prepare from srcPath now; empty means current. srcPath may be "" or gone:
// the reasons that need the source are then skipped and the rest reported.
func (p Prepared) Stale(srcPath string) []Reason {
	out := p.TextStale(srcPath)
	if p.Usable() && p.Media && !p.MediaPaired() {
		out = append(out, ReasonMedia)
	}
	return out
}

// TextStale is Stale for the text.db alone: the reasons a rebuild of the text
// answers. Media is judged after the text is current, because a text rebuild
// can be what unpairs it.
func (p Prepared) TextStale(srcPath string) []Reason {
	if !p.Usable() {
		return []Reason{ReasonSchema}
	}
	var out []Reason
	if srcPath != "" {
		if sourceChangedMeta(p.Meta, srcPath) {
			out = append(out, ReasonSource)
		}
		companion, _ := dict.AbbrevCompanion(srcPath)
		if abbrevChangedMeta(p.Meta, companion) {
			out = append(out, ReasonAbbrev)
		}
	}
	return append(out, Outdated(p.Meta)...)
}

// Stale is Inspect(textDB).Stale(srcPath).
func Stale(textDB, srcPath string) []Reason { return Inspect(textDB).Stale(srcPath) }

// KeptPlan is the Plan to REBUILD an existing text.db with: whatever it was
// built with, so a rebuild the user did not ask to change never changes it.
// An unreadable or missing database gets the default (headwords only, D24),
// which is also what a dictionary never prepared gets.
func KeptPlan(textDB string) Plan { return Inspect(textDB).Plan }

// MediaPaired is Inspect(textDB).MediaPaired().
func MediaPaired(textDB string) bool { return Inspect(textDB).MediaPaired() }

// ReadMetaSchema reads a wudict database's meta table together with its schema
// version (PRAGMA user_version). An error means the file cannot be read as a
// SQLite database with a meta table at all.
func ReadMetaSchema(dbPath string) (map[string]string, int, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, 0, err // never let the driver create an empty file
	}
	db, err := openRO(dbPath)
	if err != nil {
		return nil, 0, err
	}
	defer db.Close()
	var ver int
	if err := db.QueryRow("PRAGMA user_version").Scan(&ver); err != nil {
		return nil, 0, err
	}
	m, err := readMeta(db)
	if err != nil {
		return nil, ver, err
	}
	return m, ver, nil
}

// PlanFromMeta is the Plan a prepared database was built with, read back from
// its meta. The one reader of `ingest_level` and `has_trigram`.
func PlanFromMeta(m map[string]string) Plan {
	return Plan{
		FullText: m["ingest_level"] != levelHeadwords,
		Contains: m["has_trigram"] == "1",
		// "Inside" only describes a database that HAS an article index: one with
		// none is not inside anything, and a rebuild that adds full text adds it as
		// a file of its own (which is also why an old database and an empty plan
		// compare equal - Plan.SameIndexes).
		ArticlesInside: m["ingest_level"] != levelHeadwords && m["articles_split"] != "1",
	}
}

// ingestLevel is PlanFromMeta's inverse for `ingest_level`.
func ingestLevel(p Plan) string {
	if p.FullText {
		return levelText
	}
	return levelHeadwords
}

// mediaStale reports that a media.db cannot serve the text.db stamped with
// uuid: unreadable, another schema, another dictionary's, or packed by an
// older IngestMedia.
func mediaStale(mediaDB, uuid string) bool {
	m, schema, err := ReadMetaSchema(mediaDB)
	if err != nil || schema != sidecarSchema {
		return true
	}
	return uuid == "" || m["dict_uuid"] != uuid || stampOf(m, "media_version", 1) < MediaVersion
}

// keptUUID is the dict_uuid an ingest into dbPath should reuse, or "" for a
// fresh one. Reused only while the database there was built from the same,
// unchanged source: then the media.db beside it was packed from the same
// resources and stays paired across the rebuild, instead of being orphaned by
// a rebuild that did not touch a single resource. A changed source gets a new
// identity, and its media is repacked by whoever rebuilt it.
func keptUUID(dbPath, srcPath string) string {
	m, schema, err := ReadMetaSchema(dbPath)
	if err != nil || !textSchemaOK(schema) || srcPath == "" {
		return ""
	}
	if !fsx.SamePath(m["source_path"], srcPath) || sourceChangedMeta(m, srcPath) {
		return ""
	}
	return m["dict_uuid"]
}
