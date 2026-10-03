// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package facet

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// get returns the value id a dictionary got in one facet, or "" for none.
func get(gs []Group, facet string) string {
	for _, g := range gs {
		if g.F == facet {
			return g.V
		}
	}
	return ""
}

func all(gs []Group, facet string) []string {
	var out []string
	for _, g := range gs {
		if g.F == facet {
			out = append(out, g.V)
		}
	}
	return out
}

func TestDeriveLanguage(t *testing.T) {
	tests := []struct {
		name  string
		in    Input
		want  string
		wantP string
	}{
		{"declared wins", Input{Declared: "SpanishModernSort", Name: "Oxford Russian", Path: "/d/fr-collins.mdx"}, "es", ""},
		{"file stem", Input{Name: "Apresyan", Path: "/d/en-ru-apresyan.mdx"}, "en", "en-ru"},
		{"title last", Input{Name: "Dahl's Russian Dictionary", Path: "/d/dahl.dsl"}, "ru", ""},
		{"nothing is nothing", Input{Name: "Kolokviumo", Path: "/d/kolokviumo.mdx"}, "", ""},
		{"never english by default", Input{Name: "Unlabelled", Path: "/d/unlabelled.mdx"}, "", ""},
		{"monolingual pair in title", Input{Name: "Hagen's paradigm (Ru-Ru)", Path: "/d/hagen.dsl"}, "ru", "ru"},
		{"monolingual pair in stem", Input{Name: "DRAE", Path: "/d/es-es-drae.mdx"}, "es", "es"},
		{"declared pair", Input{Declared: "English", Contents: "Russian", Name: "Anon", Path: "/d/anon.dsl"}, "en", "en-ru"},
		{"declared monolingual pair", Input{Declared: "English", Contents: "English", Name: "Anon", Path: "/d/anon.dsl"}, "en", "en"},
		{"full names in title", Input{Name: "Larousse Compact English-Spanish", Path: "/d/larousse.mdx"}, "en", "en-es"},
		{"reverse direction is the same pair", Input{Name: "Oxford Russian-English", Path: "/d/ox.mdx"}, "", "en-ru"},
		{"folder pair", Input{Name: "Apresyan", Path: "/d/En-Ru/apresyan.dsl", Roots: []string{"/d"}}, "", "en-ru"},
		{"hyphen that is not a pair", Input{Name: "Anglo-Saxon Wordhoard", Path: "/d/as.mdx"}, "", ""},
		{"one hint is not a pair", Input{Name: "Oxford English Dictionary", Path: "/d/oed.mdx"}, "en", ""},
		{"han pair", Input{Name: "《牛津高阶英汉双解词典OALD》", Path: "/d/oald.mdx"}, "", "en-zh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := Derive(tc.in)
			// lang is Resolve's business and has its own tests; "" here means
			// the case is about the pair and does not pin the language.
			if got := get(gs, "lang"); tc.want != "" && got != tc.want {
				t.Errorf("lang = %q, want %q", got, tc.want)
			}
			if got := get(gs, "pair"); got != tc.wantP {
				t.Errorf("pair = %q, want %q", got, tc.wantP)
			}
		})
	}
}

// One label per pair whichever way round the dictionary is written, so the two
// directions count as one group in the picker.
func TestPairLabel(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"English-Russian", "English ↔ Russian"},
		{"Russian-English", "English ↔ Russian"},
		{"German-English", "English ↔ German"},
		{"es-es", "Monolingual Spanish"},
		{"英英词典", "Monolingual English"},
	} {
		v, l := languagePair(Input{Name: tc.name})
		if v == "" || l != tc.want {
			t.Errorf("%q: label %q (id %q), want %q", tc.name, l, v, tc.want)
		}
	}
}

func TestHanPair(t *testing.T) {
	for _, tc := range []struct{ s, a, b string }{
		{"牛津高阶英汉双解词典", "en", "zh"},
		{"汉英大词典", "zh", "en"},
		{"新英和中辞典", "en", "ja"},
		{"研究社和英大辞典", "ja", "en"},
		{"俄汉详解大词典", "", ""}, // 详解 is not the word a pair is followed by
		{"俄汉大词典", "ru", "zh"},
		{"英语和汉语词典", "", ""}, // 和 is "and" here
		{"西汉词典", "", ""},    // Western Han, not Spanish
		{"现代汉语词典", "", ""},
		{"", "", ""},
	} {
		if a, b := hanPair(tc.s); a != tc.a || b != tc.b {
			t.Errorf("hanPair(%q) = %q,%q; want %q,%q", tc.s, a, b, tc.a, tc.b)
		}
	}
}

