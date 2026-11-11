// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
)

func readDictEvents(t *testing.T, body []byte) []dictMsg {
	t.Helper()
	var events []dictMsg
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte{'\n'}) {
		var event dictMsg
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("decode dictionary event: %v (%s)", err, line)
		}
		events = append(events, event)
	}
	return events
}

func canonicalDictEvents(events []dictMsg) []dictMsg {
	copyEvents := append([]dictMsg(nil), events...)
	if len(copyEvents) < 2 {
		return copyEvents
	}
	rows := append([]dictMsg(nil), copyEvents[1:len(copyEvents)-1]...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Dict.ID < rows[j].Dict.ID })
	last := copyEvents[len(copyEvents)-1]
	/* What the two adapters are being compared on is the LIST: the rows, in a
	   settled order, and the validator. Free space and the volume name describe
	   the disk, not the dictionary list, and they are read per response - any
	   write anywhere in the milliseconds between the two calls moves them, which
	   is how a background pass (a sidecar, the startup sweep, a build's
	   articles.db) came to look like the transports disagreeing. */
	last.FreeBytes, last.LibraryVolume = 0, ""
	return append(append([]dictMsg{copyEvents[0]}, rows...), last)
}

func TestDictionaryListSharedByHTTPAndNativeWithoutRouter(t *testing.T) {
	// settled, not newTestServer: the registry measures every prepared
	// dictionary after it opens the backends, and that pass writes a stats.cache
	// into the prepared folder a row key includes - so the library is only at
	// rest once it is over (dictrows_test.go, settled). A single list is not
	// enough: it can land mid-pass and answer without a validator while the
	// next adapters answer with one, which reads as the transports disagreeing.
	s, _, _ := settled(t)
	httpRecorder := httptest.NewRecorder()
	s.ServeHTTP(httpRecorder, newRequest("GET", "/api/dicts", nil))
	httpEvents := readDictEvents(t, httpRecorder.Body.Bytes())

	var directEvents []dictMsg
	result, err := s.listDictionaries(context.Background(), "", func(event dictMsg) error {
		directEvents = append(directEvents, event)
		return nil
	})
	if err != nil || result.notModified {
		t.Fatalf("direct list: result=%+v error=%v", result, err)
	}

	mux := s.mux
	s.mux = http.NewServeMux()
	defer func() { s.mux = mux }()
	var nativeEvents []dictMsg
	if err := s.DesktopStream(context.Background(), "", "dicts", "", func(raw []byte) {
		var event dictMsg
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		nativeEvents = append(nativeEvents, event)
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalDictEvents(httpEvents), canonicalDictEvents(directEvents)) ||
		!reflect.DeepEqual(canonicalDictEvents(httpEvents), canonicalDictEvents(nativeEvents)) {
		t.Fatalf("dictionary list differs across transports:\nHTTP: %+v\nGo: %+v\nnative: %+v", httpEvents, directEvents, nativeEvents)
	}

	var tag string
	for _, event := range directEvents {
		if event.T == "end" {
			tag = event.ETag
		}
	}
	if tag == "" {
		t.Fatal("stable list did not include its validator")
	}
	seen := 0
	result, err = s.listDictionaries(context.Background(), tag, func(dictMsg) error {
		seen++
		return nil
	})
	if err != nil || !result.notModified || seen != 0 {
		t.Fatalf("matching validator: result=%+v events=%d error=%v", result, seen, err)
	}
}

func TestDictionaryListEmitterFailureCancelsWork(t *testing.T) {
	s := newTestServer(t)
	want := errors.New("stop stream")
	seen := 0
	_, err := s.listDictionaries(context.Background(), "", func(dictMsg) error {
		seen++
		return want
	})
	if !errors.Is(err, want) || seen != 1 {
		t.Fatalf("emitter error: got %v after %d events, want %v after begin", err, seen, want)
	}
}

// A request without a validator has nothing to decide up front, so the list
// streams row by row: no "checking N of M" pass, and the validator still rides
// on the end message for the next request. That up-front pass is what held the
// first row back - and a Browse word click waits for it before it can search.
func TestDictionaryListStreamsWithoutTheUpFrontCheck(t *testing.T) {
	s, _, _ := settled(t)
	var events []dictMsg
	if _, err := s.listDictionaries(context.Background(), "", func(event dictMsg) error {
		events = append(events, event)
		return nil
	}, true); err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, event := range events {
		switch event.T {
		case "checking":
			t.Fatalf("a list without a validator reported a checking pass: %+v", event)
		case "dict":
			rows++
		}
	}
	if len(events) < 2 || events[0].T != "begin" || events[len(events)-1].T != "end" {
		t.Fatalf("unexpected stream: first %q last %q", events[0].T, events[len(events)-1].T)
	}
	if rows != len(s.reg.all()) {
		t.Fatalf("streamed %d rows, registry holds %d", rows, len(s.reg.all()))
	}
	if tag := events[len(events)-1].ETag; tag == "" {
		t.Error("the streamed list carried no validator for the next request")
	}
}
