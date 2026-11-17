// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"reflect"
	"testing"
)

// These sequences match Xapian's TermGenerator n-gram examples in
// xapian-core/tests/api_termgen.cc, after libzim's removeAccents preprocessing.
// The Japanese dakuten is intentionally removed by libzim before indexing.
func TestFoldTextMatchesXapianNgramExamples(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "Han",
			text: "久有归天",
			want: []string{"久", "久有", "有", "有归", "归", "归天", "天"},
		},
		{
			name: "Hangul",
			text: "극지라",
			want: []string{"극", "극지", "지", "지라", "라"},
		},
		{
			name: "Japanese",
			text: "ウルス アップ",
			want: []string{"ウ", "ウル", "ル", "ルス", "ス", "ア", "アッ", "ッ", "ッフ", "フ"},
		},
		{
			name: "mixed scripts",
			text: "インtestタ",
			want: []string{"イ", "イン", "ン", "test", "タ"},
		},
		{
			name: "CJK punctuation is a separator",
			text: "申込み！月額円",
			want: []string{"申", "申込", "込", "込み", "み", "月", "月額", "額", "額円", "円"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FoldText(tc.text); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FoldText(%q) = %#v, want %#v", tc.text, got, tc.want)
			}
		})
	}
}

func TestFoldTermsDeduplicatesCJKNgrams(t *testing.T) {
	want := []string{"中", "中华", "华", "华中", "中人", "人"}
	if got := FoldTerms("中华中人"); !reflect.DeepEqual(got, want) {
		t.Errorf("FoldTerms = %#v, want %#v", got, want)
	}
}
