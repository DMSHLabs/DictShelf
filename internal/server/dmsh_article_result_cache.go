// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/ftsq"
	"github.com/DMSHLabs/DictShelf/internal/htmlref"
	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/search"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

type articleIDSource interface {
	StreamArticleIDs(context.Context, string, string, string, func([]int64, int, bool) error) error
	ArticlesByIDs(context.Context, []int64) ([]dict.Result, error)
}

func (s *Server) articleResultCacheSetup(request SearchRequest) (*entry, dict.Dictionary, articleIDSource, wildcardCacheSignature, string, error) {
	if !wildcardCacheIDPattern.MatchString(request.CacheID) || request.Dictionary == "" ||
		strings.Contains(request.Dictionary, ",") || strings.TrimSpace(request.Query) == "" ||
		len(request.Query) > 256 {
		return nil, nil, nil, wildcardCacheSignature{}, "", errors.New("invalid article result cache request")
	}
	count := max(1, request.CacheCount)
	if count > 10000 || request.CacheOrder < 0 || request.CacheOrder >= count {
		return nil, nil, nil, wildcardCacheSignature{}, "", errors.New("invalid article result cache layout")
	}
	mode, err := searchModeForArticleCache(request.Mode)
	if err != nil {
		return nil, nil, nil, wildcardCacheSignature{}, "", err
	}
	e, err := s.reg.get(request.Dictionary)
	if err != nil {
		return nil, nil, nil, wildcardCacheSignature{}, "", err
	}
	d, err := e.open()
	if err != nil {
		return nil, nil, nil, wildcardCacheSignature{}, "", err
	}
	source, ok := d.(articleIDSource)
	if !ok {
		return nil, nil, nil, wildcardCacheSignature{}, "", errors.New("this dictionary needs a prepared index to list all matching articles")
	}
	query := strings.TrimSpace(request.Query)
	if (mode == "wordLemma" || mode == "exactLemma") && !strings.Contains(query, "\x1f") {
		query = s.headwordLemmaQuery(e, d, query, mode)
	}
	path := backingDB(e.Path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, nil, wildcardCacheSignature{}, "", err
	}
	signature := wildcardCacheSignature{
		query: query,
		group: fmt.Sprintf("article-v1:%s:%s:%s:%t:%s:%d:%d", e.ID, mode, request.Rung,
			request.IgnoreDiacritics, path, info.Size(), info.ModTime().UnixNano()),
	}
	if count > 1 {
		// One search keeps every dictionary in one file, so the five-file
		// retention rule cannot evict its earlier dictionaries. Query and
		// lemma may differ by dictionary; the fresh cache ID identifies this
		// search, while dictionary rows keep their own ordered ID blocks.
		signature = wildcardCacheSignature{query: request.CacheID,
			group: fmt.Sprintf("article-group-v1:%s:%d:%s:%t", request.CacheID, count, mode, request.IgnoreDiacritics)}
	}
	return e, d, source, signature, query, nil
}

func searchModeForArticleCache(value string) (string, error) {
	switch value {
	// The strict full-text modes are the same index query with a different
	// reading: request.Rung says which, articleFullTextMatch builds that
	// reading's expression, and the archive verifies it in the article text - so
	// nothing downstream needs to know the difference.
	case "prefix", "suffix", "contains", "word", "wordLemma", "exact", "exactLemma", "wildcards", "fts", "phrase", "near":
		return value, nil
	default:
		return "", fmt.Errorf("invalid article result mode %q", value)
	}
}

func articleFullTextMatch(query, rung string) (string, error) {
	for _, candidate := range ftsq.Parse(query).RungsWith(search.NearWords()) {
		if candidate.Name == rung {
			return candidate.Match, nil
		}
	}
	return "", fmt.Errorf("full-text reading %q is unavailable", rung)
}

// fullTextReadingMode names the three article readings - the modes whose answer
// carries the server's highlight marks. They are one thing here for the same
// reason `searchModeForArticleCache` calls them one index query: the reader's
// Next/Previous walks those marks, so a page drawn without them cannot be walked
// at all. The headword modes are not here; their answer carries no marks.
func fullTextReadingMode(mode string) bool {
	return mode == "fts" || mode == "phrase" || mode == "near"
}

