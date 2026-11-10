// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"strings"
	"testing"
)

// The status bar walks two things, and the row says which: the pair on the left
// steps articles, and the pair that flanks the dictionary chip steps
// dictionaries. The two walks are told apart by position and by glyph, so both
// have to be asserted - four arrows in one row that a reader cannot tell apart
// are worse than the one pair the row had.
func TestTheBarWalksArticlesAndDictionaries(t *testing.T) {
	template := string(newTestServer(t).basePage())
	at := func(id string) int {
		i := strings.Index(template, `id="`+id+`"`)
		if i < 0 {
			t.Fatalf("the bar lost #%s", id)
		}
		return i
	}
	articlePrev, articleNext := at("sbarArticlePrev"), at("sbarArticleNext")
	find, dictPrev, chip, dictNext := at("sbarFind"), at("sbarDictPrev"), at("dictSeg"), at("sbarDictNext")
	examples := at("sbarExamples")
	if !(articlePrev < articleNext && articleNext < find) {
		t.Error("the article pair does not stand left of the Find control")
	}
	if !(find < dictPrev && dictPrev < chip && chip < dictNext && dictNext < examples) {
		t.Error("the dictionary pair does not flank the chip")
	}
	for _, gone := range []string{`id="sbarPrev"`, `id="sbarNext"`} {
		if strings.Contains(template, gone) {
			t.Errorf("the old id %s is still in the row", gone)
		}
	}
	// Each arrow names its own walk and wears its own glyph: an arrow whose
	// label says "dictionary" while its neighbour says "article" is the exact
	// confusion this row is arranged to prevent.
	pairs := []struct{ id, glyph, label, title string }{
		{"sbarArticlePrev", "↑", "search.previousArticle", "search.jumpArticleAbove"},
		{"sbarArticleNext", "↓", "search.nextArticle", "search.jumpArticleBelow"},
		{"sbarDictPrev", "▲", "search.previousDict", "search.jumpAbove"},
		{"sbarDictNext", "▼", "search.nextDict", "search.jumpBelow"},
	}
	for _, p := range pairs {
		i := at(p.id)
		close := strings.Index(template[i:], ">")
		if close < 0 {
			t.Fatalf("#%s is not closed", p.id)
		}
		tag := template[i : i+close]
		if !strings.Contains(tag, "{{T:"+p.label+"}}") {
			t.Errorf("#%s does not name %s", p.id, p.label)
		}
		if !strings.Contains(tag, "{{T:"+p.title+"}}") {
			t.Errorf("#%s does not carry %s", p.id, p.title)
		}
		if got := template[i+close+1:]; !strings.HasPrefix(got, p.glyph) {
			t.Errorf("#%s does not wear %s", p.id, p.glyph)
		}
	}
}
