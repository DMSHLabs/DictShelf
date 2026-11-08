// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/hilite"
)

// phraseMarking is the rung's shape for a phrase reading: one Phrase of the
// query's words, a trailing star meaning the token was left open - exactly
// what ftsq hands over.
func phraseMarking(words ...string) articleFindMarking {
	p := make(hilite.Phrase, len(words))
	for i, w := range words {
		p[i] = hilite.Term{Text: strings.TrimSuffix(w, "*"), Prefix: strings.HasSuffix(w, "*")}
	}
	return articleFindMarking{phrases: []hilite.Phrase{p}}
}

// nearMarking is the near rung's shape: the bag's one-token prefix phrases.
func nearMarking(bound int, words ...string) articleFindMarking {
	m := articleFindMarking{near: true, nearWords: bound}
	for _, w := range words {
		m.phrases = append(m.phrases, hilite.Phrase{{Text: w, Prefix: true}})
	}
	return m
}

func mark(t *testing.T, body string, m articleFindMarking) string {
	t.Helper()
	return markArticleFindHTML(body, m)
}

// The case this whole pass exists for: a dictionary wrapped the query's words
// in links, so the phrase lives in two text nodes. One navigation stop, on the
// word the reader meets first; the rest of the phrase painted alike but not a
// stop.
func TestPhraseMarksAcrossMarkupAsOneOccurrence(t *testing.T) {
	in := "<a href=\"xxxx\">джазовый</a>\n<span><a href=\"yyyy\">кларнетист</a></span>"
	want := "<a href=\"xxxx\"><mark class=\"wu-hl\" data-wu-match=\"0\">джазовый</mark></a>\n" +
		"<span><a href=\"yyyy\"><mark class=\"wu-hl-ex\" data-wu-match=\"0\">кларнетист</mark></a></span>"
	if got := mark(t, in, phraseMarking("джазовый", "кларнетист")); got != want {
		t.Errorf("phrase across markup:\n got %q\nwant %q", got, want)
	}
}

// Words that stand together without markup are one span, exactly as the phrase
// reading's label says.
func TestPhraseInOneTextNodeIsOneMark(t *testing.T) {
	in := `<p>Словарь: джазовый кларнетист играет</p>`
	want := `<p>Словарь: <mark class="wu-hl" data-wu-match="0">джазовый кларнетист</mark> играет</p>`
	if got := mark(t, in, phraseMarking("джазовый", "кларнетист")); got != want {
		t.Errorf("contiguous phrase:\n got %q\nwant %q", got, want)
	}
}

// A token that markup split in the middle is still one query word: both halves
// are the head, because they are the first fragment.
func TestPhraseFragmentSpanningMarkup(t *testing.T) {
	in := `<b>джазо</b>вый кларнетист`
	want := `<b><mark class="wu-hl" data-wu-match="0">джазо</mark></b>` +
		`<mark class="wu-hl" data-wu-match="0">вый</mark> ` +
		`<mark class="wu-hl-ex" data-wu-match="0">кларнетист</mark>`
	if got := mark(t, in, phraseMarking("джазовый", "кларнетист")); got != want {
		t.Errorf("fragment across markup:\n got %q\nwant %q", got, want)
	}
}

func TestPhraseLeavesOtherTextAlone(t *testing.T) {
	for _, in := range []string{
		`<p>джазовый оркестр и кларнетист</p>`,           // words apart
		`<p>джазовый</p><p>кларнетист</p>`,               // a block boundary is not adjacency
		`<p>кларнетист джазовый</p>`,                     // the phrase, reversed
		`<p>джазовый <b>и</b> кларнетист</p>`,            // an intervening token breaks it
		`<script>джазовый кларнетист</script><p>нет</p>`, // script text is never read
	} {
		if got := mark(t, in, phraseMarking("джазовый", "кларнетист")); got != in {
			t.Errorf("mark(%q)\n got %q\nwant unchanged", in, got)
		}
	}
}

