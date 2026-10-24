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

	"github.com/wuweidict/wudict/internal/dict"
)

// StreamArticleIDs visits matching prepared entries in display order without
// reading article bodies. It bounds Go memory even for a million matches.
// fullTextMatch is the selected rung of the article search ladder.
func (s *Store) StreamArticleIDs(ctx context.Context, query, mode, fullTextMatch string, emit func([]int64, int, bool) error) error {
	var rows *sql.Rows
	var err error
	match := func(string) bool { return true }
	switch mode {
	case "fts":
		if !s.ftsOK {
			return dict.ErrUnsupported
		}
		if fullTextMatch == "" {
			return emit(nil, 0, true)
		}
		rows, err = s.db.QueryContext(ctx, `SELECT e.id,e.w FROM `+s.articleFTSTable+` f
			JOIN entry e ON e.id=f.rowid WHERE `+s.articleFTSTable+` MATCH ?`+
			subEntryFilter("e.w")+` ORDER BY f.rank`, fullTextMatch)
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
			if literal := headwordTrigramLiteral(query); literal != "" {
				statement = `SELECT e.id,e.w FROM entry_trigram t JOIN entry e ON e.id=t.rowid
					WHERE entry_trigram MATCH ? AND e.w NOT LIKE '@_%' ORDER BY e.w COLLATE NOCASE,e.w,e.id`
				args = []any{`"` + strings.ReplaceAll(literal, `"`, `""`) + `"`}
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
	defer rows.Close()
	ids := make([]int64, 0, 300)
	total := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var id int64
		var word string
		if err := rows.Scan(&id, &word); err != nil {
			return err
		}
		if !match(word) {
			continue
		}
		ids = append(ids, id)
		total++
		if len(ids) == 1 && total == 1 || len(ids) == 300 {
			if err := emit(ids, total, false); err != nil {
				return err
			}
			ids = make([]int64, 0, 300)
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
		found[id] = dict.Result{Headword: word, Body: decodeBody(body)}
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
