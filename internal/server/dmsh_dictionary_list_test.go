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
	return append(append([]dictMsg{copyEvents[0]}, rows...), copyEvents[len(copyEvents)-1])
}

func TestDictionaryListSharedByHTTPAndNativeWithoutRouter(t *testing.T) {
	s := newTestServer(t)
	// First list allows self-preparing fixtures to settle; then compare the
	// stable view produced by the two adapters.
	if _, err := s.listDictionaries(context.Background(), "", func(dictMsg) error { return nil }); err != nil {
		t.Fatal(err)
	}
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
