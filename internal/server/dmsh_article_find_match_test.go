// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "testing"

func TestArticleFindHTMLMatchesAcrossInlineMarkup(t *testing.T) {
	got := matchArticleFindHTML(`<p>Jazz <b>clarinetist</b>, jazz clarinetist.</p>`, articleFindMatchOptions{
		query: "jazz clarinetist", mode: "phrase",
	})
	if len(got) != 2 {
		t.Fatalf("matches = %+v, want two", got)
	}
	if got[0] != (articleFindOccurrence{Group: 0, Start: 0, End: 16}) ||
		got[1] != (articleFindOccurrence{Group: 0, Start: 18, End: 34}) {
		t.Fatalf("matches = %+v, want phrase spans in one paragraph", got)
	}
}

func TestArticleFindPhraseMatchesAcrossHighlightFragments(t *testing.T) {
	body := `<p><mark class="wu-hl">джазовый</mark> <a><mark class="wu-hl">кларнетист</mark></a>; ` +
		`<mark class="wu-hl">джазовый</mark> один <mark class="wu-hl">кларнетист</mark></p>`
	got := matchArticleFindHTML(body, articleFindMatchOptions{query: "джазовый кларнетист", mode: "phrase"})
	if len(got) != 1 {
		t.Fatalf("phrase matches = %+v, want only the complete phrase across highlight fragments", got)
	}
}

func TestArticleFindHTMLMatchesModesAndUTF16Offsets(t *testing.T) {
	body := `<p>🍎 Catapult cat catfish.</p>`
	tests := []struct {
		name, mode, query string
		want              []articleFindOccurrence
	}{
		{"contains", "contains", "cat", []articleFindOccurrence{{Group: 0, Start: 3, End: 6}, {Group: 0, Start: 12, End: 15}, {Group: 0, Start: 16, End: 19}}},
		{"prefix", "prefix", "cat", []articleFindOccurrence{{Group: 0, Start: 3, End: 6}, {Group: 0, Start: 12, End: 15}, {Group: 0, Start: 16, End: 19}}},
		{"suffix", "suffix", "cat", []articleFindOccurrence{{Group: 0, Start: 12, End: 15}}},
		{"word", "word", "cat", []articleFindOccurrence{{Group: 0, Start: 12, End: 15}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := matchArticleFindHTML(body, articleFindMatchOptions{query: test.query, mode: test.mode})
			if len(got) != len(test.want) {
				t.Fatalf("matches = %+v, want %+v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("matches = %+v, want %+v", got, test.want)
				}
			}
		})
	}
}

func TestArticleFindHTMLNearRepeatedWordsAndExamples(t *testing.T) {
	body := `<p>red fox red quick fox</p><p class="wu-ex">red fox</p><script>red fox</script>`
	got := matchArticleFindHTML(body, articleFindMatchOptions{query: "red fox", mode: "near", nearWords: 2})
	if len(got) != 2 || got[0].Group != 0 || got[1].Group != 0 {
		t.Fatalf("near matches = %+v, want two visible article matches", got)
	}
	withExamples := matchArticleFindHTML(body, articleFindMatchOptions{query: "red fox", mode: "phrase", examples: true})
	if len(withExamples) != 2 {
		t.Fatalf("example-inclusive matches = %+v, want visible phrase and example", withExamples)
	}
}
