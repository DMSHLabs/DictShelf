// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"reflect"
	"testing"
)

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
	body := `<p><mark class="wu-hl" data-wu-match="0">джазовый</mark> <a><mark class="wu-hl-ex" data-wu-match="0">кларнетист</mark></a>; ` +
		`<mark class="wu-hl" data-wu-match="1">джазовый</mark> один <mark class="wu-hl-ex" data-wu-match="1">кларнетист</mark></p>`
	got := matchArticleFindHTML(body, articleFindMatchOptions{query: "джазовый кларнетист", mode: "phrase"})
	if len(got) != 1 {
		t.Fatalf("phrase matches = %+v, want only the complete phrase across highlight fragments", got)
	}
}

func TestArticleFindNearMatchesBothWordOrders(t *testing.T) {
	body := `<p>кларнетист x джаз; джаз at кларнетист</p>`
	got := matchArticleFindHTML(body, articleFindMatchOptions{query: "джаз кларнетист", mode: "near", nearWords: 2})
	if len(got) != 2 || got[0].Start != 0 || got[1].Start <= got[0].Start {
		t.Fatalf("near matches = %+v; want one complete window in each word order", got)
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

// The page receives PATHS, not block offsets: a child-index path to a text node
// and UTF-16 offsets inside that node - the units a DOM Range speaks - so the
// page resolves a match without rebuilding the server's text model. A phrase the
// markup split arrives as several spans of ONE occurrence, an astral character
// counts twice, and the text travels with it so the page can check what it found.
func TestArticleFindWireAddressesTextNodes(t *testing.T) {
	body := `<p>a 🍎 джазовый <b>кларнетист</b> играл</p>`
	groups, matches := articleFindHTMLSpans(body, articleFindMatchOptions{query: "джазовый кларнетист", mode: "phrase"})
	wire := articleFindWireOccurrences(groups, matches)
	if len(wire) != 1 {
		t.Fatalf("occurrences = %+v, want one phrase", wire)
	}
	occurrence := wire[0]
	if occurrence.Text != "джазовый кларнетист" {
		t.Fatalf("occurrence text = %q, want the matched words", occurrence.Text)
	}
	if len(occurrence.Spans) != 2 {
		t.Fatalf("spans = %+v, want one per text node the phrase lives in", occurrence.Spans)
	}
	first, last := occurrence.Spans[0], occurrence.Spans[1]
	if !reflect.DeepEqual(first.Path, []int{0, 0}) || !reflect.DeepEqual(last.Path, []int{0, 1, 0}) {
		t.Fatalf("paths = %v and %v, want the article's own child indices", first.Path, last.Path)
	}
	// "a 🍎 джазовый" - the emoji is two UTF-16 units, so the word starts at 5.
	// The span reaches 14 because the space that separates the two words lives in
	// THIS node: the page takes the first span's start and the last span's end, so
	// what is inside the boundary does not matter.
	if first.Start != 5 || first.End != 14 {
		t.Fatalf("first span = %+v, want UTF-16 5..14 inside its node", first)
	}
	if last.Start != 0 || last.End != 10 {
		t.Fatalf("last span = %+v, want UTF-16 0..10 inside its node", last)
	}
}
