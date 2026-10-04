// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "testing"

func TestBulkRebuildRefreshesCurrentIndex(t *testing.T) {
	s, e := demandEntry(t)
	sse(t, s, "/api/ingest?dict="+e.ID+"&contains=1&fts=1")
	want := s.currentFeatures(e)
	if !want.Contains || !want.FullText {
		t.Fatal("fixture lacks optional indexes")
	}
	progressCalls := 0
	progress := func(int, int) { progressCalls++ }
	if err := e.setFeatures(want, progress); err != nil {
		t.Fatal(err)
	}
	if progressCalls != 0 {
		t.Fatal("ordinary request rebuilt a current index")
	}
	if err := e.setFeatures(want, progress, true); err != nil {
		t.Fatal(err)
	}
	if progressCalls == 0 {
		t.Fatal("forced rebuild skipped a current index")
	}
	if got := s.currentFeatures(e); got != want {
		t.Fatalf("rebuild changed optional features: got %+v, want %+v", got, want)
	}
}
