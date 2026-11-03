// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A dictionary whose search produced a deadline is told it did not finish - and
// nothing else. It is never told to index itself: a dictionary with no index is
// not searched at all (the opener refuses it, which the next test pins), so that
// advice could only ever be wrong. The deadline here is the request's OWN
// context, expired before the search starts, so nothing waits for a clock - and
// the search no longer has one of its own (indexconst).
func TestATimedOutDictionaryIsNotToldToIndexItself(t *testing.T) {
	s := newTestServer(t) // prepared: it has an index, which is the point
	id := getDicts(t, s, "/api/dicts")[0].ID
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	row := searchRow(t, s, id, ctx)
	if row.Error != searchDidNotFinish {
		t.Errorf("row error = %q, want %q", row.Error, searchDidNotFinish)
	}
	if strings.Contains(row.Error, "index this dictionary") {
		t.Error("a dictionary that has an index was told to index itself")
	}
}

// The rule that makes that advice impossible: a dictionary whose index is not
// there is refused BEFORE anything is searched, with one message - the one the
// client translates (errors.indexNotReady) - and no results.
func TestADictionaryWithoutAnIndexIsNotSearched(t *testing.T) {
	s, e := unpreparedEntry(t)
	row := searchRow(t, s, e.ID, context.Background())
	if row.Error != errIndexNotReady.Error() {
		t.Errorf("row error = %q, want %q", row.Error, errIndexNotReady)
	}
	if len(row.Results) != 0 {
		t.Errorf("a dictionary with no index answered with %d result(s)", len(row.Results))
	}
}

// searchRow runs one /api/search and returns the dictionary's row.
func searchRow(t *testing.T, s *Server, id string, ctx context.Context) streamMsg {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest("GET", "/api/search?q=casa&mode=prefix&dict="+id, nil).WithContext(ctx))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var m streamMsg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad NDJSON line (%v): %s", err, line)
		}
		if m.T == "hit" {
			return m
		}
	}
	t.Fatalf("no dictionary row came back: %s", rec.Body.String())
	return streamMsg{}
}