func (s *Server) searchArticleResultCache(ctx context.Context, request SearchRequest, emit func(SearchEvent) error) error {
	e, _, source, signature, query, err := s.articleResultCacheSetup(request)
	if err != nil {
		return err
	}
	// A dictionary whose full text comes from its archive has ONE reading of a
	// full-text query, and a paged answer has to say so too: the page the reader
	// is looking at is drawn from this path once the result cache is running, not
	// from the search that started it (search.ArchiveFullText; without this the
	// header claims a phrase was tried and not found).
	oneReading := false
	needsIndex := false
	if af, ok := source.(search.ArchiveFullText); ok {
		oneReading = af.FullTextFromArchive()
	}
	// The same fact for the strict readings: this source answers them with the
	// words reading, because it has no positions to verify a phrase with, so the
	// plate has to carry the line naming what is missing - and the page drawn
	// from this path (the cache, once it is running) has to carry it too.
	// search.archiveStrictDeclined is what the routing asks.
	//
	// The strict readings are named, not "everything but fts": a headword mode
	// has no full-text reading to decline, and a prefix answer on an archive
	// without our index must not carry a line offering to build one.
	if request.Mode == "phrase" || request.Mode == "near" {
		if ai, ok := source.(interface{ HasArticleIndex() bool }); !ok || !ai.HasArticleIndex() {
			needsIndex = oneReading
		}
	}
	// The reading that ANSWERED. The page drawn from this path marks its words
	// from this name, so a phrase answered with words has to say "words" or the
	// highlight marks the phrase that is not there (found on the live page).
	answerRung := request.Rung
	if needsIndex {
		answerRung = "words"
	}
	switch request.CacheAction {
	case "build":
		// The complete list is the one thing the plate can end up admitting it
		// cannot produce ("Complete list unavailable"), and until 2026-11-03 that
		// admission left no trace anywhere: the client shows a fixed line and does
		// not log, so the cause existed only inside the failed request. Two lines
		// close that. The DECLINED one fires when this pass is about to answer a
		// strict reading with the words reading because the source has no
		// positions and no article index of ours - the common, explainable case,
		// gated on the caller asking for diagnostics like every other line in this
		// area. The FAILED one is unconditional: an error nobody can reproduce is
		// worth a line in any log, and it is rare.
		if needsIndex {
			logx.SystemDiagnosticIf(request.SearchDiagnostics,
				"search_complete_list stage=declined dict=%q name=%q mode=%q rung=%q query=%q reason=%q",
				e.ID, e.probeName(), request.Mode, request.Rung, query,
				"the source answers full text itself and this dictionary has no article index of ours")
		}
		if err := func() error {
			match := ""
			if fullTextReadingMode(request.Mode) {
				match, err = articleFullTextMatch(query, request.Rung)
				if err != nil {
					return err
				}
			}
			entry, token, err := s.wildcardPages.begin(request.CacheID, signature, indexconst.BrowsePageSize,
				max(1, request.CacheCount), e.ID, e.probeName(), request.CacheOrder, "", false, s.wildcardCacheDir(), request.SearchDiagnostics)
			if err != nil {
				return err
			}
			key := wildcardStreamKey{dictID: e.ID}
			defer s.wildcardPages.finish(entry, &key, token)
			if request.IgnoreDiacritics {
				ctx = store.WithIgnoreDiacritics(ctx)
			}
			started := time.Now()
			logx.SystemDiagnosticIf(request.SearchDiagnostics,
				"search stage=article_id_index_read dict=%q mode=%q query=%q source=prepared_sql_index cache=building", e.ID, request.Mode, query)
			err = source.StreamArticleIDs(ctx, query, request.Mode, match, func(ids []int64, total int, done bool) error {
				words := make([]string, len(ids))
				for i, id := range ids {
					words[i] = strconv.FormatInt(id, 10)
				}
				if err := entry.append(e.ID, "", false, token, browseFindStreamEvent{Words: words, Total: total, Done: done}); err != nil {
					return err
				}
				return emit(SearchEvent{T: "cache", Dict: e.ID, CacheID: request.CacheID, Total: total, Complete: done})
			})
			logx.SystemDiagnosticIf(request.SearchDiagnostics,
				"search stage=article_id_index_done dict=%q mode=%q query=%q elapsed_ms=%d error=%v", e.ID, request.Mode, query, time.Since(started).Milliseconds(), err)
			return err
		}(); err != nil {
			logx.SystemDiagnostic("search_complete_list stage=failed dict=%q name=%q mode=%q rung=%q query=%q cache_id=%q error=%q",
				e.ID, e.probeName(), request.Mode, request.Rung, query, request.CacheID, err)
			return err
		}
		return nil
	case "page":
		if request.Start < 0 || request.Size < 1 || request.Size > 500 {
			return errors.New("invalid article result portion")
		}
		const blockSize = indexconst.BrowsePageSize
		var total int
		var complete bool
		started := time.Now()
		deadline := time.NewTimer(5 * time.Minute)
		defer deadline.Stop()
		for {
			block, found, readErr := s.wildcardPages.page(request.CacheID, signature, e.ID, "", false, 1, request.CacheOrder, max(1, request.CacheCount))
			if readErr != nil {
				return readErr
			}
			if found {
				total, complete = block.Total, !block.Partial
				if total >= request.Start+request.Size || complete {
					break
				}
			} else if time.Since(started) > 30*time.Second {
				return errors.New("article result cache is unavailable")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				return errors.New("article result portion is not ready")
			case <-time.After(80 * time.Millisecond):
			}
		}
		end := min(request.Start+request.Size, total)
		if request.Start >= end {
			return emit(SearchEvent{T: "hit", Dict: e.ID, CacheID: request.CacheID, Total: total,
				Complete: complete, Start: request.Start, HasNext: !complete})
		}
		ids := make([]int64, 0, end-request.Start)
		for page := request.Start/blockSize + 1; page <= (end-1)/blockSize+1; page++ {
			block, found, readErr := s.wildcardPages.page(request.CacheID, signature, e.ID, "", false, page, request.CacheOrder, max(1, request.CacheCount))
			if readErr != nil {
				return readErr
			}
			if !found {
				return errors.New("article result block is not available")
			}
			for i, value := range block.Words {
				position := (page-1)*blockSize + i
				if position < request.Start || position >= end {
					continue
				}
				id, parseErr := strconv.ParseInt(value, 10, 64)
				if parseErr != nil {
					return parseErr
				}
				ids = append(ids, id)
			}
		}
		logx.SystemDiagnosticIf(request.SearchDiagnostics,
			"search stage=article_id_cache_read dict=%q result=hit start=%d ids=%d total=%d", e.ID, request.Start, len(ids), total)
		articleReadStarted := time.Now()
		results, err := source.ArticlesByIDs(ctx, ids)
		if err != nil {
			return err
		}
		logx.SystemDiagnosticIf(request.SearchDiagnostics,
			"search stage=article_sql_read dict=%q source=prepared_sql_articles ids=%d elapsed_ms=%d", e.ID, len(ids), time.Since(articleReadStarted).Milliseconds())
		format, err := parseFormat(request.Format)
		if err != nil {
			return err
		}
		hl, marking := marker(request.Highlight && fullTextReadingMode(request.Mode))(query, request.Rung)
		var assetTokens func(int64, string) (string, bool)
		if len(results) > 0 {
			if d, openErr := e.open(); openErr == nil {
				if resolver, ok := d.(interface {
					AssetToken(int64, string) (string, bool)
				}); ok {
					assetTokens = resolver.AssetToken
				}
			}
		}
		for i := range results {
			s.decorateFavourite(e.ID, &results[i])
			if s.isFavouriteDictionaryID(e.ID) && len(results[i].ResourceMap) > 0 {
				results[i].Body = RewriteFavouriteHTMLMarked(results[i].Body, e.ID, results[i].ResourceMap, hl)
			} else {
				results[i].Body = rewriteEntryHTMLFor(results[i].Body, e.ID, results[i].EntryID, hl, assetTokens)
			}
			if marking != nil {
				results[i].Body = markArticleFindHTML(results[i].Body, *marking)
			}
		}
		var styles htmlref.Styles
		if format != formatRaw && len(results) > 0 {
			styles = s.articleStyles(e.ID, results[0].Body)
		}
		for i := range results {
			results[i].Body = applyFormat(results[i].Body, format, request.Origin, styles)
		}
		return emit(SearchEvent{T: "hit", Dict: e.ID, Results: results, CacheID: request.CacheID,
			Total: total, Complete: complete, Start: request.Start, HasNext: end < total || !complete,
			Rung: answerRung, OneReading: oneReading, NeedsIndex: needsIndex, Term: query})
	default:
		return fmt.Errorf("invalid article result cache action %q", request.CacheAction)
	}
}
