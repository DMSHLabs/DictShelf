// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package glass

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestPostingCancellation(t *testing.T) {
	db, err := Open(postlistFixture("term", []uint32{7, 1060, 34193}, []uint32{2, 5, 1}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := 0
	db.Progress = func(uint32) { seen++; cancel() }
	_, err = db.lookupContext(ctx, "term")
	if !errors.Is(err, context.Canceled) || seen != 1 {
		t.Fatalf("err=%v visited=%d", err, seen)
	}
	db.Progress = nil
	got, err := db.Lookup("term")
	if err != nil || len(got.Postings) != 3 {
		t.Fatalf("next lookup: %+v %v", got, err)
	}
}
func TestSortHitsCancellationAndOrder(t *testing.T) {
	hits := make([]Hit, 10000)
	for i := range hits {
		hits[i] = Hit{DocID: uint32(i), WDF: uint32((i * 7919) % 113)}
	}
	want := append([]Hit(nil), hits...)
	sort.SliceStable(want, func(i, j int) bool { return want[i].WDF < want[j].WDF })
	got := append([]Hit(nil), hits...)
	if err := sortHits(context.Background(), got, func(i, j int) bool { return got[i].WDF < got[j].WDF }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("stable ordering differs")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	compares := 0
	err := sortHits(ctx, hits, func(i, j int) bool {
		compares++
		if compares == 100 {
			cancel()
		}
		return hits[i].WDF < hits[j].WDF
	})
	if !errors.Is(err, context.Canceled) || compares > 1200 {
		t.Fatalf("err=%v comparisons=%d", err, compares)
	}
}