func TestDeriveKind(t *testing.T) {
	tests := []struct {
		title string
		want  []string
	}{
		{"Encyclopaedia Britannica", []string{"encyclopedias"}},
		{"Wikipedia (English)", []string{"encyclopedias"}},
		{"Roget's Thesaurus of Synonyms", []string{"thesauri"}},
		{"Oxford Dictionary of Idioms", []string{"idioms & phrases"}},
		{"Dictionary of American Slang", []string{"slang"}},
		{"Online Etymology Dictionary", []string{"etymology"}},
		{"Black's Law Dictionary", []string{"law"}},
		{"Stedman's Medical Dictionary", []string{"medicine"}},
		{"Dictionary of Abbreviations and Acronyms", []string{"abbreviations"}},
		{"Dictionary of Slang and Idioms", []string{"idioms & phrases", "slang"}}, // file order
		{"Longman Dictionary of Contemporary English", nil},
		{"Malawi Gazetteer", nil}, // "law" must not fire inside a word
		{"The Lawyer's Companion", nil},
		{"Concise Oxford English Dictionary", nil},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			got := all(Derive(Input{Name: tc.title, Path: "/d/x.mdx"}), "content")
			if len(got) != len(tc.want) {
				t.Fatalf("content = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("content = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestDerivePublisher(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Oxford Advanced Learner's Dictionary", "oxford"},
		{"OALD 9", "oxford"},
		{"CALD 4", "cambridge"},
		{"Collins COBUILD Advanced", "collins"},
		{"LDOCE 6", "longman"},
		{"Merriam-Webster's Collegiate Dictionary", "webster"},
		{"Webster's Revised Unabridged Dictionary (1913)", "webster"},
		{"The American Heritage Dictionary", "american heritage"},
		{"Duden - Das große Wörterbuch", "duden"},
		{"Le Robert Micro", "le robert"},
		{"Random House Webster's Unabridged", "webster"}, // groups in file order
		{"Multitran", ""},
		{"Oxfordshire Place Names", "oxford"}, // a plain item matches anywhere
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			if got := get(Derive(Input{Name: tc.title, Path: "/d/x.mdx"}), "publisher"); got != tc.want {
				t.Errorf("publisher = %q, want %q", got, tc.want)
			}
		})
	}
}

// A group's labels must never be empty: the picker renders them verbatim, and
// a blank option is a dead row a user can select.
func TestLabelsPresent(t *testing.T) {
	for _, in := range []Input{
		{Name: "Oxford Russian-English Encyclopedia", Path: "/d/oxford.mdx"},
		{Name: "DRAE", Path: "/d/es-es-drae.mdx"},
	} {
		for _, g := range Derive(in) {
			if g.F == "" || g.FL == "" || g.V == "" || g.VL == "" || g.FO == 0 {
				t.Errorf("%q: incomplete group %+v", in.Name, g)
			}
		}
	}
}

// Facet order is what the picker lists the groups in; it must be ascending
// however many groups a dictionary happens to hold.
func TestFacetOrderAscending(t *testing.T) {
	gs := Derive(Input{Name: "Oxford Russian-English Encyclopedia", Path: "/d/oxford.mdx"})
	if len(gs) != 4 {
		t.Fatalf("want all four facets, got %+v", gs)
	}
	for i := 1; i < len(gs); i++ {
		if gs[i].FO < gs[i-1].FO {
			t.Fatalf("facet order not ascending: %+v", gs)
		}
	}
}

func TestArticleLang(t *testing.T) {
	roots := []string{"/d"}
	tests := []struct {
		name string
		in   Input
		want string
	}{
		{"declared contents wins", Input{Contents: "Russian", Name: "English-French", Path: "/d/en-de.dsl"}, "ru"},
		{"title pair", Input{Name: "Oxford Russian-English", Path: "/d/en-fr.mdx"}, "en"},
		{"file pair, 2-letter", Input{Name: "Collins", Path: "/d/en-fr-collins.mdx"}, "fr"},
		{"file pair, 3-letter", Input{Name: "Elhuyar", Path: "/d/eng-eus.mdx"}, "eu"},
		{"file pair, underscore", Input{Name: "Apresyan", Path: "/d/eng_rus_apresyan.dsl.dz"}, "ru"},
		{"folder pair", Input{Name: "Apresyan", Path: "/d/En-Ru/apresyan.dsl"}, "ru"},
		{"nearest folder pair", Input{Name: "X", Path: "/d/en-fr/de-es/x.mdx"}, "es"},
		{"folder pair not above root", Input{Name: "X", Path: "/en-ru/d/x.mdx", Roots: []string{"/en-ru/d"}}, ""},
		{"outside roots: own folder only", Input{Name: "X", Path: "/en-ru/other/x.mdx"}, ""},
		{"monolingual: headword language", Input{Name: "Oxford English Dictionary", Path: "/d/oed.mdx"}, "en"},
		{"monolingual: declared headword", Input{Declared: "Spanish", Name: "DRAE", Path: "/d/drae.mdx"}, "es"},
		{"monolingual pair", Input{Name: "DRAE", Path: "/d/es-es-drae.mdx"}, "es"},
		{"hyphen that is not a pair", Input{Name: "Anglo-Saxon Wordhoard", Path: "/d/as.mdx"}, ""},
		{"nothing is nothing", Input{Name: "Kolokviumo", Path: "/d/kolokviumo.mdx"}, ""},
		{"han pair", Input{Name: "牛津高阶英汉双解词典", Path: "/d/oald.mdx"}, "zh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.in.Roots == nil {
				tc.in.Roots = roots
			}
			if got := ArticleLang(tc.in); got != tc.want {
				t.Errorf("ArticleLang = %q, want %q", got, tc.want)
			}
		})
	}
}

