// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

// A complete-list build that fails says so in the system log, with the dictionary
// and the reading that failed. The result card only ever shows one fixed line for
// this ("Complete list unavailable", search.fullListUnavailable), and the reason
// lives in the failed request - so until 2026-11-03 a failure was reproducible
// only by guessing. The failure here is deterministic: an unknown rung cannot
// build its FTS expression, which is enough to exercise the reporting.
func TestAFailedCompleteListBuildIsLogged(t *testing.T) {
	s, id := newHLServer(t)
	s.User = UserDir(t.TempDir())
	s.wildcardPages.init(s.wildcardCacheDir())
	t.Cleanup(func() {
		s.wildcardPages.mu.Lock()
		defer s.wildcardPages.mu.Unlock()
		for _, entry := range s.wildcardPages.entries {
			_ = entry.db.Close()
		}
	})
	before, err := store.IndexLog()
	if err != nil {
		t.Fatal(err)
	}
	// Raw rather than through the stream helper: the status of a failed stream is
	// not what this test is about, the log line is.
	s.ServeHTTP(httptest.NewRecorder(), newRequest("GET", "/api/search?q=vivienda&mode=fts&rung=not-a-reading&dict="+id+
		"&cacheID=logged-failure-0001&cacheAction=build", nil))
	after, err := store.IndexLog()
	if err != nil {
		t.Fatal(err)
	}
	fresh := string(after)
	if len(before) < len(fresh) {
		fresh = fresh[len(before):] // only what this request appended
	}
	if !strings.Contains(fresh, "search_complete_list stage=failed") {
		t.Fatalf("a failed complete-list build left no trace in the system log:\n%s", fresh)
	}
	for _, want := range []string{id, "not-a-reading", "error="} {
		if !strings.Contains(fresh, want) {
			t.Errorf("the line does not name %q:\n%s", want, fresh)
		}
	}
}
