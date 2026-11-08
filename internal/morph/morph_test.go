// Copyright (C) 2026 glowinthedark
// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package morph

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// installed writes lemma files into a temp LEMMA_DIR and returns it. Languages
// other than the built-in English and Russian packs use the same file path a
// user does, so these fixtures exercise the real loading path.
func installed(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The Russian fixture is keyed on the ё spellings, exactly as golem's own ru
// data is, so the е→ё retry is still what makes "идет" resolve.
var ruFixture = map[string]string{
	"ru.txt": "идти\tидёт\tидёшь\nсказать\tсказала\tсказал\nмёд\tмёда\n",
	"es.txt": "ser\tfuiste\tfui\nhaber\thubiera\thubo\n",
}

func TestLemma(t *testing.T) {
	c := New(6, installed(t, ruFixture))
	for _, tc := range []struct {
		code, in, want string
		ok             bool
	}{
		{"en", "knew", "know", true},
		{"en", "running", "run", true},
		{"en", "Houses", "house", true}, // case is the caller's to get wrong
		{"en", "house", "", false},      // already a lemma
		{"en", "qwertyuiop", "", false}, // unknown
		{"es", "fuiste", "ser", true},   // installed, not built in
		{"es", "hubiera", "haber", true},
		{"ru", "сказала", "сказать", true},
		{"ru", "идет", "идти", true}, // yo fallback: the data knows "идёт"
		{"ru", "мед", "", false},     // "мёд" is itself a lemma: nothing to offer
		{"de", "Hauser", "", false},  // nothing installed for German
		{"xx", "whatever", "", false},
		{"en", "", "", false},
	} {
		got, ok := c.Lemma(tc.code, tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("Lemma(%q,%q) = %q,%v; want %q,%v", tc.code, tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDisabled(t *testing.T) {
	c := New(0, "")
	if c.Enabled() {
		t.Fatal("New(0) must be disabled")
	}
	if _, ok := c.Lemma("en", "knew"); ok {
		t.Error("disabled cache lemmatized")
	}
	if len(c.kept) != 0 {
		t.Error("disabled cache loaded a pack")
	}
	if c.Supports("en") {
		t.Error("MORPH_CACHE=0 must not report English either")
	}
}

func TestEviction(t *testing.T) {
	c := New(1, installed(t, ruFixture))
	if _, ok := c.Lemma("en", "knew"); !ok {
		t.Fatal("en")
	}
	if _, ok := c.Lemma("es", "fuiste"); !ok {
		t.Fatal("es")
	}
	c.mu.Lock()
	n, has := len(c.kept), c.kept["es"] != nil
	c.mu.Unlock()
	if n != 1 || !has {
		t.Fatalf("kept %d entries, es present=%v; want 1 and es", n, has)
	}
	// Evicted, not broken: en reloads and answers the same.
	if got, ok := c.Lemma("en", "knew"); !ok || got != "know" {
		t.Errorf("after eviction Lemma(en,knew) = %q,%v", got, ok)
	}
}

func TestConcurrent(t *testing.T) {
	c := New(2, installed(t, ruFixture))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if got, ok := c.Lemma("en", "knew"); !ok || got != "know" {
					t.Errorf("en: %q,%v", got, ok)
					return
				}
				if got, ok := c.Lemma("es", "fuiste"); !ok || got != "ser" {
					t.Errorf("es: %q,%v", got, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestBuiltInEnglishAndRussian ensures an empty LEMMA_DIR has both built-in
// packs available, while other languages remain opt-in files.
func TestBuiltInEnglishAndRussian(t *testing.T) {
	if len(packs) != 2 || packs["en"] == nil || packs["ru"] == nil {
		t.Fatalf("built-in packs = %v; want en and ru", keys(packs))
	}
	c := New(2, "")
	for _, code := range []string{"en", "ru"} {
		if !c.Supports(code) {
			t.Errorf("%s must work with nothing installed", code)
		}
	}
	if got, ok := c.Lemma("ru", "сказала"); !ok || got != "сказать" {
		t.Errorf("built-in Russian: %q, %v; want сказать, true", got, ok)
	}
	for _, code := range []string{"de", "es", "fr", "it", "pl"} {
		if c.Supports(code) {
			t.Errorf("%s must not be built in", code)
		}
		if _, ok := c.Lemma(code, "whatever"); ok {
			t.Errorf("%s lemmatized with an empty LEMMA_DIR", code)
		}
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