// The embedded groups.ini parses clean, and every regex in it compiles.
func TestDefaultParsesClean(t *testing.T) {
	r, probs := Parse(DefaultText)
	if len(probs) > 0 {
		t.Fatalf("the embedded groups.ini has problems: %+v", probs)
	}
	var ids []string
	for _, f := range r.facets {
		ids = append(ids, f.id)
	}
	if strings.Join(ids, ",") != "content,publisher" {
		t.Errorf("sections %v, want content,publisher", ids)
	}
}

// The default on titles and file names from a real library (D162 Am. 1): the
// matches substring finds that whole words missed, and the traps the regex
// items exist for.
func TestDefaultOnRealNames(t *testing.T) {
	tests := []struct {
		name, file, facet, want string
	}{
		{"OALDPE En-Cn 精装版 V2026.07.13", "oaldpe.mdx", "publisher", "oxford"},
		{"《牛津高阶英汉双解词典OALD》", "x.mdx", "publisher", "oxford"},
		{"CollinsCobuild (En-En)", "CollinsCobuildEnEn.dsl", "publisher", "collins"},
		{"AHD美语传统双解词典", "AHD双解.mdx", "publisher", "american heritage"},
		{"AHD5 2017", "AHD5.mdx", "publisher", "american heritage"},
		{"LDOCE6 No-Voice", "LDOCE6_NoVoice.mdx", "publisher", "longman"},
		{"MWC", "Merriam-Webster.bgl", "publisher", "webster"},
		{"中国大百科全书", "The Great Chinese Encyclopedia.mdx", "content", "encyclopedias"},
		{"Black's Law Dictionary", "x.mdx", "content", "law"},
		{"lawt100403", "la-en.slob", "content", ""},
		{"Malawi Gazetteer", "x.mdx", "content", ""},
		{"The Lawyer's Companion", "x.mdx", "content", ""},
		{"Encyclopedia of Pesticides", "x.mdx", "publisher", ""},
		{"Schroeder's Handbook", "x.mdx", "publisher", ""},
		{"Caldwell Atlas", "x.mdx", "publisher", ""},
		{"OED2 on CD", "x.mdx", "publisher", "oxford"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := get(Derive(Input{Name: tc.name, File: tc.file}), tc.facet); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.facet, got, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	type grp struct{ facet, group, items string }
	many := ""
	for i := 0; i < maxSections+1; i++ {
		many += fmt.Sprintf("[S%d]\nG = xx\n", i)
	}
	tests := []struct {
		name  string
		text  string
		want  []grp // facet label / group label / items joined by |
		probs []int // the lines reported, once per problem
	}{
		{"the one-liner", "MyPersonalGroup: aaa, bbb, ccc, ddd",
			[]grp{{MyGroups, "MyPersonalGroup", "aaa|bbb|ccc|ddd"}}, nil},
		{"ini form", "[Mine]\nPoetry = poetry, verse", []grp{{"Mine", "Poetry", "poetry|verse"}}, nil},
		{"first of = or : separates", "A=b:c, dd", []grp{{MyGroups, "A", "b:c|dd"}}, nil},
		{"comments and blanks", "; c\n# c\n\n[S] ; note\nG = ww ; note\nH = xx # note",
			[]grp{{"S", "G", "ww"}, {"S", "H", "xx"}}, nil},
		{"BOM and CRLF", "\uFEFF[S]\r\nG = ww\r\n", []grp{{"S", "G", "ww"}}, nil},
		{"plain items are lower-cased, nothing else", "G = Merriam-Webster's,  OED ",
			[]grp{{MyGroups, "G", "merriam-webster's|oed"}}, nil},
		{"a regex is verbatim, commas and comment marks included", "G = `A{2,3};#\\d`, bb ; note",
			[]grp{{MyGroups, "G", "`A{2,3};#\\d`|bb"}}, nil},
		{"repeated group and section merge", "[S]\nG = aa\n[T]\nH = cc\n[s]\ng = bb, aa, `a`",
			[]grp{{"S", "G", "aa|bb|`a`"}, {"T", "H", "cc"}}, nil},
		{"a plain item needs 2 characters; a regex does not", "G = x, ab, `c`, 词典, 词, c#, f#",
			[]grp{{MyGroups, "G", "ab|`c`|词典"}}, []int{1, 1, 1}},
		{"bad lines are skipped, the rest applies", "[S]\nno separator\n= w\nG =\nG2 = ok",
			[]grp{{"S", "G2", "ok"}}, []int{2, 3, 4}},
		{"a bad regex costs only itself", "G = aa, `(`, bb, ``, `x",
			[]grp{{MyGroups, "G", "aa|bb"}}, []int{1, 1, 1}},
		{"text glued to a regex is reported", "G = `x`y, zz\nH = ab`q`",
			[]grp{{MyGroups, "G", "`x`|zz"}, {MyGroups, "H", "`q`"}}, []int{1, 2}},
		{"bad section skips its lines",
			"[S\nG = a\n[]\nH = b\n[Lang]\nI = c\n[T] x\nJ = d\n[language pair]\nL = l\n[" +
				strings.Repeat("n", 65) + "]\nM = m\n[U]\nK = ee",
			[]grp{{"U", "K", "ee"}}, []int{1, 3, 5, 7, 9, 11}},
		{"at most 32 sections", many, nil, []int{2*maxSections + 1}},
		{"empty file", "", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, probs := Parse(tc.text)
			var got []grp
			for _, f := range r.facets {
				for _, g := range f.groups {
					if g.seen != nil {
						t.Error("the parse-time dedup map outlives Parse")
					}
					got = append(got, grp{f.label, g.label, strings.Join(g.items, "|")})
				}
			}
			if tc.name == "at most 32 sections" {
				if len(r.facets) != maxSections {
					t.Errorf("%d sections kept, want %d", len(r.facets), maxSections)
				}
			} else if len(got) != len(tc.want) {
				t.Fatalf("groups %v, want %v", got, tc.want)
			} else {
				for i := range got {
					if got[i] != tc.want[i] {
						t.Errorf("group %d = %v, want %v", i, got[i], tc.want[i])
					}
				}
			}
			var lines []int
			for _, p := range probs {
				lines = append(lines, p.Line)
			}
			if fmt.Sprint(lines) != fmt.Sprint(tc.probs) && !(len(lines) == 0 && len(tc.probs) == 0) {
				t.Errorf("problem lines %v, want %v (%+v)", lines, tc.probs, probs)
			}
		})
	}
}

// An invalid regex says why, in the regex package's own words.
func TestParseRegexMessage(t *testing.T) {
	_, probs := Parse("G = `a(b`")
	if len(probs) != 1 || !strings.Contains(probs[0].Msg, "missing closing )") || strings.Contains(probs[0].Msg, "(?i)") {
		t.Errorf("problems = %+v", probs)
	}
}

// A user's rules replace the default entirely. A plain item matches anywhere
// in the title or the file name; a regex is tested against each separately.
func TestDeriveUserRules(t *testing.T) {
	r, _ := Parse("Mine = aaa, bbb two\nChinese = 词典\n[Content]\nPoetry = verse\n[Re]\nStarts = `^the `\nSlob = `\\.slob$`")
	tests := []struct {
		name string
		in   Input
		want []string // facet/group ids
	}{
		{"title, any case", Input{Name: "The AAA Lexicon", Path: "/d/x.mdx"}, []string{"my groups/mine", "re/starts"}},
		{"inside a word", Input{Name: "xaaay", Path: "/d/x.mdx"}, []string{"my groups/mine"}},
		{"file name, spaces literal", Input{Name: "Lexicon", Path: "/d/bbb two.mdx"}, []string{"my groups/mine"}},
		{"an underscore is not a space", Input{Name: "Lexicon", Path: "/d/bbb_two.mdx"}, nil},
		{"CJK", Input{Name: "牛津高阶英汉双解词典", Path: "/d/oald.mdx"}, []string{"my groups/chinese"}},
		{"regex per field", Input{Name: "Book of the Day", Path: "/d/the day.slob"}, []string{"re/starts", "re/slob"}},
		{"regex anchors the field, not the pair", Input{Name: "Day the", Path: "/d/x.slob.mdx"}, nil},
		{"the user's section", Input{Name: "Collected Verse", Path: "/d/x.mdx"}, []string{"content/poetry"}},
		{"the default is gone", Input{Name: "Oxford Thesaurus", Path: "/d/x.mdx"}, nil},
		{"File wins over Path", Input{Name: "x", Path: "/lib/a.mdx", File: "Mine aaa"}, []string{"my groups/mine"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Rules = r
			var got []string
			gs := Derive(tc.in)
			for i, g := range gs {
				if g.F != "lang" && g.F != "pair" {
					got = append(got, g.F+"/"+g.V)
				}
				if i > 0 && g.FO < gs[i-1].FO {
					t.Errorf("facet order not ascending: %+v", gs)
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Errorf("groups %v, want %v", got, tc.want)
			}
		})
	}
	// a standalone text.db is matched by its library folder, dots and all
	if gs := Derive(Input{Name: "Pocket", Path: "/lib/Pocket.Encyclopedia"}); get(gs, "content") != "encyclopedias" {
		t.Errorf("folder name lost a word: %+v", gs)
	}
}

// Parsing is linear in the items of one group: a whole 256 KB file of them
// stays far below what would stall /api/dicts behind the cache lock.
func TestParseLinear(t *testing.T) {
	var b strings.Builder
	b.WriteString("G = ")
	for i := 0; b.Len() < 256<<10; i++ {
		fmt.Fprintf(&b, "w%d, ", i)
	}
	start := time.Now()
	r, _ := Parse(b.String())
	if d := time.Since(start); d > time.Second {
		t.Errorf("parsing %d items took %v", len(r.facets[0].groups[0].items), d)
	}
}

// The user's own sections - any the built-in list does not have - rank above
// Language, in file order; a built-in section keeps its place after Language
// pair, wherever the file lists it.
func TestUserSectionsRankFirst(t *testing.T) {
	r, _ := Parse("Fav = xx\n[Content]\nPoetry = verse\n[Mine]\nA = yy")
	gs := Derive(Input{Name: "English xx verse yy", Path: "/d/en-ru.mdx", Rules: r})
	var order []string
	for _, g := range gs {
		order = append(order, g.F)
	}
	if got := strings.Join(order, ","); got != "my groups,mine,lang,pair,content" {
		t.Errorf("facet order %s, want my groups,mine,lang,pair,content", got)
	}
	for _, g := range gs {
		if g.FO == 0 {
			t.Errorf("rank 0 is no rank: %+v", g)
		}
	}
}

// What one file may cost: problems are clipped and, past maxProblems, counted
// rather than listed; regexes stop at maxRegexes.
func TestParseLimits(t *testing.T) {
	long := strings.Repeat("y", 4000)
	_, probs := Parse("G = " + strings.Repeat("`("+long+"`, ", 300))
	if len(probs) != maxProblems+1 {
		t.Fatalf("%d problems listed, want %d and a summary", len(probs), maxProblems)
	}
	if last := probs[maxProblems]; last.More != 300-maxProblems || last.Line != 0 {
		t.Errorf("summary = %+v", last)
	}
	if n := CountProblems(probs); n != 300 {
		t.Errorf("CountProblems = %d, want 300", n)
	}
	for _, p := range probs {
		if len(p.Text) > maxProblemLen+len("…") || len(p.Msg) > maxProblemLen+len("…") || !utf8.ValidString(p.Text) {
			t.Fatalf("problem not clipped: text %d bytes, msg %d bytes", len(p.Text), len(p.Msg))
		}
	}
	var b strings.Builder
	b.WriteString("G = ")
	for i := 0; i <= maxRegexes; i++ {
		fmt.Fprintf(&b, "`r%d`, ", i)
	}
	r, probs := Parse(b.String())
	if n := len(r.facets[0].groups[0].res); n != maxRegexes || len(probs) != 1 {
		t.Errorf("%d regexes kept, %d problems; want %d and 1", n, len(probs), maxRegexes)
	}
	if got := clip("ab词典", 3); got != "ab…" {
		t.Errorf("clip cut a rune: %q", got)
	}
}