// The near reading: one stop per window, the first term the reader meets is
// the stop - in either order - and the words between the terms are painted
// paler without being matches.
func TestNearWindowMarksTheWordsBetween(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"in order", `<p>джазовый и кларнетист играл</p>`,
			`<p><mark class="wu-hl" data-wu-match="0">джазовый</mark><span class="wu-hl-ex2" data-wu-match="0"> и </span><mark class="wu-hl-ex" data-wu-match="0">кларнетист</mark> играл</p>`},
		{"reversed: the first word read is the stop", `<p>кларнетист и джазовый</p>`,
			`<p><mark class="wu-hl" data-wu-match="0">кларнетист</mark><span class="wu-hl-ex2" data-wu-match="0"> и </span><mark class="wu-hl-ex" data-wu-match="0">джазовый</mark></p>`},
		{"punctuation between the terms is not a bridge", `<p>кларнетист, джазовый</p>`,
			`<p><mark class="wu-hl" data-wu-match="0">кларнетист</mark>, <mark class="wu-hl-ex" data-wu-match="0">джазовый</mark></p>`},
		{"nothing between: no bridge", `<p>джазовый кларнетист</p>`,
			`<p><mark class="wu-hl" data-wu-match="0">джазовый</mark> <mark class="wu-hl-ex" data-wu-match="0">кларнетист</mark></p>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mark(t, tc.in, nearMarking(10, "джазовый", "кларнетист")); got != tc.want {
				t.Errorf("near:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestNearWindowBoundIsRespected(t *testing.T) {
	in := `<p>джазовый надо ещё пять слов кларнетист</p>`
	if got := mark(t, in, nearMarking(2, "джазовый", "кларнетист")); got != in {
		t.Errorf("a window past the bound was marked: %q", got)
	}
	if got := mark(t, in, nearMarking(10, "джазовый", "кларнетист")); got == in {
		t.Error("a window inside the bound was not marked")
	}
}

// Occurrences are numbered in document order, one number per occurrence, and
// every fragment of an occurrence carries it.
func TestOccurrencesAreNumberedInDocumentOrder(t *testing.T) {
	in := `<p>джазовый кларнетист</p><p>потом джазовый кларнетист</p>`
	want := `<p><mark class="wu-hl" data-wu-match="0">джазовый кларнетист</mark></p>` +
		`<p>потом <mark class="wu-hl" data-wu-match="1">джазовый кларнетист</mark></p>`
	if got := mark(t, in, phraseMarking("джазовый", "кларнетист")); got != want {
		t.Errorf("numbering:\n got %q\nwant %q", got, want)
	}
}

// The cap stops marking, never splits an occurrence, and never loses text.
func TestMarkMaxMarksNeverSplitsAnOccurrence(t *testing.T) {
	body := strings.Repeat(`<p>джазовый кларнетист</p>`, hilite.MaxMarks+20)
	got := mark(t, body, phraseMarking("джазовый", "кларнетист"))
	if n := strings.Count(got, `class="wu-hl"`); n != hilite.MaxMarks {
		t.Errorf("occurrences = %d, want %d", n, hilite.MaxMarks)
	}
	if n := strings.Count(got, `class="wu-hl-ex"`); n != 0 {
		t.Errorf("continuations without a head: %d", n)
	}
	if n := strings.Count(got, `</mark>`); n != hilite.MaxMarks {
		t.Errorf("marks = %d, want %d", n, hilite.MaxMarks)
	}
}

// Marking wraps text; it never changes it.
func TestMarkPreservesTheArticleText(t *testing.T) {
	in := `<p>Der Fuchs läuft; café &amp; co. &#1053;&#1077; <b>джазо</b>вый кларнетист 42</p>`
	got := mark(t, in, phraseMarking("джазовый", "кларнетист"))
	groups := articleFindHTMLGroups(in, true)
	wantGroups := articleFindHTMLGroups(got, true)
	if len(groups) != len(wantGroups) {
		t.Fatalf("text groups = %d, want %d", len(wantGroups), len(groups))
	}
	for i := range groups {
		if string(groups[i].text) != string(wantGroups[i].text) {
			t.Errorf("group %d text changed:\n got %q\nwant %q", i, string(wantGroups[i].text), string(groups[i].text))
		}
	}
}

func TestMarkPreservesEntities(t *testing.T) {
	in := `<p>джазовый&nbsp;кларнетист</p>`
	// The renderer writes the character, not the entity: same text, and the
	// page renders identically.
	want := "<p><mark class=\"wu-hl\" data-wu-match=\"0\">джазовый кларнетист</mark></p>"
	if got := mark(t, in, phraseMarking("джазовый", "кларнетист")); got != want {
		t.Errorf("entity:\n got %q\nwant %q", got, want)
	}
}

// A phrase whose last token was left open marks the longer token, as the FTS5
// expression it stands for does.
func TestPhrasePrefixOnTheLastToken(t *testing.T) {
	in := `<p>джазовый кларнетиста играл</p>`
	want := `<p><mark class="wu-hl" data-wu-match="0">джазовый кларнетиста</mark> играл</p>`
	if got := mark(t, in, phraseMarking("джазовый", "кларнетист*")); got != want {
		t.Errorf("prefix phrase:\n got %q\nwant %q", got, want)
	}
}

func TestMarkReturnsTheBodyWhenNothingMatches(t *testing.T) {
	in := `<p   class=prose>текст</p>`
	if got := mark(t, in, phraseMarking("джазовый")); got != in {
		t.Errorf("unmatched body was rewritten: %q", got)
	}
	if got := markArticleFindHTML(in, articleFindMarking{}); got != in {
		t.Errorf("empty marking rewrote the body: %q", got)
	}
}

// The navigation total is the number of heads, read from the same markup the
// reader sees - never a substring of a class attribute.
func TestCountArticleFindHeads(t *testing.T) {
	body := `<p><mark class="wu-hl" data-wu-match="0">a</mark>` +
		`<span class="wu-hl-ex2" data-wu-match="0">x</span>` +
		`<mark class="wu-hl-ex" data-wu-match="0">b</mark></p>` +
		`<p><mark class="wu-hl" data-wu-match="1">c</mark></p>`
	if n := countArticleFindHeads(body); n != 2 {
		t.Errorf("heads = %d, want 2", n)
	}
	if n := countArticleFindHeads(`<mark class="wu-hl wu-extra" data-wu-match="0">a</mark>`); n != 1 {
		t.Errorf("a multi-name class list = %d, want 1", n)
	}
	if n := countArticleFindHeads(`<mark class="wu-hl-lemma">a</mark>`); n != 0 {
		t.Errorf("a lemma mark counted as a match: %d", n)
	}
}

// What one phrase-marked article costs: a parse, the match, the paint and a
// re-serialization, where the words reading only rides the rewriter's hook.
// The numbers live in docs/MEASUREMENTS.md; this is the command that produces
// them (a micro-benchmark on a synthetic body, not a real dictionary).
func BenchmarkMarkArticleFindHTML(b *testing.B) {
	body := strings.Repeat(`<p>джазовый оркестр играет в зале, а <a href="x">кларнетист</a> молчит.</p>`, 120)
	m := phraseMarking("джазовый", "кларнетист")
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = markArticleFindHTML(body, m)
	}
}

func BenchmarkCountArticleFindHeads(b *testing.B) {
	body := markArticleFindHTML(strings.Repeat(`<p>джазовый кларнетист</p>`, 200), phraseMarking("джазовый", "кларнетист"))
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = countArticleFindHeads(body)
	}
}
