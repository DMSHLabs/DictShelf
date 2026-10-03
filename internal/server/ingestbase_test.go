// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// The panel's index chip: a request naming no feature, at a dictionary that has
// no database at all, prepares it - at the cheapest level, with nothing else
// built.
func TestIngestBaseIndexesAnUnpreparedDictionary(t *testing.T) {
	s, e := demandEntry(t)
	if prepared(e) {
		t.Fatal("setup: the fixture was supposed to be unprepared")
	}

	sse(t, s, "/api/ingest?dict="+e.ID)

	if !prepared(e) {
		t.Fatal("the index chip left the dictionary unprepared")
	}
	if f := s.currentFeatures(e); f.FullText || f.Contains || f.Media {
		t.Errorf("the cheap index built more than headwords: %+v", f)
	}
}

// A feature sent is a state, and one left out keeps its value: turning full
// text off leaves contains where it was.
func TestIngestFullTextOffKeepsContains(t *testing.T) {
	s := newTestServer(t)
	id := getDicts(t, s, "/api/dicts")[0].ID
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	sse(t, s, "/api/ingest?dict="+id+"&fts=1&contains=1")
	if f := s.currentFeatures(e); !f.FullText || !f.Contains {
		t.Fatalf("setup: %+v, want full text and contains", f)
	}

	sse(t, s, "/api/ingest?dict="+id+"&fts=off")

	f := s.currentFeatures(e)
	if f.FullText {
		t.Error("fts=off must remove full text")
	}
	if !f.Contains {
		t.Error("a parameter that was not sent must keep its value")
	}
}

// Every boolean parameter reads the same spellings, and "sent" is what lets a
// caller keep a state the request did not mention.
func TestQueryFlag(t *testing.T) {
	for raw, want := range map[string]struct{ on, sent bool }{
		"":           {false, false},
		"x=1":        {true, true},
		"x=true":     {true, true},
		"x=yes":      {true, true},
		"x=0":        {false, true},
		"x=false":    {false, true},
		"x=No":       {false, true},
		"x=off":      {false, true},
		"x":          {false, true},
		"x=&y=1":     {false, true},
		"y=1":        {false, false},
		"x=%20on%20": {true, true},
	} {
		on, sent := queryFlag(mustQuery(t, raw), "x")
		if on != want.on || sent != want.sent {
			t.Errorf("%q: on=%v sent=%v, want %+v", raw, on, sent, want)
		}
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// Every stream - the ingest's events included - tells a reverse proxy not to
// buffer it (respStream), and an event keeps the SSE wire format: event line,
// data line, blank line.
func TestIngestStreamIsUnbuffered(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest("GET", "/api/ingest?dict="+s.reg.all()[0].ID, nil))
	if got := rec.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: done\ndata: {") || !strings.HasSuffix(body, "}\n\n") {
		t.Errorf("not an SSE event: %q", body)
	}
}

// An index change is the dictionary's job, not the request's: while it runs
// the list row says so, and a second request - a reloaded page - follows the
// running job and gets its outcome instead of starting another.
func TestIngestIsAJobAPageCanFollow(t *testing.T) {
	s := newTestServer(t)
	t.Cleanup(s.jobs.wg.Wait)
	id := s.reg.all()[0].ID
	release, ran := make(chan struct{}), make(chan struct{}, 2)
	s.jobs.start(ingestKey(id), 0, jobStatus{}, func(j *job) {
		ran <- struct{}{}
		j.update(func(js *jobStatus) { js.Done, js.Total = 5, 10 })
		<-release
		j.update(func(js *jobStatus) { js.Result = dictInfo{ID: id, Name: "followed"} })
	})
	<-ran

	var row struct {
		Job *jobProgress `json:"job"`
	}
	waitUntil(t, "the row to show the job", func() bool {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, newRequest("GET", "/api/dicts", nil))
		for _, l := range strings.Split(rec.Body.String(), "\n") {
			var m struct {
				T    string          `json:"t"`
				Dict json.RawMessage `json:"dict"`
			}
			if json.Unmarshal([]byte(l), &m) == nil && m.T == "dict" {
				json.Unmarshal(m.Dict, &row)
			}
		}
		return row.Job != nil && row.Job.Total == 10
	})

	got := make(chan string, 1)
	attached := make(chan struct{})
	go func() {
		rec := &signalRecorder{ResponseRecorder: httptest.NewRecorder(), first: attached}
		s.ServeHTTP(rec, newRequest("GET", "/api/ingest?dict="+id+"&fts=1", nil))
		got <- rec.Body.String()
	}()
	<-attached // it has the running job's progress: it is following that job
	close(release)
	select {
	case body := <-got:
		if !strings.Contains(body, `"name":"followed"`) || !strings.Contains(body, "event: done") {
			t.Errorf("the second request did not follow the running job: %q", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the follower never finished")
	}
	select {
	case <-ran:
		t.Error("the second request started a job of its own")
	default:
	}
}

// signalRecorder closes first on the first flushed write: a stream has started.
type signalRecorder struct {
	*httptest.ResponseRecorder
	first chan struct{}
	once  sync.Once
}

func (r *signalRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.once.Do(func() { close(r.first) })
}
