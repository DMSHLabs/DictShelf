// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

func TestWildcardIndexedWordBatchesKeepRowIDs(t *testing.T) {
	s := &Server{User: UserDir(t.TempDir())}
	dir := s.wildcardCacheDir()
	s.wildcardPages.init(dir)
	id := "batchrowids123456"
	signature := wildcardCacheSignature{query: "*", group: "all", wildcards: true, fts: true, trigram: true}
	for order, dictID := range []string{"first", "second"} {
		entry, token, err := s.wildcardPages.begin(id, signature, browsePageSize, 2, dictID, dictID, order, "", false, dir)
		if err != nil {
			t.Fatal(err)
		}
		defer entry.db.Close()
		words := make([]string, 330)
		for i := range words {
			words[i] = fmt.Sprintf("%s café %03d", dictID, i)
		}
		for _, end := range []int{151, len(words)} {
			from := 0
			if end == len(words) {
				from = 151
			}
			if err := entry.append(dictID, "", false, token, browseFindStreamEvent{
				Words: words[from:end], Total: end, Done: end == len(words),
			}); err != nil {
				t.Fatal(err)
			}
		}
		key := wildcardStreamKey{dictID: dictID}
		s.wildcardPages.finish(entry, &key, token)
		for _, position := range []int{0, 127, 128, 150, 151, 255, 256, 299, 300, 329} {
			var ftsWord, trigramWord string
			err := entry.db.QueryRow(`SELECT f.word,t.word FROM wildcard_word w
				JOIN wildcard_word_fts f ON f.rowid=w.id
				JOIN wildcard_word_trigram t ON t.rowid=w.id
				WHERE w.dict_id=? AND w.base_position=?`, dictID, position).Scan(&ftsWord, &trigramWord)
			if err != nil || ftsWord != words[position] || trigramWord != dict.Fold(words[position]) {
				t.Fatalf("%s position %d: FTS=%q trigram=%q error=%v", dictID, position, ftsWord, trigramWord, err)
			}
		}
	}
}

func TestWildcardDictionaryCacheIndexesFilterTheSameWords(t *testing.T) {
	words := []string{"dog", "dogma", "dig", "a dog", "hmm", "café"}
	for _, flags := range []struct {
		name string
		fts  bool
		tri  bool
	}{
		{"plain", false, false},
		{"fts", true, false},
		{"trigram", false, true},
		{"both", true, true},
	} {
		t.Run(flags.name, func(t *testing.T) {
			s := &Server{User: UserDir(t.TempDir())}
			dir := s.wildcardCacheDir()
			s.wildcardPages.init(dir)
			id := fmt.Sprintf("%016d", len(flags.name))
			signature := wildcardCacheSignature{query: "*", group: "all", wildcards: true, fts: flags.fts, trigram: flags.tri}
			base, token, err := s.wildcardPages.begin(id, signature, browsePageSize, 2, "dict", "Dictionary", 0, "", false, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer base.db.Close()
			if err := base.append("dict", "", false, token, browseFindStreamEvent{Words: words, Total: len(words), Done: true}); err != nil {
				t.Fatal(err)
			}
			key := wildcardStreamKey{dictID: "dict"}
			s.wildcardPages.finish(base, &key, token)
			other, otherToken, err := s.wildcardPages.begin(id, signature, browsePageSize, 2, "other", "Other", 1, "", false, dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := other.append("other", "", false, otherToken, browseFindStreamEvent{Words: []string{"dog"}, Total: 1, Done: true}); err != nil {
				t.Fatal(err)
			}
			otherKey := wildcardStreamKey{dictID: "other"}
			s.wildcardPages.finish(other, &otherKey, otherToken)
			for _, query := range []struct {
				text    string
				mask    bool
				want    []string
				indexed bool
			}{
				{"dog", false, []string{"dog", "dogma", "a dog"}, flags.fts},
				{"cafe", false, []string{"café"}, flags.fts},
				{"*dog*", true, []string{"dog", "dogma", "a dog"}, flags.tri},
				{"*caf*", true, []string{"café"}, flags.tri},
				{"*h*m*", true, []string{"hmm"}, false},
				{"*d*g*", true, []string{"dog", "dogma", "dig", "a dog"}, false},
			} {
				request := browseFindStreamRequest{entry: &entry{ID: "dict"}, query: "*", mask: true,
					within: query.text, withinMask: query.mask, cacheID: id, cacheGroup: "all", cacheCount: 2,
					cacheFTS: flags.fts, cacheTrigram: flags.tri}
				var got []string
				var last browseFindStreamEvent
				err := s.streamCachedWildcardWithin(context.Background(), request, func(event browseFindStreamEvent) error {
					got = append(got, event.Words...)
					last = event
					return nil
				})
				if err != nil {
					t.Fatalf("%q: %v", query.text, err)
				}
				if !reflect.DeepEqual(got, query.want) {
					t.Errorf("%q: got %q, want %q", query.text, got, query.want)
				}
				if !last.Done || last.Total != len(query.want) {
					t.Errorf("%q: incomplete result: %+v", query.text, last)
				}
				if query.indexed && last.ScanTotal >= len(words) {
					t.Errorf("%q: index was not selected: scanned %d of %d", query.text, last.ScanTotal, len(words))
				}
			}
			base, token, err = s.wildcardPages.begin(id, signature, browsePageSize, 2, "dict", "Dictionary", 0, "", false, dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := base.append("dict", "", false, token, browseFindStreamEvent{Words: []string{"cat", "dog"}, Total: 2, Done: true}); err != nil {
				t.Fatal(err)
			}
			s.wildcardPages.finish(base, &key, token)
			for _, query := range []struct {
				text string
				mask bool
			}{{"dog", false}, {"*dog*", true}} {
				request := browseFindStreamRequest{entry: &entry{ID: "dict"}, query: "*", mask: true,
					within: query.text, withinMask: query.mask, cacheID: id, cacheGroup: "all", cacheCount: 2,
					cacheFTS: flags.fts, cacheTrigram: flags.tri}
				var got []string
				if err := s.streamCachedWildcardWithin(context.Background(), request, func(event browseFindStreamEvent) error {
					got = append(got, event.Words...)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, []string{"dog"}) {
					t.Errorf("%q after rebuilding base: got %q", query.text, got)
				}
			}
			if err := base.db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := &wildcardPageCache{}
			reopened.init(dir)
			page, ok, err := reopened.page(id, signature, "dict", "", false, 1, 0, 2)
			if err != nil || !ok || !reflect.DeepEqual(page.Words, []string{"cat", "dog"}) {
				t.Errorf("cached base after reopen: %+v, available=%t, error=%v", page, ok, err)
			}
			if saved := reopened.entries[id]; saved != nil {
				_ = saved.db.Close()
			}
		})
	}
}
