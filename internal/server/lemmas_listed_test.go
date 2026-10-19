// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/wuweidict/wudict/internal/lemmas"
	"github.com/wuweidict/wudict/internal/morph"
)

// countingCatalog serves catalogSite's manifest and counts how often it is
// read, which is what "drawn without the network" is a statement about.
func countingCatalog(t *testing.T) (url string, reads *int32, stop func()) {
	t.Helper()
	inner := newCatalogSite(t, "", nil)
	var n int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		resp, err := http.Get(inner.url)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()
		var v any
		json.NewDecoder(resp.Body).Decode(&v)
		json.NewEncoder(w).Encode(v)
	}))
	return ts.URL + "/manifest.json", &n, ts.Close
}

func getLemmasQ(t *testing.T, s *Server, q string) lemmaInfo {
	t.Helper()
	rec := do(t, s, "GET", "/api/lemmas"+q)
	if rec.Code != 200 {
		t.Fatalf("GET /api/lemmas%s: status %d: %s", q, rec.Code, rec.Body.String())
	}
	var info lemmaInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	return info
}

func hasRow(info lemmaInfo, code string) bool {
	for _, r := range info.Languages {
		if r.Code == code {
			return true
		}
	}
	return false
}

// The page's first answer asks no host; the list a host answered is kept on
// disk, drawn at once by the next run, and drawn again when the host cannot be
// reached - with the reason, and with when the list was read.
func TestLemmasListedWithoutNetwork(t *testing.T) {
	url, reads, stop := countingCatalog(t)
	s, dir := lemmaServer(t, url, 2)

	first := getLemmasQ(t, s, "?offline=1")
	if *reads != 0 || !first.Offline || hasRow(first, "pl") || first.Checked != "" {
		t.Fatalf("offline, nothing read yet: %d reads, %+v", *reads, first)
	}
	if !hasRow(first, "en") {
		t.Error("built-in English missing from the offline answer")
	}

	online := getLemmasQ(t, s, "")
	if *reads != 1 || online.Offline || !hasRow(online, "pl") || online.Checked == "" {
		t.Fatalf("online: %d reads, %+v", *reads, online)
	}
	if _, err := os.Stat(filepath.Join(dir, lemmas.CacheName)); err != nil {
		t.Fatalf("the catalogue was not kept: %v", err)
	}
	if local, _ := lemmas.Installed(dir); len(local) != 0 {
		t.Errorf("the kept catalogue reads as lemma data: %v", local)
	}

	// A restart: a new server over the same folder draws the kept list.
	s2, _ := lemmaServerIn(t, url, dir)
	again := getLemmasQ(t, s2, "?offline=1")
	if *reads != 1 || !hasRow(again, "pl") || again.Checked != online.Checked {
		t.Fatalf("after a restart: %d reads, %+v", *reads, again)
	}

	// The host gone: the answer that asks it says why, and still lists pl.
	stop()
	s3, _ := lemmaServerIn(t, url, dir)
	down := getLemmasQ(t, s3, "")
	if down.Error == "" || !hasRow(down, "pl") || down.Checked != online.Checked {
		t.Fatalf("host down: %+v", down)
	}
	// An install still asks the host: a kept list is drawn, never installed from.
	if rec := do(t, s3, "POST", "/api/lemmas?code=pl"); rec.Code != http.StatusBadGateway {
		t.Errorf("install with the host down: status %d, want 502", rec.Code)
	}
}

// Each language says how many of the user's dictionaries search with it, from
// the rows /api/dicts has derived.
func TestLemmasCountDictionaries(t *testing.T) {
	url, _, _ := countingCatalog(t)
	s, _ := lemmaServer(t, url, 2)
	s.dictRows.put("a", "k", dictInfo{LemmaLang: "pl"})
	s.dictRows.put("b", "k", dictInfo{LemmaLang: "pl"})
	s.dictRows.put("c", "k", dictInfo{LemmaLang: "de"})
	s.dictRows.put("d", "k", dictInfo{})
	info := getLemmasQ(t, s, "")
	if got := rowOf(t, info, "pl").Dicts; got != 2 {
		t.Errorf("pl dicts = %d, want 2", got)
	}
}

// lemmaServerIn is lemmaServer over a given lemma folder: the same folder a
// previous run wrote, which is what a restart reads.
func lemmaServerIn(t *testing.T, url, dir string) (*Server, string) {
	t.Helper()
	s, _ := lemmaServer(t, url, 2)
	s.LemmaDir = dir
	s.Morph = morph.New(2, dir)
	return s, dir
}
