// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http/httptest"

	// The server package registers no formats - the CLI does. memlimit_test.go
	// carries these under !windows (its sweep is unix-only), so without them
	// here a Windows test binary discovers only the formats another test file
	// happens to have pulled in, and a broken .mdx finds no row at all.
	_ "github.com/wuweidict/wudict/internal/format/bgl"
	_ "github.com/wuweidict/wudict/internal/format/mdx"
	_ "github.com/wuweidict/wudict/internal/format/slob"
	_ "github.com/wuweidict/wudict/internal/format/stardict"
	_ "github.com/wuweidict/wudict/internal/format/zim"

	"github.com/wuweidict/wudict/internal/store"
)

// dictsWith requests /api/dicts with an optional If-None-Match and returns the
// status, the rows and the validator the `end` frame carried.
func dictsWith(t *testing.T, s *Server, inm string) (int, []dictInfo, string) {
	t.Helper()
	req := newRequest("GET", "/api/dicts", nil)
	if inm != "" {
		req.Header.Set("If-None-Match", inm)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var rows []dictInfo
	tag := ""
	sc := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	for sc.Scan() {
		var m dictMsg
		if line := strings.TrimSpace(sc.Text()); line != "" {
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("bad NDJSON line (%v): %s", err, line)
			}
		}
		switch m.T {
		case "dict":
			rows = append(rows, *m.Dict)
		case "end":
			tag = m.ETag
		}
	}
	return rec.Code, rows, tag
}

