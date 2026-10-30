// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/search"
)

type wildcardGroupFindRow struct {
	id, name, within                               string
	order, baseTotal, scanned, found, lastPosition int
	key                                            wildcardStreamKey
	matches                                        func(string) bool
	sections                                       dict.MaskSections
	words                                          []string
	positions                                      []int
}

// streamCachedWildcardGroupWithin searches the shared upper-result database
// once for the whole group. Dictionary IDs and original positions in that
// database determine the derived pages and navigation targets.
func (s *Server) streamCachedWildcardGroupWithin(ctx context.Context, request browseFindStreamRequest, send func(browseFindStreamEvent) error) error {
	if !request.mask || request.cacheID == "" || request.within == "" || request.cacheOrder != 0 {
		return errors.New("invalid grouped wildcard result search")
	}
	started := time.Now()
	firstWithin := request.within
	if isLemmaHeadwordMode(request.withinMode) {
		var err error
		firstWithin, err = s.wildcardHeadwordLemmaQuery(request.entry, request.within, request.withinMode)
		if err != nil {
			return err
		}
	}
	firstKey := wildcardStreamKey{dictID: request.entry.ID, within: wildcardWithinCacheKey(firstWithin, request.withinMode), withinMask: request.withinMask}
	entry, token, err := s.wildcardPages.begin(request.cacheID, wildcardCacheSignature{
		query: request.query, group: request.cacheGroup, wildcards: true,
		fts: request.cacheFTS, trigram: request.cacheTrigram,
	}, browsePageSize, request.cacheCount, request.entry.ID, request.entry.probeName(), 0,
		firstKey.within, request.withinMask, s.wildcardCacheDir(), request.diagnostics)
	if err != nil {
		return err
	}
	var groupRows []*wildcardGroupFindRow
	defer func() {
		entry.mu.Lock()
		for _, row := range groupRows {
			if row.key != firstKey && entry.streams[row.key] == token {
				delete(entry.streams, row.key)
			}
		}
		entry.mu.Unlock()
		s.wildcardPages.finish(entry, &firstKey, token)
	}()

	baseRows, err := entry.db.QueryContext(ctx, `SELECT dict_id,name,dict_order,total,complete FROM wildcard_dictionary
		WHERE within_text='' AND within_wildcards=0 ORDER BY dict_order`)
	if err != nil {
		return err
	}
	baseTotal := 0
	for baseRows.Next() {
		row := &wildcardGroupFindRow{lastPosition: -1}
		var complete int
		if err = baseRows.Scan(&row.id, &row.name, &row.order, &row.baseTotal, &complete); err != nil {
			break
		}
		if row.order != len(groupRows) || complete != 1 || row.baseTotal < 0 {
			err = errors.New("upper wildcard result cache is incomplete or unordered")
			break
		}
		baseTotal += row.baseTotal
		groupRows = append(groupRows, row)
	}
	if err == nil {
		err = baseRows.Err()
	}
	closeErr := baseRows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if len(groupRows) != request.cacheCount || len(groupRows) == 0 || groupRows[0].id != request.entry.ID {
		return errors.New("upper wildcard result cache does not match the dictionary group")
	}
	byID := make(map[string]*wildcardGroupFindRow, len(groupRows))
	var ftsMatches []string
	seenFTS := make(map[string]bool)
	for _, row := range groupRows {
		if _, exists := byID[row.id]; exists {
			return errors.New("duplicate dictionary in wildcard result cache")
		}
		byID[row.id] = row
		row.within = request.within
		if row.order == 0 {
			row.within = firstWithin
		} else if isLemmaHeadwordMode(request.withinMode) {
			dictionary, getErr := s.reg.get(row.id)
			if getErr != nil {
				return getErr
			}
			row.within, err = s.wildcardHeadwordLemmaQuery(dictionary, request.within, request.withinMode)
			if err != nil {
				return err
			}
		}
		row.key = wildcardStreamKey{dictID: row.id, within: wildcardWithinCacheKey(row.within, request.withinMode), withinMask: request.withinMask}
		if request.withinMask {
			pattern, compileErr := search.CompileMask(row.within)
			if compileErr != nil {
				return compileErr
			}
			row.matches = pattern.MatchString
		} else {
			row.matches = headwordModeMatcher(row.within, request.withinMode)
		}
		if entry.signature.fts && !request.withinMask && request.withinMode != "suffix" && request.withinMode != "contains" {
			if expression := mergedHeadwordFTSMatchMode(row.within, request.withinMode); expression != "" && !seenFTS[expression] {
				seenFTS[expression] = true
				ftsMatches = append(ftsMatches, "("+expression+")")
			}
		}
	}
	// One transaction resets all derived dictionary rows, rather than opening
	// and pruning the same cache once for every dictionary in the group.
	entry.mu.Lock()
	tx, err := entry.db.BeginTx(ctx, nil)
	if err == nil {
		for _, row := range groupRows[1:] {
			_, err = tx.ExecContext(ctx, `INSERT INTO wildcard_dictionary(within_text,within_wildcards,dict_id,name,dict_order,total,complete,alphabet_json)
				VALUES(?,?,?,?,?,0,0,'[]') ON CONFLICT(within_text,within_wildcards,dict_id) DO UPDATE SET
				name=excluded.name,dict_order=excluded.dict_order,total=0,complete=0,alphabet_json='[]'`,
				row.key.within, boolInt(row.key.withinMask), row.id, row.name, row.order)
			if err != nil {
				break
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM wildcard_block WHERE within_text=? AND within_wildcards=? AND dict_id=?`,
				row.key.within, boolInt(row.key.withinMask), row.id)
			if err != nil {
				break
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	if err == nil {
		for _, row := range groupRows[1:] {
			entry.streams[row.key] = token
		}
	}
	entry.mu.Unlock()
	if err != nil {
		return err
	}

	index, match := "", ""
	if request.withinMask && entry.signature.trigram {
		if literal := mergedMaskTrigramLiteral(request.within); literal != "" {
			index, match = "wildcard_word_trigram", `"`+strings.ReplaceAll(literal, `"`, `""`)+`"`
		}
	} else if entry.signature.trigram && (request.withinMode == "suffix" || request.withinMode == "contains") {
		if literal := mergedTextTrigramLiteral(request.within); literal != "" {
			index, match = "wildcard_word_trigram", `"`+strings.ReplaceAll(literal, `"`, `""`)+`"`
		}
	} else if len(ftsMatches) > 0 {
		index, match = "wildcard_word_fts", strings.Join(ftsMatches, " OR ")
	}
	scanTotal := baseTotal
	if index != "" {
		var indexedTotal int
		if err := entry.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wildcard_word`).Scan(&indexedTotal); err != nil {
			return err
		}
		if indexedTotal != baseTotal {
			index, match = "", ""
		} else {
			countSQL := `SELECT COUNT(*) FROM ` + index + ` f CROSS JOIN wildcard_word w ON w.id=f.rowid WHERE ` + index + ` MATCH ?`
			if err := entry.db.QueryRowContext(ctx, countSQL, match).Scan(&scanTotal); err != nil {
				return err
			}
			if scanTotal >= baseTotal {
				index, match = "", ""
				scanTotal = baseTotal
			}
		}
	}
	const batchSize = 3000
	candidateTable := ""
	if index != "" && scanTotal > batchSize*2 {
		// Materialize a large candidate set once. Repeating FTS MATCH with a
		// new offset for every bounded batch would rescan the same postings.
		candidateTable = fmt.Sprintf("wildcard_group_candidates_%d", token)
		if _, err := entry.db.ExecContext(ctx, `CREATE TEMP TABLE `+candidateTable+`(id INTEGER PRIMARY KEY,dict_id TEXT NOT NULL,base_position INTEGER NOT NULL,word TEXT NOT NULL)`); err != nil {
			return err
		}
		defer func() { _, _ = entry.db.Exec(`DROP TABLE IF EXISTS ` + candidateTable) }()
		insert := `INSERT INTO ` + candidateTable + `(id,dict_id,base_position,word)
			SELECT w.id,w.dict_id,w.base_position,w.word FROM ` + index + ` f CROSS JOIN wildcard_word w ON w.id=f.rowid WHERE ` + index + ` MATCH ?`
		if _, err := entry.db.ExecContext(ctx, insert, match); err != nil {
			return err
		}
	}
	if browseFindDiagnosticEnabled(ctx) {
		logx.SystemDiagnostic("headword_find_wildcard_group stage=plan cache=%q mode=%s typed_query=%q dictionaries=%d base_total=%d index=%s index_query=%q candidates=%d materialized=%t",
			request.cacheID, request.withinMode, request.typedWithin, len(groupRows), baseTotal, index, match, scanTotal, candidateTable != "")
	}
	scanned, found := 0, 0
	nextProgress := time.Now().Add(250 * time.Millisecond)
	flush := func(row *wildcardGroupFindRow) error {
		if len(row.words) == 0 {
			return nil
		}
		event := browseFindStreamEvent{Type: "block", DictID: row.id, Words: row.words, Positions: row.positions,
			Total: row.found, Scanned: row.scanned, ScanTotal: row.baseTotal, Size: browsePageSize, DiskCache: true}
		if err := entry.append(row.id, row.key.within, row.key.withinMask, token, event); err != nil {
			return err
		}
		row.words = nil
		row.positions = nil
		event.Words = nil
		event.Positions = nil
		return send(event)
	}
	add := func(row *wildcardGroupFindRow, position int, word string) error {
		if position <= row.lastPosition || position < 0 || position >= row.baseTotal {
			return fmt.Errorf("wildcard result position is invalid for dictionary %q: %d", row.id, position)
		}
		row.lastPosition = position
		row.scanned++
		scanned++
		if row.matches(word) {
			row.words = append(row.words, word)
			row.positions = append(row.positions, position)
			row.sections.Add(word)
			row.found++
			found++
			if len(row.words) >= browsePageSize {
				if err := flush(row); err != nil {
					return err
				}
			}
		}
		if time.Now().After(nextProgress) {
			if err := send(browseFindStreamEvent{Type: "progress", Scanned: scanned, ScanTotal: scanTotal, Total: found}); err != nil {
				return err
			}
			nextProgress = time.Now().Add(250 * time.Millisecond)
		}
		return nil
	}
	lastID := int64(0)
	lastOrder, lastBlock := -1, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if index != "" {
			query := `SELECT w.id,w.dict_id,w.base_position,w.word FROM ` + index + ` f CROSS JOIN wildcard_word w ON w.id=f.rowid WHERE ` + index + ` MATCH ? AND w.id>? ORDER BY w.id LIMIT ?`
			args := []any{match, lastID, batchSize}
			if candidateTable != "" {
				query = `SELECT id,dict_id,base_position,word FROM ` + candidateTable + ` WHERE id>? ORDER BY id LIMIT ?`
				args = []any{lastID, batchSize}
			}
			rows, queryErr := entry.db.QueryContext(ctx, query, args...)
			if queryErr != nil {
				return queryErr
			}
			type candidate struct {
				id       int64
				dictID   string
				position int
				word     string
			}
			batch := make([]candidate, 0, batchSize)
			for rows.Next() {
				var item candidate
				if err := rows.Scan(&item.id, &item.dictID, &item.position, &item.word); err != nil {
					_ = rows.Close()
					return err
				}
				batch = append(batch, item)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for _, item := range batch {
				row := byID[item.dictID]
				if row == nil {
					return fmt.Errorf("unknown wildcard dictionary %q", item.dictID)
				}
				if err := add(row, item.position, item.word); err != nil {
					return err
				}
			}
			if len(batch) == 0 {
				break
			}
			lastID = batch[len(batch)-1].id
			if len(batch) < batchSize {
				break
			}
			continue
		}
		rows, queryErr := entry.db.QueryContext(ctx, `SELECT dict_order,block_no,dict_id,start_offset,word_count,words_json FROM wildcard_block
			WHERE within_text='' AND within_wildcards=0 AND (dict_order>? OR (dict_order=? AND block_no>?))
			ORDER BY dict_order,block_no LIMIT ?`, lastOrder, lastOrder, lastBlock, 64)
		if queryErr != nil {
			return queryErr
		}
		type block struct {
			order, blockNo, start, count int
			dictID                       string
			encoded                      string
		}
		batch := make([]block, 0, 64)
		for rows.Next() {
			var item block
			if err := rows.Scan(&item.order, &item.blockNo, &item.dictID, &item.start, &item.count, &item.encoded); err != nil {
				_ = rows.Close()
				return err
			}
			batch = append(batch, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, item := range batch {
			row := byID[item.dictID]
			if row == nil {
				return fmt.Errorf("unknown wildcard dictionary %q", item.dictID)
			}
			if row.order != item.order || item.blockNo != item.start/browsePageSize+1 {
				return errors.New("upper wildcard result cache has inconsistent block order")
			}
			if item.start != row.scanned || item.count < 1 {
				return errors.New("upper wildcard result cache has a block gap")
			}
			var words []string
			if err := json.Unmarshal([]byte(item.encoded), &words); err != nil {
				return err
			}
			if len(words) != item.count {
				return errors.New("upper wildcard result cache block length differs from metadata")
			}
			for offset, word := range words {
				if err := add(row, item.start+offset, word); err != nil {
					return err
				}
			}
		}
		if len(batch) == 0 {
			break
		}
		lastOrder, lastBlock = batch[len(batch)-1].order, batch[len(batch)-1].blockNo
		if len(batch) < 64 {
			break
		}
	}
	if index == "" && scanned != baseTotal {
		return errors.New("upper wildcard result cache total differs from blocks")
	}
	if index == "" {
		for _, row := range groupRows {
			if row.scanned != row.baseTotal {
				return fmt.Errorf("upper wildcard result cache total differs for dictionary %q", row.id)
			}
		}
	}
	if index != "" && scanned != scanTotal {
		return errors.New("wildcard index candidates changed during search")
	}
	for _, row := range groupRows {
		if err := flush(row); err != nil {
			return err
		}
	}
	// Complete every dictionary in one transaction. Empty dictionaries otherwise
	// cause two separate synchronous SQLite writes each despite having no pages.
	type completion struct {
		row      *wildcardGroupFindRow
		alphabet []dict.Letter
		encoded  string
	}
	completed := make([]completion, 0, len(groupRows))
	for _, row := range groupRows {
		alphabet := row.sections.Finish()
		encoded, encodeErr := json.Marshal(alphabet)
		if encodeErr != nil {
			return encodeErr
		}
		completed = append(completed, completion{row: row, alphabet: alphabet, encoded: string(encoded)})
	}
	entry.mu.Lock()
	for _, item := range completed {
		if entry.streams[item.row.key] != token {
			entry.mu.Unlock()
			return errWildcardCacheStreamReplaced
		}
	}
	tx, err = entry.db.BeginTx(ctx, nil)
	if err == nil {
		for _, item := range completed {
			result, execErr := tx.ExecContext(ctx, `UPDATE wildcard_dictionary SET total=?,complete=1,alphabet_json=?
				WHERE within_text=? AND within_wildcards=? AND dict_id=?`, item.row.found, item.encoded,
				item.row.key.within, boolInt(item.row.key.withinMask), item.row.id)
			if execErr != nil {
				err = execErr
				break
			}
			if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
				if affectedErr != nil {
					err = affectedErr
				} else {
					err = errors.New("wildcard result dictionary disappeared")
				}
				break
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	entry.mu.Unlock()
	if err != nil {
		return err
	}
	for _, item := range completed {
		if err := send(browseFindStreamEvent{Type: "done", DictID: item.row.id, Total: item.row.found,
			Scanned: item.row.scanned, ScanTotal: item.row.baseTotal, Size: browsePageSize, DiskCache: true,
			Done: true, Alphabet: item.alphabet, EffectiveQueries: splitMergedHeadwordAlternatives(item.row.within)}); err != nil {
			return err
		}
	}
	if browseFindDiagnosticEnabled(ctx) {
		logx.SystemDiagnostic("headword_find_wildcard_group stage=done cache=%q mode=%s typed_query=%q dictionaries=%d strategy=%s scanned=%d found=%d elapsed_ms=%d",
			request.cacheID, request.withinMode, request.typedWithin, len(groupRows), map[bool]string{true: "index_candidates", false: "full_block_scan"}[index != ""], scanned, found, time.Since(started).Milliseconds())
	}
	return nil
}
