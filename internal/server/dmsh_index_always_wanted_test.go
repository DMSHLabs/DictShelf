// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

// A dictionary's index is not a thing that can be removed on its own: it is what
// makes the dictionary usable, and building it again would cost the whole
// source. The whole dictionary may go, or its files only.
func TestTheIndexCannotBeRemovedOnItsOwn(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	textDB := store.TextDBPath(dir)
	e := s.reg.entryAt(textDB)
	if _, ok := store.LookupDir(e.Path); !ok {
		t.Fatal("the fixture is not prepared")
	}

	rec := deleteReq(t, s, "/api/library?dict="+e.ID+"&prepared=1&source=0")
	if rec.Code != 400 {
		t.Fatalf("removing the index alone = %d %s, want a refusal", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cannot be removed on its own") {
		t.Errorf("the refusal does not say what the rule is: %s", rec.Body.String())
	}
	if _, err := os.Stat(textDB); err != nil {
		t.Errorf("the refused request removed the index anyway: %v", err)
	}
	if e.indexBlocked() {
		t.Error("the refused request left the dictionary blocked")
	}
	if _, err := e.open(); err != nil {
		t.Errorf("the dictionary is no longer usable after the refusal: %v", err)
	}

	// The whole dictionary still goes, and so does its files-only mode.
	if rec := deleteReq(t, s, "/api/library?dict="+e.ID+"&prepared=1&source=1"); rec.Code != 200 {
		t.Fatalf("removing the whole dictionary = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(textDB); !os.IsNotExist(err) {
		t.Errorf("the index is still there after removing the dictionary: %v", err)
	}
}

// The headword index is mandatory, so "there is no index" is a state to LEAVE,
// never one to respect: a dictionary whose index is missing - never built, built
// and deleted out of band, or remembered as missing by an older library - is
// prepared by the next rescan, and a stale marker beside a complete index is
// simply cleared.
func TestAMissingOrRememberedIndexIsPrepared(t *testing.T) {
	t.Run("the folder is gone", func(t *testing.T) {
		s := newTestServer(t)
		dir := preparedDSL(t, s)
		textDB := store.TextDBPath(dir)
		e := s.reg.entryAt(textDB)
		markIndexPending(t, s, e)
		// Out of band, as a disk cleanup or an older build would leave it.
		e.closeNow()
		if _, err := store.RemovePrepared(dir); err != nil {
			t.Fatal(err)
		}

		if err := s.reg.Rescan(); err != nil {
			t.Fatal(err)
		}
		waitNewDictionaryIndexing(t, s.reg)

		if _, ok := store.LookupDir(e.Path); !ok {
			t.Fatal("a dictionary with no index was not prepared again")
		}
		if e.indexBlocked() {
			t.Error("the marker survived the build that gave the dictionary its index back")
		}
		if _, err := e.open(); err != nil {
			t.Errorf("the rebuilt dictionary does not open: %v", err)
		}
	})

	t.Run("the index is there and the marker is stale", func(t *testing.T) {
		s := newTestServer(t)
		dir := preparedDSL(t, s)
		textDB := store.TextDBPath(dir)
		e := s.reg.entryAt(textDB)
		before := store.KeptPlan(textDB)
		markIndexPending(t, s, e)

		if err := s.reg.Rescan(); err != nil {
			t.Fatal(err)
		}
		waitNewDictionaryIndexing(t, s.reg)

		if e.indexBlocked() {
			t.Error("a dictionary with a complete index stayed blocked by a stale marker")
		}
		if got := store.KeptPlan(textDB); got != before {
			t.Errorf("the stale marker changed the index: %+v, want %+v", got, before)
		}
		if _, err := e.open(); err != nil {
			t.Errorf("the dictionary does not open: %v", err)
		}
	})
}

// markIndexPending writes the "no index right now" marker the way an older
// library (or a build interrupted mid-preparation) would have left it.
func markIndexPending(t *testing.T, s *Server, e *entry) {
	t.Helper()
	if err := s.reg.prefs.mutate(func(f *prefsFile) {
		if f.DSLRemoved == nil {
			f.DSLRemoved = map[string]bool{}
		}
		f.DSLRemoved[e.indexRemovalKey()] = true
	}); err != nil {
		t.Fatal(err)
	}
	if !e.indexBlocked() {
		t.Fatal("the fixture did not take the marker")
	}
	// A moment for the registry to look at the state it was just handed.
	time.Sleep(20 * time.Millisecond)
}
