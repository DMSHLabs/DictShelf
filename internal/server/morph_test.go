// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/morph"
)

// morphServer builds a registry over one temp folder holding the named DSL
// files, with lemmatization enabled. The file NAMES are the point: language
// detection reads them (prefix only), and the folder is a configured root so
// nothing above it is ever consulted.
func morphServer(t *testing.T, files map[string]string) *Server {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	isolatedDBDir(t)
	reg, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, reg)
	s := New(reg)
	// Only English is built in (D87), so Russian arrives the way a user's
	// Russian arrives: as a file in LEMMA_DIR. Keyed on the ё spellings, as
	// golem's own ru data is, so the е→ё retry is still what resolves "идет".
	lemmas := t.TempDir()
	if err := os.WriteFile(filepath.Join(lemmas, "ru.txt"), []byte("идти\tидёт\tидёшь\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Morph = morph.New(2, lemmas)
	return s
}

// stream returns EVERY message of an /api/search response, not just the hits:
// what is being asserted here is that a message type appears, or does not.
func stream(t *testing.T, s *Server, path string) []streamMsg {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest("GET", path, nil))
	if rec.Code != 200 {
		t.Fatalf("GET %s: status %d: %s", path, rec.Code, rec.Body.String())
	}
	var out []streamMsg
	sc := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m streamMsg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("GET %s: bad NDJSON line (%v): %s", path, err, line)
		}
		out = append(out, m)
	}
	return out
}

func morphMsgs(msgs []streamMsg) []streamMsg {
	var out []streamMsg
	for _, m := range msgs {
		if m.T == "morph" {
			out = append(out, m)
		}
	}
	return out
}

// dslFile is the smallest well-formed DSL holding one headword.
func dslFile(name string, words ...string) string {
	var b strings.Builder
	b.WriteString("#NAME \"" + name + "\"\n\n")
	for _, w := range words {
		b.WriteString(w + "\n\t" + w + " defined\n\n")
	}
	return b.String()
}

func searchHitsByName(msgs []streamMsg) map[string][]string {
	out := make(map[string][]string)
	for _, msg := range msgs {
		if msg.T != "hit" {
			continue
		}
		for _, result := range msg.Results {
			out[msg.Name] = append(out[msg.Name], result.Headword)
		}
	}
	return out
}

func TestLemmaOnlyInExplicitModes(t *testing.T) {
	s := morphServer(t, map[string]string{"words.dsl": dslFile("Unlabelled", "know")})
	getDicts(t, s, "/api/dicts")
	for _, mode := range []string{"exact", "word", "prefix", "suffix", "contains", "fts"} {
		msgs := stream(t, s, "/api/search?q=knew&mode="+mode+"&dict=all")
		if len(morphMsgs(msgs)) != 0 || len(searchHitsByName(msgs)) != 0 {
			t.Fatalf("%s searched a lemma without an explicit lemma mode: %+v", mode, msgs)
		}
	}
	for _, mode := range []string{"wordLemma", "exactLemma"} {
		msgs := stream(t, s, "/api/search?q=knew&mode="+mode+"&dict=all")
		if got := searchHitsByName(msgs)["Unlabelled"]; len(got) != 1 || got[0] != "know" {
			t.Fatalf("%s did not find the lemma: %+v", mode, msgs)
		}
		if len(morphMsgs(msgs)) != 0 {
			t.Fatalf("%s emitted a second-wave event: %+v", mode, msgs)
		}
	}
}

func TestExplicitLemmaModesIncludeBothForms(t *testing.T) {
	s := morphServer(t, map[string]string{"en-en-a.dsl": dslFile("English", "knew", "know")})
	getDicts(t, s, "/api/dicts")
	for _, mode := range []string{"wordLemma", "exactLemma"} {
		msgs := stream(t, s, "/api/search?q=knew&mode="+mode+"&dict=all")
		got := searchHitsByName(msgs)["English"]
		if len(got) != 2 || !(got[0] == "knew" && got[1] == "know" || got[0] == "know" && got[1] == "knew") {
			t.Fatalf("%s should return typed word and lemma together, got %v", mode, got)
		}
	}
}

func TestExplicitLemmaUsesDictionaryLanguage(t *testing.T) {
	s := morphServer(t, map[string]string{
		"en-en-a.dsl": dslFile("English", "know"),
		"ru-ru-b.dsl": dslFile("Russian", "идти"),
	})
	getDicts(t, s, "/api/dicts")
	for _, tc := range []struct{ query, want, other string }{
		{"knew", "English", "Russian"},
		{"идет", "Russian", "English"},
	} {
		msgs := stream(t, s, "/api/search?q="+tc.query+"&mode=exactLemma&dict=all")
		hits := searchHitsByName(msgs)
		if len(hits[tc.want]) != 1 || len(hits[tc.other]) != 0 {
			t.Fatalf("%q searched the wrong language: %+v", tc.query, hits)
		}
	}
}

func TestExplicitLemmaDisabled(t *testing.T) {
	s := morphServer(t, map[string]string{"en-en-a.dsl": dslFile("English", "know")})
	s.Morph = morph.New(0, "")
	getDicts(t, s, "/api/dicts")
	msgs := stream(t, s, "/api/search?q=knew&mode=exactLemma&dict=all")
	if len(searchHitsByName(msgs)) != 0 {
		t.Fatalf("disabled lemma pack must not add a form: %+v", msgs)
	}
}
