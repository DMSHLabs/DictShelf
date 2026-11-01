// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
)

// Two bugs, one shape: a strict reading reported articles as absent that plainly
// contain the phrase. Both were found on the same live Wikipedia article
// (Ниммонс, Фил, 2026-11-02), and both are pinned here so the reading cannot go
// quiet again.
//
//  1. The markup stripper this package used to carry counted every closing
//     </script> as an opening tag, so the skip it kept never ended and the whole
//     of the article after the first script or style block was dropped. Phrase
//     and near search then matched only the head of the document - the title -
//     which is why a single word worked and a real phrase found nothing.
//  2. The article text was folded with the QUERY folding, which drops repeated
//     words. The profession "кларнетист" stands in the infobox before it stands
//     in the first sentence, so the occurrence the phrase needed was the one
//     dropped.
//
// The body below is that article's shape: a head with script and style, an
// infobox naming the profession, and the phrase in the first sentence with the
// two words separated by a wikilink - the ordinary case, not a corner.
const articleWithPhrase = `<!DOCTYPE html><html><head><title>Ниммонс, Фил</title>
<style>.infobox{color:red}</style><script>window.x=1;</script></head>
<body><table class="infobox"><tr><th>Профессии</th><td>кларнетист, композитор</td></tr></table>
<p>Ниммонс — <a rel="mw:WikiLink" href="entry://Канада" title="Канада">канадский</a>
<a rel="mw:WikiLink" href="entry://Джаз" title="Джаз">джазовый</a>
<a rel="mw:WikiLink" href="entry://Кларнет" title="Кларнет">кларнетист</a>, бэнд-лидер.</p>
</body></html>`

func TestArticleTokensKeepTheProseAndItsRepeats(t *testing.T) {
	tokens := articleTokens([]byte(articleWithPhrase))
	joined := strings.Join(tokens, " ")

	// The prose after the script/style block has to be there at all.
	if !strings.Contains(joined, "бэнд-лидер") && !strings.Contains(joined, "лидер") {
		t.Errorf("the article after its script and style blocks was dropped: %v", tokens)
	}
	// And the phrase has to verify: adjacent, in this order.
	want := glass.FoldText("джазовый кларнетист")
	if !phraseIn(tokens, want) {
		t.Errorf("the phrase in the article did not verify; tokens around it: %v", tokens)
	}
	// The repeated word survives: it stands in the infobox and in the sentence.
	repeats := 0
	for _, tok := range tokens {
		if tok == want[1] {
			repeats++
		}
	}
	if repeats < 2 {
		t.Errorf("the repeated word appears %d time(s) in the token sequence, want 2: %v", repeats, tokens)
	}
	// A phrase whose words are not adjacent anywhere must not verify.
	if phraseIn(tokens, glass.FoldText("композитор канадский")) {
		t.Error("a phrase that is not in the article verified")
	}
}

// The near reading counts positions, so it needs the repeats too: the words are
// three apart in the sentence, and the infobox copy of "кларнетист" must not be
// the only one the check can see.
func TestNearSeesTheSentencesOccurrence(t *testing.T) {
	tokens := articleTokens([]byte(articleWithPhrase))
	near := glass.FoldText("канадский кларнетист")
	if !nearIn(tokens, near, ReadingNearDistance) {
		t.Errorf("a pair two words apart did not verify as near: %v", tokens)
	}
}
