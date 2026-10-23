// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/indexconst"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
)

// mergedBrowseFromWildcard sorts only the already found headwords. A view
// switch must not build a merged index of every headword in the dictionaries.
func (s *Server) mergedBrowseFromWildcard(ctx context.Context, q url.Values, ids []string, progress func(int, int) error) (browseResp, error) {
	started := time.Now()
	sourceID := q.Get("sourcecache")
	if !wildcardCacheIDPattern.MatchString(sourceID) {
		return browseResp{}, errors.New("invalid Wildcards result cache")
	}
	useFTS, useTrigram := q.Get("mergedfts") == "1", q.Get("mergedtrigram") == "1"
	var signature strings.Builder
	signature.WriteString(mergedBrowseSignatureVersion + "\n")
	fmt.Fprintf(&signature, "fts=%t\ntrigram=%t\n", useFTS, useTrigram)
	for _, id := range ids {
		signature.WriteString(id)
		signature.WriteByte(0)
		signature.WriteString(sourceID)
		signature.WriteByte(0)
		signature.WriteString(q.Get("q"))
		signature.WriteByte('\n')
	}
	key := sha256.Sum256([]byte(signature.String()))
	cacheID := hex.EncodeToString(key[:16])
	dir := s.wildcardCacheDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return browseResp{}, err
	}
	path := filepath.Join(dir, "wildcard_merged_"+cacheID+".db")
	unlock := lockMergedCache(path)
	defer unlock()
	s.wildcardPages.beginExternal(path)
	defer s.wildcardPages.endExternal(path)
	if db, err := store.OpenWildcardCacheDB(path); err == nil {
		var saved string
		err = db.QueryRowContext(ctx, `SELECT signature FROM merged_meta LIMIT 1`).Scan(&saved)
		_ = db.Close()
		if err == nil && saved == signature.String() {
			if browseFindDiagnosticEnabled(ctx) {
				logx.System("headword_merged_browse cache=wildcard_results_hit elapsed_ms=%d", time.Since(started).Milliseconds())
			}
			return readMergedBrowsePage(path, q, cacheID)
		}
	}
	sourcePath := filepath.Join(dir, "wildcard_"+sourceID+".db")
	if _, err := os.Stat(sourcePath); err != nil {
		return browseResp{}, fmt.Errorf("Wildcards result cache is unavailable: %w", err)
	}
	s.wildcardPages.beginExternal(sourcePath)
	defer s.wildcardPages.endExternal(sourcePath)
	source, err := store.OpenWildcardCacheDB(sourcePath)
	if err != nil {
		return browseResp{}, fmt.Errorf("opening Wildcards result cache: %w", err)
	}
	defer source.Close()
	var savedID, query, group string
	var wildcards, fts, trigram, pageSize, dictCount int
	err = source.QueryRowContext(ctx, `SELECT cache_id,query_text,group_id,wildcards,fts,trigram,page_size,dict_count
		FROM wildcard_cache_meta LIMIT 1`).Scan(&savedID, &query, &group, &wildcards, &fts, &trigram, &pageSize, &dictCount)
	if err != nil || savedID != sourceID || query != q.Get("q") || group != q.Get("wildgroup") || wildcards != 1 ||
		fts != boolInt(useFTS) || trigram != boolInt(useTrigram) || pageSize != indexconst.WildcardCacheBlockSize || dictCount != len(ids) {
		return browseResp{}, errors.New("Wildcards result cache does not match the selected search")
	}
	rows, err := source.QueryContext(ctx, `SELECT dict_id,total,complete FROM wildcard_dictionary
		WHERE within_text='' AND within_wildcards=0`)
	if err != nil {
		return browseResp{}, err
	}
	totals := make(map[string]int, len(ids))
	matched := 0
	for rows.Next() {
		var id string
		var total, complete int
		if err := rows.Scan(&id, &total, &complete); err != nil {
			_ = rows.Close()
			return browseResp{}, err
		}
		if complete != 1 || total < 0 {
			_ = rows.Close()
			return browseResp{}, errors.New("unfinished Wildcards result cache")
		}
		totals[id] = total
		if total > 0 {
			matched++
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return browseResp{}, err
	}
	if len(totals) != len(ids) {
		return browseResp{}, errors.New("unfinished Wildcards result cache")
	}
	all := 0
	for _, id := range ids {
		total, ok := totals[id]
		if !ok {
			return browseResp{}, errors.New("Wildcards result cache has different dictionaries")
		}
		all += total
	}
	removeWildcardCacheFiles(path)
	db, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return browseResp{}, err
	}
	completed := false
	defer func() {
		_ = db.Close()
		if !completed {
			removeWildcardCacheFiles(path)
		}
	}()
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE merged_meta(signature TEXT NOT NULL,total INTEGER NOT NULL,alphabet_json TEXT NOT NULL)`,
		`CREATE TABLE merged_word(word TEXT NOT NULL COLLATE NOCASE PRIMARY KEY)`,
		`CREATE TABLE merged_anchor(offset INTEGER PRIMARY KEY,word TEXT NOT NULL COLLATE NOCASE)`,
		`CREATE TABLE merged_stage(word TEXT NOT NULL COLLATE NOCASE PRIMARY KEY)`,
		`CREATE TABLE merged_source_meta(matched_dictionaries INTEGER NOT NULL)`,
	}
	if useFTS {
		statements = append(statements, `CREATE VIRTUAL TABLE merged_word_fts USING fts5(word, content='', columnsize=0, tokenize='unicode61 remove_diacritics 2')`)
	}
	if useTrigram {
		statements = append(statements, `CREATE VIRTUAL TABLE merged_word_trigram USING fts5(word, content='', columnsize=0, tokenize='trigram')`)
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return browseResp{}, err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return browseResp{}, err
	}
	insertStage, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO merged_stage(word) VALUES(?)`)
	if err != nil {
		_ = tx.Rollback()
		return browseResp{}, err
	}
	blocks, err := source.QueryContext(ctx, `SELECT dict_id,words_json,word_count FROM wildcard_block
		WHERE within_text='' AND within_wildcards=0 ORDER BY dict_order,block_no`)
	if err != nil {
		_ = insertStage.Close()
		_ = tx.Rollback()
		return browseResp{}, err
	}
	counts := make(map[string]int, len(ids))
	loaded := 0
	for blocks.Next() {
		var id, encoded string
		var count int
		if err = blocks.Scan(&id, &encoded, &count); err != nil {
			break
		}
		if _, ok := totals[id]; !ok {
			err = errors.New("Wildcards result cache has different dictionaries")
			break
		}
		var words []string
		if err = json.Unmarshal([]byte(encoded), &words); err != nil {
			break
		}
		if len(words) != count {
			err = errors.New("Wildcards result cache has an incomplete block")
			break
		}
		for _, word := range words {
			if _, err = insertStage.ExecContext(ctx, word); err != nil {
				break
			}
		}
		if err != nil {
			break
		}
		counts[id] += count
		loaded += count
		if progress != nil && loaded%3000 < count {
			err = progress(loaded, max(1, all*2))
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = blocks.Err()
	}
	_ = blocks.Close()
	_ = insertStage.Close()
	if err == nil {
		for _, id := range ids {
			if counts[id] != totals[id] {
				err = errors.New("Wildcards result cache has an incomplete dictionary")
				break
			}
		}
	}
	if err != nil {
		_ = tx.Rollback()
		return browseResp{}, err
	}
	if err := tx.Commit(); err != nil {
		return browseResp{}, err
	}
	// The cache DB uses one connection. Read sorted rows through a second
	// connection so the writer transaction cannot wait on its own cursor.
	orderedDB, err := store.OpenWildcardCacheDB(path)
	if err != nil {
		return browseResp{}, err
	}
	defer orderedDB.Close()
	ordered, err := orderedDB.QueryContext(ctx, `SELECT word FROM merged_stage ORDER BY word COLLATE NOCASE`)
	if err != nil {
		return browseResp{}, err
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		_ = ordered.Close()
		return browseResp{}, err
	}
	insertWord, err := tx.PrepareContext(ctx, `INSERT INTO merged_word(rowid,word) VALUES(?,?)`)
	if err != nil {
		_ = ordered.Close()
		_ = tx.Rollback()
		return browseResp{}, err
	}
	var sections dict.MaskSections
	unique := 0
	for ordered.Next() {
		var word string
		if err = ordered.Scan(&word); err != nil {
			break
		}
		rowID := unique + 1
		if _, err = insertWord.ExecContext(ctx, rowID, word); err != nil {
			break
		}
		if useFTS {
			_, err = tx.ExecContext(ctx, `INSERT INTO merged_word_fts(rowid,word) VALUES(?,?)`, rowID, word)
			if err != nil {
				break
			}
		}
		if useTrigram {
			_, err = tx.ExecContext(ctx, `INSERT INTO merged_word_trigram(rowid,word) VALUES(?,?)`, rowID, dict.Fold(word))
			if err != nil {
				break
			}
		}
		if unique%mergedBrowseAnchorStep == 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO merged_anchor(offset,word) VALUES(?,?)`, unique, word)
			if err != nil {
				break
			}
		}
		sections.Add(word)
		unique++
		if progress != nil && unique%3000 == 0 {
			err = progress(all+unique, max(1, all*2))
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = ordered.Err()
	}
	_ = ordered.Close()
	_ = insertWord.Close()
	if err != nil {
		_ = tx.Rollback()
		return browseResp{}, err
	}
	alphabet, err := json.Marshal(sections.Finish())
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO merged_meta(signature,total,alphabet_json) VALUES(?,?,?)`, signature.String(), unique, string(alphabet))
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO merged_source_meta(matched_dictionaries) VALUES(?)`, matched)
	}
	if err != nil {
		_ = tx.Rollback()
		return browseResp{}, err
	}
	if err := tx.Commit(); err != nil {
		return browseResp{}, err
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE merged_stage`); err != nil {
		return browseResp{}, err
	}
	if progress != nil {
		if err := progress(all*2, max(1, all*2)); err != nil {
			return browseResp{}, err
		}
	}
	completed = true
	_ = os.Chtimes(path, time.Now(), time.Now())
	if err := rememberMergedCacheGroup(path, q.Get("wildgroup"), useFTS, useTrigram); err != nil {
		return browseResp{}, err
	}
	if browseFindDiagnosticEnabled(ctx) {
		logx.System("headword_merged_browse cache=wildcard_results_built source_words=%d unique_words=%d dictionaries=%d elapsed_ms=%d",
			all, unique, matched, time.Since(started).Milliseconds())
	}
	return readMergedBrowsePage(path, q, cacheID)
}
