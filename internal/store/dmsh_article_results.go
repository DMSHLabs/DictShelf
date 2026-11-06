// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// StreamArticleIDs visits matching prepared entries in display order without
// reading article bodies. It bounds Go memory even for a million matches.
// fullTextMatch is the selected rung of the article search ladder.
func (s *Store) StreamArticleIDs(ctx context.Context, query, mode, fullTextMatch string, emit func([]int64, int, bool) error) error {
	var rows *sql.Rows
	var hits []articleHit
	var err error
	match := func(string) bool { return true }
	switch mode {
	// The strict readings ask the same question of the same index: what differs
	// is the expression in fullTextMatch, which the caller built from the
	// mode's own rung. The index may live in this database or in its sibling
	// articles.db, which is a difference of connection, not of query
	// (articleMatches).
	case "fts", "phrase", "near":
		if !s.ftsOK {
			return dict.ErrUnsupported
		}
		if fullTextMatch == "" {
			return emit(nil, 0, true)
		}
		hits, err = s.articleMatches(ctx, fullTextMatch, 0, false)
	case "exact":
		if s.hasFolded {
			column, key := s.headwordSearchKey(ctx, query)
			statement := strings.ReplaceAll(`SELECT e.id,e.w FROM entry e JOIN (
				SELECT id, min(priority) AS priority FROM (
					SELECT id, CASE WHEN w=?2 THEN 0 WHEN w=?2 COLLATE NOCASE THEN 1 ELSE 2 END AS priority
						FROM entry WHERE @KEY@=?1 COLLATE NOCASE
					UNION ALL
					SELECT entry_id AS id, CASE WHEN w=?2 THEN 0 WHEN w=?2 COLLATE NOCASE THEN 1 ELSE 2 END AS priority
						FROM alias WHERE @KEY@=?1 COLLATE NOCASE
				) GROUP BY id
			) m ON e.id=m.id ORDER BY m.priority,e.w COLLATE NOCASE,e.w`, "@KEY@", column)
			rows, err = s.db.QueryContext(ctx, statement, key, query)
		} else {
			rows, err = s.db.QueryContext(ctx, `SELECT id,w FROM entry ORDER BY w COLLATE NOCASE,w,id`)
			match = dict.HeadwordExactMatcher(query)
		}
	case "prefix", "suffix", "contains", "word", "wordLemma", "exactLemma", "wildcards":
		statement := `SELECT id,w FROM entry WHERE w NOT LIKE '@_%' ORDER BY w COLLATE NOCASE,w,id`
		var args []any
		switch mode {
		case "prefix":
			match = dict.HeadwordTermMatcher(query)
			if s.hasHeadwordFTS {
				if candidate := buildMatch(query, "w"); candidate != "" {
					statement = `SELECT e.id,e.w FROM entry_fts f JOIN entry e ON e.id=f.rowid
						WHERE entry_fts MATCH ? AND e.w NOT LIKE '@_%' ORDER BY e.w COLLATE NOCASE,e.w,e.id`
					args = []any{candidate}
				}
			}
		case "suffix":
			match = dict.HeadwordSuffixMatcher(query)
		case "contains":
			match = dict.HeadwordContainsMatcher(query)
		case "word", "wordLemma":
			match = dict.HeadwordWordMatcher(query)
			if s.hasHeadwordFTS {
				if candidate := buildWholeWordMatch(query, "w"); candidate != "" {
					statement = `SELECT e.id,e.w FROM entry_fts f JOIN entry e ON e.id=f.rowid
						WHERE entry_fts MATCH ? AND e.w NOT LIKE '@_%' ORDER BY e.w COLLATE NOCASE,e.w,e.id`
					args = []any{candidate}
				}
			}
		case "exactLemma":
			match = dict.HeadwordExactMatcher(query)
		case "wildcards":
			var re *regexp.Regexp
			re, err = regexp.Compile("(?i)^" + wildcardRegexp(query) + "$")
			if err == nil {
				match = re.MatchString
			}
		}
		if (mode == "suffix" || mode == "contains") && s.hasTrigram && !s.staleFold {
			// Candidate trigrams, never the whole word. The trigram index is
			// detail=none, so a quoted literal of four runes or more is a
			// multi-token PHRASE to the trigram tokenizer, which that table
			// refuses outright ("phrase queries are not supported
			// (detail!=full)"): asking for the whole word made the complete list
			// fail for every dictionary holding a Contains index. Several single
			// trigrams ANDed is the way to use more than three runes of the
			// query at all; the whole word is still the matcher's job, below,
			// and the index only has to narrow the candidates.
			if candidate := trigramCandidates(headwordTrigramLiteral(query)); candidate != "" {
				statement = `SELECT e.id,e.w FROM entry_trigram t JOIN entry e ON e.id=t.rowid
					WHERE entry_trigram MATCH ? AND e.w NOT LIKE '@_%' ORDER BY e.w COLLATE NOCASE,e.w,e.id`
				args = []any{candidate}
			}
		}
		if ctx.Value(ignoreDiacriticsKey{}) != true && mode != "wildcards" {
			caseMatch := headwordSearchMatcher(ctx, query, mode)
			baseMatch := match
			match = func(word string) bool { return baseMatch(word) && caseMatch(word) }
		}
		rows, err = s.db.QueryContext(ctx, statement, args...)
	default:
		return fmt.Errorf("unsupported article result mode %q", mode)
	}
	if err != nil {
		return err
	}
	if rows != nil {
		defer rows.Close()
	}
	// The scan is ours, so it is watched (internal/watchdog); the MATCH that
	// produced the hits is opaque and is not (indexconst). The record is the
	// entry id being visited.
	watch := s.watch("article-ids")
	defer watch.Done()
	// The pages are built the same way whether the rows came from one query or
	// from two: the first hit goes out alone (a search must paint something
	// immediately), then a page every 300.
	ids := make([]int64, 0, 300)
	total := 0
	add := func(id int64, word string) error {
		if !match(word) {
			return nil
		}
		ids = append(ids, id)
		total++
		if len(ids) == 1 && total == 1 || len(ids) == 300 {
			if err := emit(ids, total, false); err != nil {
				return err
			}
			ids = make([]int64, 0, 300)
		}
		return nil
	}
	if hits != nil {
		for _, h := range hits {
			if err := ctx.Err(); err != nil {
				return err
			}
			watch.Moved(h.ID)
			if err := add(h.ID, h.W); err != nil {
				return err
			}
		}
		return emit(ids, total, true)
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var id int64
		var word string
		if err := rows.Scan(&id, &word); err != nil {
			return err
		}
		watch.Moved(id)
		if err := add(id, word); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return emit(ids, total, true)
}

// ArticlesByIDs reads only the selected portion, preserving the result order
// supplied by the cache rather than the database's row order.
func (s *Store) ArticlesByIDs(ctx context.Context, ids []int64) ([]dict.Result, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 500 {
		return nil, fmt.Errorf("article portion exceeds 500 entries")
	}
	parts := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("invalid article ID %s", strconv.FormatInt(id, 10))
		}
		parts[i], args[i] = "?", id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,w,m FROM entry WHERE id IN (`+strings.Join(parts, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := make(map[int64]dict.Result, len(ids))
	for rows.Next() {
		var id int64
		var word string
		var body []byte
		if err := rows.Scan(&id, &word, &body); err != nil {
			return nil, err
		}
		found[id] = dict.Result{Headword: word, Body: s.body.decode(body)}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]dict.Result, 0, len(ids))
	for _, id := range ids {
		result, ok := found[id]
		if !ok {
			return nil, fmt.Errorf("article ID %d disappeared from prepared index", id)
		}
		out = append(out, result)
	}
	return out, nil
}

// entryIDBatch is how many headwords one lookup statement may carry. SQLite's
// default limit on host parameters is 999 per statement (SQLITE_MAX_VARIABLE_NUMBER), and a cache build asks about thousands of articles at a time.
const entryIDBatch = 400

// EntryIDsForHeadwords maps headwords to prepared entry ids, keeping the order
// it is given and dropping what this database does not hold.
//
// It exists for the one path that cannot ask SQLite for its results: a
// dictionary whose full text comes from its archive (see
// internal/format/zim/glass). The archive answers with article headwords, and a
// page of the answer still has to become entry ids, because that is what the
// result cache stores and what ArticlesByIDs reads back.
//
// The first id for a headword wins, which matters when two ranked hits name the
// same article: the list keeps the position of the better-ranked one and does
// not show the article twice.
func (s *Store) EntryIDsForHeadwords(ctx context.Context, words []string) ([]int64, error) {
	if len(words) == 0 {
		return nil, nil
	}
	out := make([]int64, 0, len(words))
	seen := make(map[int64]bool, len(words))
	watch := s.watch("article-id-lookup")
	defer watch.Done()
	for start := 0; start < len(words); start += entryIDBatch {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+entryIDBatch, len(words))
		batch := words[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, len(batch))
		for i, w := range batch {
			args[i] = w
		}
		rows, err := s.db.QueryContext(ctx, `SELECT w,id FROM entry WHERE w IN (`+placeholders+`)`+subEntryFilter("w"), args...)
		if err != nil {
			return nil, err
		}
		byWord := make(map[string]int64, len(batch))
		for rows.Next() {
			var w string
			var id int64
			if err := rows.Scan(&w, &id); err != nil {
				rows.Close()
				return nil, err
			}
			if _, dup := byWord[w]; !dup {
				byWord[w] = id
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		for _, w := range batch {
			id, ok := byWord[w]
			if !ok || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		watch.Moved(int64(len(out)))
	}
	return out, nil
}
