// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/search"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// One message for two situations was the finding: a dictionary that is merely
// late was told to index itself. The decision is what these tests pin - which
// situation a dictionary is in when a search of it ran out of time.
func TestWhichDictionaryRanOutOfTime(t *testing.T) {
	s := newTestServer(t) // prepared, with the contains index, WITHOUT full text
	id := getDicts(t, s, "/api/dicts")[0].ID
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	if searchIndexMissingFor(e, search.Prefix) {
		t.Error("a prepared dictionary has nothing to answer a headword mode from")
	}
	if searchIndexMissingFor(e, search.Contains) {
		t.Error("the fixture is built with the contains index")
	}
	if !searchIndexMissingFor(e, search.FullText) {
		t.Error("a dictionary with no article index and no archive index claims to answer full text")
	}

	// The archive answering full text itself is the case the user's Wikipedias
	// were in: nothing of ours, and still nothing to index.
	textDB, ok := validPrepared(e.Path)
	if !ok {
		t.Fatal("the fixture is not prepared")
	}
	db, err := sql.Open(sqliteDriver(), textDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('` + store.OwnFullTextKey + `', '1')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if searchIndexMissingFor(e, search.FullText) {
		t.Error("a dictionary whose archive answers full text was told to index itself")
	}
	if searchIndexMissingFor(e, search.Phrase) {
		t.Error("a dictionary whose archive answers full text was told to index itself (a strict reading)")
	}
}

// And the words themselves: two situations, two sentences.
func TestTheTimeoutSaysWhichSituationItIs(t *testing.T) {
	if got := searchTimeoutMessage(true); got != "timed out — index this dictionary to search it" {
		t.Errorf("no index: %q", got)
	}
	if got := searchTimeoutMessage(false); got != "timed out — search this dictionary alone to allow more time" {
		t.Errorf("prepared and late: %q", got)
	}
}

// A dictionary with nothing prepared is the first situation whatever the mode.
func TestADictionaryWithNoIndexAtAllRanOutOfTimeUnprepared(t *testing.T) {
	s, e := unpreparedEntry(t)
	_ = s
	for _, mode := range []search.Mode{search.Prefix, search.Contains, search.FullText, search.WholeWord} {
		if !searchIndexMissingFor(e, mode) {
			t.Errorf("%s: a dictionary with no prepared database claims to answer it", mode)
		}
	}
}

// And the wire, which is where it matters: the row's error is what the reader
// sees. The deadline is the request's OWN context, expired before the search
// starts - the handler derives its budget from it - so nothing here waits thirty
// seconds, and the two rows below differ only in what the dictionary has.
func TestATimedOutDictionarySaysWhichSituationItIs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) (*Server, string)
		want  string
	}{
		{"prepared, merely late", func(t *testing.T) (*Server, string) {
			s := newTestServer(t)
			return s, getDicts(t, s, "/api/dicts")[0].ID
		}, "timed out — search this dictionary alone to allow more time"},
		{"nothing prepared to answer from", func(t *testing.T) (*Server, string) {
			s, e := unpreparedEntry(t)
			return s, e.ID
		}, "timed out — index this dictionary to search it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, id := tc.setup(t)
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, newRequest("GET", "/api/search?q=casa&mode=prefix&dict="+id, nil).WithContext(ctx))
			var hit *streamMsg
			for _, m := range decodeStream(t, rec) {
				if m.T == "hit" {
					hit = &m
				}
			}
			if hit == nil {
				t.Fatalf("no dictionary row came back: %s", rec.Body.String())
			}
			if hit.Error != tc.want {
				t.Errorf("row error = %q, want %q", hit.Error, tc.want)
			}
		})
	}
}

// decodeStream reads one /api/search NDJSON response.
func decodeStream(t *testing.T, rec *httptest.ResponseRecorder) []streamMsg {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out []streamMsg
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var m streamMsg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad NDJSON line (%v): %s", err, line)
		}
		out = append(out, m)
	}
	return out
}
