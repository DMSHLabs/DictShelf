// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/search"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// TestWildcardCachePerformance runs only when explicitly enabled. It uses real
// source dictionaries but isolates both the prepared library and result cache.
// Counters stay in memory during each phase and are printed only at phase end.
func TestWildcardCachePerformance(t *testing.T) {
	if os.Getenv("WUDICT_WILDCARD_BENCH") != "1" {
		t.Skip("set WUDICT_WILDCARD_BENCH=1 to run the real-dictionary Wildcards measurement")
	}
	root := os.Getenv("WUDICT_WILDCARD_BENCH_DIR")
	if root == "" {
		root = `D:\Dictionaries\wuDict\Dictionaries\Big`
	}
	selectors := strings.Split(os.Getenv("WUDICT_WILDCARD_BENCH_DICTS"), ";")
	selectors = nonEmptyStrings(selectors)
	if len(selectors) == 0 {
		t.Fatal("set WUDICT_WILDCARD_BENCH_DICTS to one or more source-name fragments separated by semicolons")
	}
	top := os.Getenv("WUDICT_WILDCARD_BENCH_TOP")
	if top == "" {
		top = "*a*"
	}
	within := os.Getenv("WUDICT_WILDCARD_BENCH_WITHIN")
	if within == "" {
		within = "*ing*"
	}
	if search.IsSimpleMask(within) {
		within = search.NormalizeSimpleMask(within)
	}
	withinFTS := os.Getenv("WUDICT_WILDCARD_BENCH_WITHIN_FTS")
	if withinFTS == "" {
		withinFTS = "h"
	}
	indexes := os.Getenv("WUDICT_WILDCARD_BENCH_INDEXES") != "0"
	fts := wildcardPerfIndexEnabled("WUDICT_WILDCARD_BENCH_FTS", indexes)
	trigram := wildcardPerfIndexEnabled("WUDICT_WILDCARD_BENCH_TRIGRAM", indexes)
	upperOnly := os.Getenv("WUDICT_WILDCARD_BENCH_UPPER_ONLY") == "1"
	skipCombine := os.Getenv("WUDICT_WILDCARD_BENCH_SKIP_COMBINE") == "1"

	isolatedDBDir(t)
	reg := unpreparedRegistry(t, []string{root})
	entries := reg.all()
	selected := make([]*entry, 0, len(selectors))
	seen := make(map[string]bool)
	for _, selector := range selectors {
		needle := strings.ToLower(strings.TrimSpace(selector))
		var match *entry
		for _, candidate := range entries {
			if !seen[candidate.ID] && strings.Contains(strings.ToLower(candidate.Path), needle) {
				match = candidate
				break
			}
		}
		if match == nil {
			t.Fatalf("no dictionary path contains %q under %s", selector, root)
		}
		seen[match.ID] = true
		selected = append(selected, match)
	}

	s := New(reg)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.collectWriteStats = true
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		for _, cached := range s.wildcardPages.entries {
			cached.mu.Lock()
			_ = cached.db.Close()
			cached.mu.Unlock()
		}
		s.wildcardPages.mu.Unlock()
	})

	opened := make([]dict.Dictionary, len(selected))
	for i, e := range selected {
		var err error
		opened[i], err = e.open()
		if err != nil {
			t.Fatalf("open %s: %v", e.Path, err)
		}
	}

	cacheID := wildcardPerfCacheID(t)
	group := "wildcard-perf-test"
	cacheFTS, cacheTrigram := fts, trigram
	topStarted := time.Now()
	topErrors := make([]error, len(selected))
	topCounts := make([]int, len(selected))
	var workers sync.WaitGroup
	workerCount := min(len(selected), indexconst.IndexDictionaryLimit(runtime.NumCPU()))
	jobs := make(chan int)
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				e := selected[i]
				request := browseFindStreamRequest{entry: e, dict: opened[i], query: top, typedQuery: top,
					matchMode: "wildcards", mask: true, cacheID: cacheID, cacheGroup: group,
					cacheOrder: i, cacheCount: len(selected), cacheFTS: cacheFTS, cacheTrigram: cacheTrigram}
				topErrors[i] = s.streamBrowseFindCached(context.Background(), request, func(event browseFindStreamEvent) error {
					topCounts[i] += len(event.Words)
					return nil
				})
			}
		}()
	}
	for i := range selected {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	for i, err := range topErrors {
		if err != nil {
			t.Fatalf("upper mask on %s: %v", selected[i].Path, err)
		}
	}
	upperStats := wildcardPerfTakeStats(t, s, cacheID)
	t.Logf("wildcard-perf stage=upper dictionaries=%d workers=%d words=%d elapsed=%s append_sum=%s tx_sum=%s lock_wait_sum=%s transactions=%d block_insert_sql=%d index_insert_sql=%d metadata_sql=%d metadata_time_sum=%s fts=%t trigram=%t",
		len(selected), workerCount, sumInts(topCounts), time.Since(topStarted).Round(time.Millisecond),
		upperStats.AppendElapsed.Round(time.Millisecond), upperStats.TransactionTime.Round(time.Millisecond),
		upperStats.LockWait.Round(time.Millisecond), upperStats.Transactions,
		upperStats.BlockStatements, upperStats.IndexStatements,
		upperStats.MetadataStatements, upperStats.MetadataTime.Round(time.Millisecond), fts, trigram)
	if sumInts(topCounts) == 0 {
		t.Fatalf("upper mask %q matched no words; choose a broader WUDICT_WILDCARD_BENCH_TOP", top)
	}
	if upperOnly {
		t.Log("wildcard-perf stopped after upper cache as requested")
		return
	}

	lowerRequest := browseFindStreamRequest{entry: selected[0], dict: opened[0], query: top, typedQuery: top,
		matchMode: "wildcards", mask: true, within: within, typedWithin: within,
		withinMode: "wildcards", withinMask: true, cacheID: cacheID, cacheGroup: group,
		cacheOrder: 0, cacheCount: len(selected), cacheFTS: cacheFTS, cacheTrigram: cacheTrigram}
	var trigramCandidates int
	if trigram {
		literal := mergedMaskTrigramLiteral(within)
		match := `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
		if err := s.wildcardPages.entries[cacheID].db.QueryRow(`SELECT COUNT(*) FROM wildcard_word_trigram WHERE wildcard_word_trigram MATCH ?`, match).Scan(&trigramCandidates); err != nil {
			t.Fatal(err)
		}
		if trigramCandidates <= 0 || trigramCandidates >= sumInts(topCounts) {
			t.Fatalf("trigram query %q is not selective on the upper cache: %d of %d candidates", within, trigramCandidates, sumInts(topCounts))
		}
	}
	lowerStarted := time.Now()
	if err := s.streamCachedWildcardGroupWithin(context.Background(), lowerRequest, func(browseFindStreamEvent) error {
		return nil
	}); err != nil {
		t.Fatalf("lower mask: %v", err)
	}
	lowerStats := wildcardPerfTakeStats(t, s, cacheID)
	t.Logf("wildcard-perf stage=lower-group dictionaries=%d words=%d elapsed=%s append_sum=%s tx_sum=%s lock_wait_sum=%s transactions=%d block_insert_sql=%d index_insert_sql=%d metadata_sql=%d metadata_time_sum=%s",
		len(selected), lowerStats.Words, time.Since(lowerStarted).Round(time.Millisecond),
		lowerStats.AppendElapsed.Round(time.Millisecond), lowerStats.TransactionTime.Round(time.Millisecond),
		lowerStats.LockWait.Round(time.Millisecond), lowerStats.Transactions,
		lowerStats.BlockStatements, lowerStats.IndexStatements,
		lowerStats.MetadataStatements, lowerStats.MetadataTime.Round(time.Millisecond))
	t.Logf("wildcard-perf stage=lower-trigram-candidates query=%q candidates=%d indexed=%t",
		within, trigramCandidates, trigram)

	ftsRequest := lowerRequest
	ftsRequest.within = withinFTS
	ftsRequest.typedWithin = withinFTS
	ftsRequest.withinMode = "prefix"
	ftsRequest.withinMask = false
	var ftsCandidates int
	if fts {
		match := mergedHeadwordFTSMatchMode(withinFTS, "prefix")
		if err := s.wildcardPages.entries[cacheID].db.QueryRow(`SELECT COUNT(*) FROM wildcard_word_fts WHERE wildcard_word_fts MATCH ?`, match).Scan(&ftsCandidates); err != nil {
			t.Fatal(err)
		}
		if ftsCandidates <= 0 || ftsCandidates >= sumInts(topCounts) {
			t.Fatalf("FTS prefix %q is not selective on the upper cache: %d of %d candidates", withinFTS, ftsCandidates, sumInts(topCounts))
		}
	}
	ftsStarted := time.Now()
	if err := s.streamCachedWildcardGroupWithin(context.Background(), ftsRequest, func(browseFindStreamEvent) error {
		return nil
	}); err != nil {
		t.Fatalf("lower FTS prefix %q: %v", withinFTS, err)
	}
	ftsStats := wildcardPerfTakeStats(t, s, cacheID)
	ftsMatches := int(ftsStats.Words)
	t.Logf("wildcard-perf stage=lower-fts-prefix query=%q matches=%d elapsed=%s indexed=%t append_sum=%s tx_sum=%s",
		withinFTS, ftsMatches, time.Since(ftsStarted).Round(time.Millisecond), fts, ftsStats.AppendElapsed.Round(time.Millisecond),
		ftsStats.TransactionTime.Round(time.Millisecond))
	t.Logf("wildcard-perf stage=lower-fts-candidates query=%q candidates=%d indexed=%t",
		withinFTS, ftsCandidates, fts)
	if ftsMatches == 0 {
		t.Fatalf("lower FTS prefix %q matched no words; choose a broader WUDICT_WILDCARD_BENCH_WITHIN_FTS", withinFTS)
	}
	if skipCombine {
		t.Log("wildcard-perf stopped after lower searches as requested")
		return
	}

	ids := make([]string, len(selected))
	for i := range selected {
		ids[i] = selected[i].ID
	}
	mergeQuery := url.Values{
		"sourcecache":   {cacheID},
		"wildgroup":     {group},
		"q":             {top},
		"mergedfts":     {boolQuery(fts)},
		"mergedtrigram": {boolQuery(trigram)},
	}
	mergeStarted := time.Now()
	merged, err := s.mergedBrowseFromWildcard(context.Background(), mergeQuery, ids, nil)
	if err != nil {
		t.Fatalf("combine result cache: %v", err)
	}
	mergeElapsed := time.Since(mergeStarted)
	basePath := filepath.Join(s.wildcardCacheDir(), "wildcard_merged_"+merged.mergedCacheID+".db")
	mergedInfo, err := os.Stat(basePath)
	if err != nil {
		t.Fatalf("stat combined cache: %v", err)
	}
	t.Logf("wildcard-perf stage=combine-base dictionaries=%d source_words=%d unique_words=%d elapsed=%s cache_bytes=%d fts=%t trigram=%t",
		len(selected), sumInts(topCounts), merged.Total, mergeElapsed.Round(time.Millisecond), mergedInfo.Size(), fts, trigram)

	findSignature := mergedFindSignature(merged.mergedCacheID, top, within, "wildcards", true, true)
	findPath := mergedFindCachePath(s.wildcardCacheDir(), findSignature)
	combinedFindStarted := time.Now()
	if err := ensureMergedFindCache(context.Background(), basePath, merged.mergedCacheID,
		findPath, findSignature, top, within, "wildcards", true, true, nil); err != nil {
		t.Fatalf("combine lower mask cache: %v", err)
	}
	findDB, err := store.OpenWildcardCacheDB(findPath)
	if err != nil {
		t.Fatal(err)
	}
	var combinedMatches int
	if err := findDB.QueryRow(`SELECT total FROM merged_find_meta LIMIT 1`).Scan(&combinedMatches); err != nil {
		_ = findDB.Close()
		t.Fatal(err)
	}
	if err := findDB.Close(); err != nil {
		t.Fatal(err)
	}
	findInfo, err := os.Stat(findPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("wildcard-perf stage=combine-lower matches=%d elapsed=%s cache_bytes=%d",
		combinedMatches, time.Since(combinedFindStarted).Round(time.Millisecond), findInfo.Size())
}

func wildcardPerfTakeStats(t *testing.T, s *Server, id string) wildcardCacheWriteStats {
	t.Helper()
	s.wildcardPages.mu.Lock()
	entry := s.wildcardPages.entries[id]
	s.wildcardPages.mu.Unlock()
	if entry == nil {
		t.Fatalf("Wildcards cache %s disappeared before its statistics snapshot", id)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	stats := entry.writeStats
	entry.writeStats = wildcardCacheWriteStats{}
	return stats
}

func wildcardPerfCacheID(t *testing.T) string {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw[:])
}

func wildcardPerfIndexEnabled(name string, fallback bool) bool {
	switch os.Getenv(name) {
	case "0":
		return false
	case "1":
		return true
	default:
		return fallback
	}
}

func nonEmptyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func boolQuery(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
