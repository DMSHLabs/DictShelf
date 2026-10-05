// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "testing"

func TestUserCountCollapsesParserVariants(t *testing.T) {
	r := &Registry{entries: []*entry{
		{Path: "first.dsl", dslSource: "first.dsl"},
		{Path: "first.dslgd", dslSource: "first.dsl"},
		{Path: "cached/text.db", dslSource: "second.dsl"},
		{Path: "other/first.dsl", dslSource: "other/first.dsl"},
		{Path: "standalone.db"},
		{Path: "guide.wmd", builtin: true},
	}}
	if got := r.UserCount(); got != 4 {
		t.Fatalf("UserCount = %d, want 4 source dictionaries", got)
	}
	if got := r.Count(); got != 6 {
		t.Fatalf("Count = %d, want all 6 backend entries", got)
	}
}
