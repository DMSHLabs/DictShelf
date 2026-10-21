// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

func TestBrowseFindHeadwordsAcrossPages(t *testing.T) {
	s := browseStore(t, []string{"apple", "blue house", "house", "house boat", "house boat", "warehouse", "white house", "@_hidden"})
	words, total, err := s.FindHeadwords(context.Background(), "hou", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || fmt.Sprint(words) != "[blue house house]" {
		t.Fatalf("first result page = %v of %d", words, total)
	}
	words, total, err = s.FindHeadwords(context.Background(), "hou", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || fmt.Sprint(words) != "[house boat white house]" {
		t.Fatalf("second result page = %v of %d", words, total)
	}
	words, total, err = s.FindHeadwords(context.Background(), "ware", 0, 2)
	if err != nil || total != 1 || fmt.Sprint(words) != "[warehouse]" {
		t.Fatalf("whole-title prefix = %v of %d, %v", words, total, err)
	}
}

func TestBrowseFindHeadwordsMask(t *testing.T) {
	s := browseStore(t, []string{"corazón", "corazonada", "casa", "@_hidden"})
	words, total, _, err := s.FindHeadwordsMask(context.Background(), "coraz?n", 0, 10)
	if err != nil || total != 1 || len(words) != 1 || words[0] != "corazón" {
		t.Fatalf("mask find = %v, %d, %v", words, total, err)
	}
	words, total, _, err = s.FindHeadwordsMask(context.Background(), "*", 0, 10)
	if err != nil || total != 3 || len(words) != 3 {
		t.Fatalf("broad mask should return visible words only: %v, %d, %v", words, total, err)
	}
	for _, word := range words {
		if strings.HasPrefix(word, "@") {
			t.Fatalf("internal headword leaked: %q", word)
		}
	}
}

// browseStore ingests a headword-only fixture: browsing reads the index and
// nothing else, so no fixture here needs an article, a trigram table or FTS.
func browseStore(t *testing.T, words []string) *Store {
	t.Helper()
	entries := make([]dict.Entry, 0, len(words))
	for _, w := range words {
		entries = append(entries, h(w, "<p>x</p>"))
	}
	r := &fakeReader{
		meta:    dict.Meta{Name: "Browse Fixture", Format: "mdx", Path: "/nonexistent/browse.mdx"},
		entries: entries,
	}
	dbPath := filepath.Join(t.TempDir(), "browse.text.db")
	if _, err := IngestPlan(r, dbPath, Plan{}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBrowseLocateUsesAnchorRange(t *testing.T) {
	words := make([]string, 700)
	for i := range words {
		words[i] = fmt.Sprintf("word%04d", i)
	}
	s := browseStore(t, words)
	for _, target := range []int{100, 101, 299, 300, 653} {
		got, stats, err := s.LocateWithStats(words[target])
		if err != nil || got != target || !stats.AnchorFound || stats.CountedRows >= 100 {
			t.Errorf("LocateWithStats(%q) = %d, %+v, %v; want ordinal %d within one anchor stride", words[target], got, stats, err, target)
		}
	}
	var plan string
	err := s.db.QueryRow(`EXPLAIN QUERY PLAN SELECT COUNT(*) FROM entry WHERE `+browseWhere+
		` AND w >= ? COLLATE NOCASE`+
		` AND (w > ? COLLATE NOCASE OR (w = ? COLLATE NOCASE AND id > ?))`+
		` AND w < ? COLLATE NOCASE`, "word0600", "word0600", "word0600", 1, "word0653").Scan(new(int), new(int), new(int), &plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "idx_entry_w") || !strings.Contains(plan, "w>?") {
		t.Errorf("Locate count does not seek from its anchor: %s", plan)
	}
}

// TestBrowseAlphabet checks one page's new initials, including folded accents
// and the existing compact labels for non-Latin scripts.
func TestBrowseAlphabet(t *testing.T) {
	s := browseStore(t, []string{
		"1000", "apple", "azalea", "Zebra", // ASCII, case-folded
		"Ärger", "Ça va", "äpfel", // one A chip, split around C
		"—dash",    // not a letter
		"ア",        // katakana
		"中国", "漢字", // han, one chip
		"각", "나라", // hangul, two leads
	})
	got, err := s.Alphabet()
	if err != nil {
		t.Fatal(err)
	}
	want := []dict.Letter{{Letter: "1", Offset: 0, Count: 13}}
	if len(got) != len(want) {
		t.Fatalf("alphabet = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chip %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// The server totals the dictionary by summing the strip, so the strip has
	// to account for every row exactly once.
	sum := 0
	for _, l := range got {
		sum += l.Count
	}
	if sum != 13 {
		t.Errorf("chip counts sum to %d, want 13", sum)
	}
	// Every page marker addresses the same sequence Page reads.
	for _, l := range got {
		w, err := s.Page(l.Offset, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(w) != 1 {
			t.Fatalf("chip %q offset %d addresses nothing", l.Letter, l.Offset)
		}
		if at, err := s.Locate(w[0]); err != nil || at != l.Offset {
			t.Errorf("Locate(%q) = %d, %v; want %d", w[0], at, err, l.Offset)
		}
	}
}

// TestBrowsePageInitials assigns an initial to the page where its run begins,
// even when that transition occurs near the end of a page.
func TestBrowsePageInitials(t *testing.T) {
	words := make([]string, 0, 609)
	for i := 0; i < 299; i++ {
		words = append(words, fmt.Sprintf("a%03d", i))
	}
	for i := 0; i < 300; i++ {
		words = append(words, fmt.Sprintf("b%03d", i))
	}
	for i := 0; i < 10; i++ {
		words = append(words, fmt.Sprintf("c%03d", i))
	}
	s := browseStore(t, words)
	got, err := s.Alphabet()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("alphabet has %d page labels, want 3", len(got))
	}
	for i, l := range got {
		wantOffset := i * dict.MaskPageSize
		wantCount := min(dict.MaskPageSize, len(words)-wantOffset)
		if l.Offset != wantOffset || l.Count != wantCount {
			t.Errorf("page label %d = %+v, want offset %d count %d", i, l, wantOffset, wantCount)
		}
	}
	if got[0].Letter != "A-B" || got[1].Letter != "B001-C" || got[2].Letter != "C001" {
		t.Errorf("page labels = %q, %q, %q; want A-B, B001-C, C001", got[0].Letter, got[1].Letter, got[2].Letter)
	}
}