// An unchanged library is answered with a 304 and no body; a source file that
// changes on disk changes the validator and the row is derived again.
func TestDictsRevalidation(t *testing.T) {
	s := newTestServer(t)
	// A DSL prepares itself on its first open, which this first list does:
	// a validator for it would describe the files from before that, so it
	// carries none. Revalidation is about the list after it.
	if _, _, first := dictsWith(t, s, ""); first != "" {
		t.Fatalf("the answer that prepared its dictionary carried a tag: %s", first)
	}
	code, rows, tag := dictsWith(t, s, "")
	if code != 200 || len(rows) != 1 || tag == "" {
		t.Fatalf("first answer: %d, %d rows, tag %q", code, len(rows), tag)
	}

	req := newRequest("GET", "/api/dicts", nil)
	req.Header.Set("If-None-Match", tag)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 304 || rec.Body.Len() != 0 {
		t.Fatalf("unchanged list: %d with %d bytes, want 304 and no body", rec.Code, rec.Body.Len())
	}
	for _, inm := range []string{"W/" + tag, `"other", ` + tag, "*"} {
		if code, _, _ := dictsWith(t, s, inm); code != 304 {
			t.Errorf("If-None-Match %s: %d, want 304", inm, code)
		}
	}

	// the cached row is the row
	_, again, tag2 := dictsWith(t, s, `"stale"`)
	if tag2 != tag {
		t.Errorf("an unchanged list changed its tag: %s → %s", tag, tag2)
	}
	a, _ := json.Marshal(rows[0])
	b, _ := json.Marshal(again[0])
	if string(a) != string(b) {
		t.Errorf("a cached row differs from the derived one:\n%s\n%s", a, b)
	}

	// an edit in place: same path, new content and mtime
	src := rows[0].Path
	edited := strings.Replace(sampleDSL, "Server Test Dict", "Edited Dict", 1)
	if err := os.WriteFile(src, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(src, later, later); err != nil {
		t.Fatal(err)
	}
	code, rows, tag3 := dictsWith(t, s, tag)
	if code != 200 || tag3 == tag {
		t.Fatalf("an edited source: %d, tag %s (was %s); want 200 and a new tag", code, tag3, tag)
	}
	// the open backend is the previous edition's until a rescan (revalidate),
	// so what the re-derived row says is that its prepared data is behind
	if len(rows) != 1 || !rows[0].Outdated {
		t.Errorf("the edited source did not re-derive its row: %+v", rows)
	}
}

// A row that carries an error may be gone on the next try, so the answer it
// is part of is never offered for keeping.
func TestDictsErrorRowIsNotKept(t *testing.T) {
	isolatedDBDir(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.mdx"), []byte("not an mdx file"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, reg)
	s := New(reg)
	_, rows, tag := dictsWith(t, s, "")
	if len(rows) != 1 || rows[0].Error == "" {
		t.Fatalf("setup: want one row with an error, got %+v", rows)
	}
	if tag != "" {
		t.Errorf("an answer with an error row carried tag %s", tag)
	}
}

func TestMatchesTag(t *testing.T) {
	for _, c := range []struct {
		header string
		want   bool
	}{
		{`"a"`, true}, {`W/"a"`, true}, {`"b", "a"`, true}, {`*`, true},
		{`"b"`, false}, {`a`, false}, {``, false},
	} {
		if got := matchesTag(c.header, `"a"`); got != c.want {
			t.Errorf("matchesTag(%q) = %v, want %v", c.header, got, c.want)
		}
	}
}

// settled is newTestServer after its DSL has prepared itself, which the first
// list does: the answers from here on describe a library at rest.
func settled(t *testing.T) (*Server, dictInfo, string) {
	t.Helper()
	s := newTestServer(t)
	dictsWith(t, s, "")
	_, rows, tag := dictsWith(t, s, "")
	if len(rows) != 1 || tag == "" {
		t.Fatalf("setup: %d rows, tag %q", len(rows), tag)
	}
	return s, rows[0], tag
}

func wantFull(t *testing.T, s *Server, tag, why string) string {
	t.Helper()
	code, _, next := dictsWith(t, s, tag)
	if code != 200 {
		t.Fatalf("%s: %d, want 200 (the kept answer is no longer true)", why, code)
	}
	return next
}

// A row derived while its own derivation changed the disk - a DSL preparing
// itself on its first open - is not kept under the key from before: removing
// the prepared data later would bring that key back.
func TestDictsSelfPrepareIsNotKeptUnderOldKey(t *testing.T) {
	s := newTestServer(t)
	id := s.reg.all()[0].ID
	// Nor is the answer: its validator is the key from before, which the same
	// removal brings back, and a 304 then would vouch for the prepared row.
	if _, _, tag := dictsWith(t, s, ""); tag != "" {
		t.Errorf("the answer holding that row was offered for keeping: etag %s", tag)
	}
	if _, ok := s.dictRows.rows[id]; ok {
		t.Fatal("the row derived while the dictionary prepared itself was kept")
	}
	dictsWith(t, s, "")
	if _, ok := s.dictRows.rows[id]; !ok {
		t.Fatal("a row derived from a library at rest was not kept")
	}
}

// A change the app makes to prepared data invalidates the kept answer even
// where the filesystem clock could not tell (entry.gen, bumped by reconcile).
func TestDictsReconcileInvalidates(t *testing.T) {
	s, row, tag := settled(t)
	e, _ := s.reg.get(row.ID)
	before := e.gen.Load()
	sse(t, s, "/api/ingest?dict="+row.ID+"&contains=1")
	if e.gen.Load() == before {
		t.Fatal("a reconcile did not move gen")
	}
	wantFull(t, s, tag, "after an ingest")
}

// A reconcile that finds everything as wanted - the startup abbreviation sweep
// on a dictionary already current - changes nothing a row says, so the kept
// answer still stands.
func TestDictsQuietReconcileKeepsTag(t *testing.T) {
	s, row, _ := settled(t)
	e, _ := s.reg.get(row.ID)
	// The fixture's DSL was opened before it prepared itself; the first
	// reconcile swaps that backend for the prepared one, a real change to
	// what the row is derived from. The library is at rest after it.
	if _, err := e.reconcile(e.probeName(), store.Target{}, nil); err != nil {
		t.Fatal(err)
	}
	_, _, tag := dictsWith(t, s, "")
	if tag == "" {
		t.Fatal("no tag for a library at rest")
	}
	before := e.gen.Load()
	if _, err := e.reconcile(e.probeName(), store.Target{}, nil); err != nil {
		t.Fatal(err)
	}
	if e.gen.Load() != before {
		t.Error("a reconcile that changed nothing moved gen")
	}
	if code, _, _ := dictsWith(t, s, tag); code != 304 {
		t.Errorf("after a reconcile that changed nothing: %d, want 304", code)
	}
}

// /api/rescan answers in full even to a client holding the current etag: the
// rows the flush re-derives are the point of asking.
func TestDictsRescanIgnoresTag(t *testing.T) {
	s, _, tag := settled(t)
	req := newRequest("GET", "/api/rescan", nil)
	// the fork gates a rescan on a loopback client (removalOffered)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("If-None-Match", tag)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"t":"dict"`) {
		t.Errorf("rescan with the current etag: %d, rows sent %v; want 200 with rows",
			rec.Code, strings.Contains(rec.Body.String(), `"t":"dict"`))
	}
}

// A rescan that drops a backend the app had open - the source was replaced -
// invalidates the row derived while it was open, whoever called Rescan.
func TestDictsRescanInvalidatesChangedEntry(t *testing.T) {
	s, row, _ := settled(t)
	e, _ := s.reg.get(row.ID)
	if _, err := e.open(); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(3 * time.Second)
	if err := os.Chtimes(row.Path, later, later); err != nil {
		t.Fatal(err)
	}
	tag := wantFull(t, s, "", "after the edit")
	if err := s.reg.Rescan(); err != nil { // as an import or a removal does
		t.Fatal(err)
	}
	wantFull(t, s, tag, "after a rescan dropped the open backend")
}

// While an index is being built the answer is not offered for keeping: its
// rows say so, and that is over in a moment.
func TestDictsRunningJobIsNotKept(t *testing.T) {
	s, row, tag := settled(t)
	release := make(chan struct{})
	s.jobs.start(ingestKey(row.ID), 0, jobStatus{Total: 10}, func(*job) { <-release })
	defer close(release)
	code, rows, next := dictsWith(t, s, tag)
	if code != 200 || next != "" {
		t.Fatalf("with a job running: %d, tag %q; want 200 and no tag", code, next)
	}
	if rows[0].Job == nil {
		t.Error("the row does not report its running job")
	}
}
