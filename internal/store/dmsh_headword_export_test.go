// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestExportHeadwordsUnicodeAndCancellation(t *testing.T) {
	s := browseStore(t, []string{"apple", "apple", "&#945;", "слово", "@_hidden"})
	var out bytes.Buffer
	var count int64
	if err := s.ExportHeadwords(context.Background(), &out, func(n int64) { count = n }); err != nil {
		t.Fatal(err)
	}
	if out.String() != "α\napple\napple\nслово\n" || count != 4 {
		t.Fatalf("export = %q, count %d", out.String(), count)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out.Reset()
	if err := s.ExportHeadwords(ctx, &out, func(int64) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("canceled export wrote data")
	}
}
