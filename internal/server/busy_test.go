// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
)

// slowOpen holds a ".slowfake" open, which says it is indexing, until the
// test closes it. Made by each run, before anything can open.
var slowOpen chan struct{}

func init() {
	dict.RegisterFormat(".slowfake", func(p string) (dict.Dictionary, error) {
		dict.SetActivity(p, dict.Activity{Phase: dict.PhaseIndexing, Done: 5, Total: 10})
		defer dict.ClearActivity(p)
		<-slowOpen
		return &fakeDict{words: []string{"a"}, path: p}, nil
	})
}

// TestDictsBusy: a row that keeps the list waiting is named on the stream,
// with what its open is doing, before its row lands - and nothing follows
// "end".
func TestDictsBusy(t *testing.T) {
	savedAfter, savedTick := busyAfter, busyTick
	busyAfter, busyTick = 0, 10*time.Millisecond
	defer func() { busyAfter, busyTick = savedAfter, savedTick }()
	slowOpen = make(chan struct{})
	dir := t.TempDir()
	for _, n := range []string{"quick.fake", "s.slowfake"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	isolatedDBDir(t)
	reg, err := NewRegistry([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	closeBackends(t, reg)
	var slowID string
	for _, e := range reg.all() {
		if filepath.Ext(e.Path) == ".slowfake" {
			slowID = e.ID
		}
	}
	if slowID == "" {
		t.Fatal("the slow dictionary was not found")
	}

	srv := httptest.NewServer(New(reg))
	defer srv.Close()
	release := func() {
		select {
		case <-slowOpen:
		default:
			close(slowOpen)
		}
	}
	defer release() // before srv.Close, which waits for the held handler
	req, _ := http.NewRequest("GET", srv.URL+"/api/dicts", nil)
	req.Host = "127.0.0.1:6888"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	released, rowSeen, ended := false, false, false
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		var m dictMsg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad line %q: %v", sc.Text(), err)
		}
		if ended {
			t.Fatalf("after end: %s", sc.Text())
		}
		switch m.T {
		case "busy":
			if released {
				continue // a frame already in flight when the row landed
			}
			want := busyRow{ID: slowID, Name: "s.slowfake", Phase: dict.PhaseIndexing, Done: 5, Total: 10}
			if len(m.Busy) != 1 || m.Busy[0] != want {
				t.Fatalf("busy %+v, want [%+v]", m.Busy, want)
			}
			released = true
			release()
		case "dict":
			if m.Dict.ID == slowID {
				if !released {
					t.Fatal("the slow row landed before it was named")
				}
				rowSeen = true
			}
		case "end":
			ended = true
		}
	}
	if !released || !rowSeen || !ended {
		t.Errorf("named %v, row %v, end %v", released, rowSeen, ended)
	}
}
