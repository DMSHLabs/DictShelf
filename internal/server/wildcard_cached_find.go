// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/search"
)

// streamCachedWildcardWithin filters the completed upper-mask result blocks.
// Positions refer to that result list; no dictionary index is opened here.
func (s *Server) streamCachedWildcardWithin(ctx context.Context, request browseFindStreamRequest, send func(browseFindStreamEvent) error) error {
	diagnostic := browseFindDiagnosticEnabled(ctx)
	started := time.Now()
	typedWithin := request.within
	if isLemmaHeadwordMode(request.withinMode) {
		resolved, err := s.wildcardHeadwordLemmaQuery(request.entry, request.within, request.withinMode)
		if err != nil {
			return err
		}
		request.within = resolved
	}
	if diagnostic {
		logx.SystemDiagnostic("headword_find_wildcard_filter stage=start dict=%q dict_name=%q cache=%q upper_query=%q mode=%s typed_query=%q effective_queries=%q base_total_pending=true",
			request.entry.ID, request.entry.probeName(), request.cacheID, request.query, request.withinMode,
			typedWithin, strings.Split(request.within, mergedHeadwordAlternativeSeparator))
	}
	var matches func(string) bool
	if request.withinMask {
		re, err := search.CompileMask(request.within)
		if err != nil {
			return err
		}
		matches = re.MatchString
	} else {
		matches = headwordModeMatcher(request.within, request.withinMode)
	}
	cacheWithin := wildcardWithinCacheKey(request.within, request.withinMode)
	entry, token, err := s.wildcardPages.begin(request.cacheID, wildcardCacheSignature{
		query: request.query, group: request.cacheGroup, wildcards: request.mask,
		fts: request.cacheFTS, trigram: request.cacheTrigram,
	}, browsePageSize, request.cacheCount, request.entry.ID, request.entry.probeName(), request.cacheOrder,
		cacheWithin, request.withinMask, s.wildcardCacheDir(), request.diagnostics)
	if err != nil {
		return err
	}
	key := wildcardStreamKey{dictID: request.entry.ID, within: cacheWithin, withinMask: request.withinMask}
	defer s.wildcardPages.finish(entry, &key, token)
	var baseTotal, complete int
	err = entry.db.QueryRowContext(ctx, `SELECT total,complete FROM wildcard_dictionary
		WHERE within_text='' AND within_wildcards=0 AND dict_id=?`, request.entry.ID).Scan(&baseTotal, &complete)
	if errors.Is(err, sql.ErrNoRows) || err == nil && complete == 0 {
		return errors.New("upper wildcard result cache is not complete")
	}
	if err != nil {
		return err
	}
	var sections dict.MaskSections
	words := make([]string, 0, browsePageSize)
	positions := make([]int, 0, browsePageSize)
	found, scanned, scanTotal := 0, 0, baseTotal
	nextProgress := time.Now().Add(250 * time.Millisecond)
	streamBatchSize := browsePageSize * 10
	emit := func(done bool) error {
		event := browseFindStreamEvent{Type: "progress", Words: words, Positions: positions,
			Total: found, Scanned: scanned, ScanTotal: scanTotal, Size: browsePageSize, DiskCache: true, Done: done}
		if len(words) > 0 {
			event.Type = "block"
		}
		if done {
			event.Type = "done"
			event.Alphabet = sections.Finish()
			event.EffectiveQueries = splitMergedHeadwordAlternatives(request.within)
		}
		if err := entry.append(request.entry.ID, cacheWithin, request.withinMask, token, event); err != nil {
			return err
		}
		words = make([]string, 0, browsePageSize)
		positions = make([]int, 0, browsePageSize)
		return send(event)
	}
	index, match := "", ""
	if request.withinMask && entry.signature.trigram {
		if literal := mergedMaskTrigramLiteral(request.within); literal != "" {
			index, match = "wildcard_word_trigram", `"`+strings.ReplaceAll(literal, `"`, `""`)+`"`
		}
	} else if !request.withinMask && entry.signature.trigram && (request.withinMode == "suffix" || request.withinMode == "contains") {
		if literal := mergedTextTrigramLiteral(request.within); literal != "" {
			index, match = "wildcard_word_trigram", `"`+strings.ReplaceAll(literal, `"`, `""`)+`"`
		}
	} else if !request.withinMask && entry.signature.fts && request.withinMode != "suffix" && request.withinMode != "contains" {
		if expression := mergedHeadwordFTSMatchMode(request.within, request.withinMode); expression != "" {
			index, match = "wildcard_word_fts", expression
		}
	}
	if index != "" {
		var indexedTotal, candidateTotal int
		if err := entry.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wildcard_word WHERE dict_id=?`, request.entry.ID).Scan(&indexedTotal); err != nil {
			return err
		}
		if indexedTotal == baseTotal {
			countQuery := `SELECT COUNT(*) FROM ` + index + ` f CROSS JOIN wildcard_word w ON w.id=f.rowid WHERE ` + index + ` MATCH ? AND w.dict_id=?`
			if err := entry.db.QueryRowContext(ctx, countQuery, match, request.entry.ID).Scan(&candidateTotal); err != nil {
				return err
			}
			if candidateTotal < baseTotal {
				if diagnostic {
					logx.SystemDiagnostic("headword_find_wildcard_filter stage=plan dict=%q mode=%s typed_query=%q effective_queries=%q index=%s index_query=%q candidates=%d base_total=%d strategy=index_candidates",
						request.entry.ID, request.withinMode, typedWithin,
						strings.Split(request.within, mergedHeadwordAlternativeSeparator), index, match, candidateTotal, baseTotal)
				}
				scanTotal = candidateTotal
				// The cache has one SQLite connection. Read a bounded batch, close
				// its cursor, and only then append filtered result blocks.
				const batchSize = 10000
				// CROSS JOIN keeps the selective FTS scan outermost. With a
				// regular JOIN, SQLite can instead walk every dictionary word
				// in base-position order and probe FTS once per word.
				query := `SELECT w.base_position,w.word FROM ` + index + ` f CROSS JOIN wildcard_word w ON w.id=f.rowid WHERE ` + index + ` MATCH ? AND w.dict_id=? AND w.base_position>? ORDER BY w.base_position LIMIT ?`
				type candidateWord struct {
					position int
					word     string
				}
				lastPosition := -1
				for scanned < candidateTotal {
					if err := ctx.Err(); err != nil {
						return err
					}
					rows, err := entry.db.QueryContext(ctx, query, match, request.entry.ID, lastPosition, batchSize)
					if err != nil {
						return err
					}
					candidates := make([]candidateWord, 0, min(batchSize, candidateTotal-scanned))
					for rows.Next() {
						var candidate candidateWord
						if err := rows.Scan(&candidate.position, &candidate.word); err != nil {
							_ = rows.Close()
							return err
						}
						candidates = append(candidates, candidate)
					}
					if err := rows.Err(); err != nil {
						_ = rows.Close()
						return err
					}
					if err := rows.Close(); err != nil {
						return err
					}
					if len(candidates) == 0 {
						return errors.New("wildcard index candidates changed during search")
					}
					lastPosition = candidates[len(candidates)-1].position
					for _, candidate := range candidates {
						scanned++
						if matches(candidate.word) {
							words = append(words, candidate.word)
							positions = append(positions, candidate.position)
							sections.Add(candidate.word)
							found++
							if found == 1 || len(words) >= streamBatchSize {
								if err := emit(false); err != nil {
									return err
								}
							}
						}
						if len(words) >= streamBatchSize || time.Now().After(nextProgress) {
							if err := emit(false); err != nil {
								return err
							}
							nextProgress = time.Now().Add(250 * time.Millisecond)
						}
					}
				}
				if err := emit(true); err != nil {
					return err
				}
				if diagnostic {
					logx.SystemDiagnostic("headword_find_wildcard_filter stage=done dict=%q mode=%s typed_query=%q effective_queries=%q strategy=index_candidates scanned=%d candidates=%d found=%d elapsed_ms=%d",
						request.entry.ID, request.withinMode, typedWithin,
						strings.Split(request.within, mergedHeadwordAlternativeSeparator), scanned, candidateTotal, found, time.Since(started).Milliseconds())
				}
				return nil
			}
			if diagnostic {
				logx.SystemDiagnostic("headword_find_wildcard_filter stage=plan dict=%q mode=%s typed_query=%q effective_queries=%q index=%s index_query=%q candidates=%d base_total=%d strategy=full_block_scan reason=index_not_selective",
					request.entry.ID, request.withinMode, typedWithin,
					strings.Split(request.within, mergedHeadwordAlternativeSeparator), index, match, candidateTotal, baseTotal)
			}
		} else if diagnostic {
			logx.SystemDiagnostic("headword_find_wildcard_filter stage=plan dict=%q mode=%s typed_query=%q effective_queries=%q index=%s strategy=full_block_scan reason=index_word_count_mismatch indexed_words=%d base_total=%d",
				request.entry.ID, request.withinMode, typedWithin,
				strings.Split(request.within, mergedHeadwordAlternativeSeparator), index, indexedTotal, baseTotal)
		}
	}
	if diagnostic && index == "" {
		logx.SystemDiagnostic("headword_find_wildcard_filter stage=plan dict=%q mode=%s typed_query=%q effective_queries=%q strategy=full_block_scan reason=no_usable_index base_total=%d",
			request.entry.ID, request.withinMode, typedWithin,
			strings.Split(request.within, mergedHeadwordAlternativeSeparator), baseTotal)
	}
	for block := 1; scanned < baseTotal; block++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var encoded string
		var start, count int
		err := entry.db.QueryRowContext(ctx, `SELECT words_json,start_offset,word_count FROM wildcard_block
			WHERE within_text='' AND within_wildcards=0 AND dict_id=? AND block_no=?`, request.entry.ID, block).
			Scan(&encoded, &start, &count)
		if err != nil {
			return err
		}
		if start != scanned || count < 1 {
			return errors.New("upper wildcard result cache has a block gap")
		}
		var blockWords []string
		if err := json.Unmarshal([]byte(encoded), &blockWords); err != nil {
			return err
		}
		if len(blockWords) != count {
			return errors.New("upper wildcard result cache block length differs from metadata")
		}
		for i, word := range blockWords {
			if matches(word) {
				words = append(words, word)
				positions = append(positions, start+i)
				sections.Add(word)
				found++
				if found == 1 || len(words) >= streamBatchSize {
					scanned = start + i + 1
					if err := emit(false); err != nil {
						return err
					}
				}
			}
		}
		scanned = start + count
		if len(words) >= streamBatchSize || time.Now().After(nextProgress) {
			if err := emit(false); err != nil {
				return err
			}
			nextProgress = time.Now().Add(250 * time.Millisecond)
		}
	}
	if err := emit(true); err != nil {
		return err
	}
	if diagnostic {
		logx.SystemDiagnostic("headword_find_wildcard_filter stage=done dict=%q mode=%s typed_query=%q effective_queries=%q strategy=full_block_scan scanned=%d base_total=%d found=%d elapsed_ms=%d",
			request.entry.ID, request.withinMode, typedWithin,
			strings.Split(request.within, mergedHeadwordAlternativeSeparator), scanned, baseTotal, found, time.Since(started).Milliseconds())
	}
	return nil
}
