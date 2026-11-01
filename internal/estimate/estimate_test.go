// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package estimate

import "testing"

// following are the measurements out of HANDOFF.md, one row per dictionary:
// what it weighed before full text, what its contains index measured, and what
// full text actually added. The estimate is checked against them, so narrowing
// a band below what the corpus shows fails here rather than in a dialog.
type nonZIMRow struct {
	name     string
	dbBytes  int64
	entries  int64
	contains int64
	growth   int64
}

var nonZIM = []nonZIMRow{
	{"Urban Dictionary 2015", 1021018112, 1418551, 32919552, 610902016},
	{"Oxford English Dictionary 2ed", 421613568, 258793, 5115904, 276189184},
	{"Webster's Unabridged 3", 148078592, 296188, 6479872, 56811520},
	{"Longman DOCE 6", 110100480, 69172, 1904640, 45293568},
	{"WordNet 3.0", 121839616, 148731, 3526656, 32428032},
	{"Oxford Advanced Learner's 10th", 108253184, 70059, 1740800, 32444416},
	{"Lingvo Explanatory", 113860608, 133276, 3362816, 18837504},
	{"Longman DOCE 5th Ed Extras", 68874240, 32357, 688128, 60272640},
	{"est_rus_dict_tm_v01", 76255232, 59332, 1261568, 26030080},
	{"rus_est_dict_tm_v01", 81076224, 75678, 1966080, 16019456},
	{"Cambridge Advanced Learner's 4", 67567616, 58964, 1458176, 24539136},
	{"Oxford Advanced Learner's 8,9", 62787584, 71293, 1695744, 28270592},
	{"Apresyan", 68460544, 114671, 2412544, 22269952},
	{"Oxford American Dictionary 3rd", 55902208, 98529, 2289664, 26124288},
	{"American Heritage 4th", 57708544, 105953, 2387968, 23130112},
	{"Universal Thesaurus", 54460416, 87792, 2101248, 26300416},
	{"Lingvo Universal x6", 55500800, 90061, 1949696, 19943424},
	{"Longman DOCE 5th Ed", 47505408, 54819, 1179648, 24018944},
	{"Ushakov's Dictionary", 50515968, 85852, 2166784, 18444288},
	{"Collins", 46981120, 102247, 2244608, 21315584},
	{"Lingvo Universal (Ru-En)", 56000512, 67944, 1785856, 11919360},
	{"Lubenskaya", 34009088, 24451, 659456, 17412096},
	{"Merriam-Webster's Advanced", 32186368, 51700, 1155072, 17203200},
	{"Universal Idiom", 29626368, 52259, 1789952, 13291520},
	{"Collins COBUILD 8", 26263552, 34282, 716800, 15220736},
	{"Longman Pronunciation", 32514048, 68049, 1368064, 8593408},
	{"Ozhegov Shvedova", 27783168, 39727, 1019904, 11726848},
	{"Mueller 2013", 26636288, 50091, 1036288, 11059200},
	{"Oxford (En-Ru)", 21114880, 35762, 749568, 10260480},
	{"Universal Collocations", 16334848, 27550, 585728, 10907648},
	{"Vasmer", 11800576, 17362, 393216, 9912320},
	{"Oxford American Writer's Thesaurus", 10821632, 15141, 327680, 8790016},
	{"Universal Synonyms", 11194368, 27891, 593920, 7188480},
	{"Zimmerman", 9736192, 15889, 602112, 3280896},
	{"English Dialects 3.0", 7135232, 20178, 512000, 3919872},
	{"Universal Phrasal Verbs", 4739072, 5717, 126976, 5570560},
	{"Lohwater's (Ru-En)", 6250496, 13213, 409600, 1220608},
	{"The BBI Combinatory Dictionary", 4550656, 11011, 241664, 2842624},
}

// The contains index is priced per headword on both families, and this is the
// measurement that says the band is wide enough: the cheapest headword in the
// corpus pays 21.9 bytes and the dearest 42.4 (wikipedia_ru, whose titles are
// long), while the 38 dz/mdx dictionaries average 23.2.
func TestContainsBandCoversTheCorpus(t *testing.T) {
	for _, row := range nonZIM {
		got := Inputs{Entries: row.entries}.Contains()
		if got.Basis != BasisEntries || row.contains < got.Low || row.contains > got.High {
			t.Errorf("%s: contains measured %d, predicted %d-%d (%s)",
				row.name, row.contains, got.Low, got.High, got.Basis)
		}
	}
	for _, row := range zimRows {
		got := Inputs{Entries: row.articles}.Contains()
		if row.contains < got.Low || row.contains > got.High {
			t.Errorf("%s: contains measured %d, predicted %d-%d", row.name, row.contains, got.Low, got.High)
		}
	}
}

// Full text over bodies already in the file is a share of the file, and every
// dictionary in the corpus has to fall inside the band it is quoted from.
func TestFullTextBandCoversTheCorpus(t *testing.T) {
	var measured, low, high int64
	for _, row := range nonZIM {
		got := Inputs{DBBytes: row.dbBytes}.FullText()
		if got.Basis != BasisDatabase {
			t.Fatalf("%s: basis = %s, want %s", row.name, got.Basis, BasisDatabase)
		}
		if row.growth < got.Low || row.growth > got.High {
			t.Errorf("%s: full text measured %d, predicted %d-%d", row.name, row.growth, got.Low, got.High)
		}
		measured += row.growth
		low += got.Low
		high += got.High
	}
	// The total is what the dialog's bottom line shows, and there the band is
	// narrow enough to be worth reading: 4.3-30 GiB for work that measured
	// 16.7 GiB.
	if measured < low || measured > high {
		t.Errorf("total: measured %d, predicted %d-%d", measured, low, high)
	}
	t.Logf("не-ZIM: измерено %d МиБ, предсказано %d-%d МиБ", measured>>20, low>>20, high>>20)
}

// zimRow is a read-through archive: what its directory measured before any
// build, what its contains index came to, and what the database weighed after.
type zimRow struct {
	name       string
	articles   int64
	redirects  int64
	textBytes  int64 // compressed article clusters, from the directory
	indexBytes int64 // article_fts as built
	storedBody int64 // bodies as they cost the file
	dbBytes    int64 // the prepared database afterwards
	contains   int64
}

var zimRows = []zimRow{
	{"wiktionary_ru nopic", 1528188, 999439, 1078917793, 1291063296, 6895775363, 9644752896, 41935000},
	{"wiktionary_en nopic", 9087944, 42006, 4363231833, 4556365824, 15637032557, 25264631808, 228300000},
	{"wikipedia_ru maxi", 2199375, 2825093, 13775113880, 7414427648, 22777146986, 33439162368, 93340000},
	{"wikipedia_de maxi", 3275258, 1878526, 13624003973, 9212135040, 22293036307, 35043479552, 125950000},
}

// A ZIM whose bodies are not in the database is the case the whole estimate
// exists for: the read-through database is a property of the family, told
// apart by how many compressed bytes one article takes.
func TestZIMReadThroughBandCoversTheCorpus(t *testing.T) {
	for _, row := range zimRows {
		in := Inputs{ZIM: true, ZIMArticles: row.articles, ZIMTextBytes: row.textBytes}
		got := in.FullText()
		if got.Basis != BasisZIMText {
			t.Fatalf("%s: basis = %s, want %s", row.name, got.Basis, BasisZIMText)
		}
		if row.dbBytes < got.Low || row.dbBytes > got.High {
			t.Errorf("%s: built %d bytes, predicted %d-%d", row.name, row.dbBytes, got.Low, got.High)
		}
		t.Logf("%s: %d -> предсказано %d-%d, факт %d МиБ",
			row.name, row.textBytes, got.Low>>20, got.High>>20, row.dbBytes>>20)
	}
}

// With the bodies already stored, only the index is added, and that is priced
// against what the bodies cost the file.
func TestZIMIndexOverStoredBodiesCoversTheCorpus(t *testing.T) {
	for _, row := range zimRows {
		in := Inputs{ZIM: true, BodiesPresent: true, StoredBodyBytes: row.storedBody,
			ZIMArticles: row.articles, ZIMTextBytes: row.textBytes}
		got := in.FullText()
		if got.Basis != BasisBodies {
			t.Fatalf("%s: basis = %s, want %s", row.name, got.Basis, BasisBodies)
		}
		if row.indexBytes < got.Low || row.indexBytes > got.High {
			t.Errorf("%s: index measured %d, predicted %d-%d", row.name, row.indexBytes, got.Low, got.High)
		}
	}
}

// An index that is already there adds nothing, and a missing measurement is
// reported as unknown rather than as zero bytes.
func TestBuiltAndUnknown(t *testing.T) {
	if got := (Inputs{Entries: 1000, HasContains: true}).Contains(); !got.Built() || got.Low != 0 {
		t.Errorf("built contains = %+v, want built and zero", got)
	}
	if got := (Inputs{DBBytes: 1 << 30, HasFullText: true}).FullText(); !got.Built() {
		t.Errorf("built full text = %+v, want built", got)
	}
	if got := (Inputs{}).Contains(); got.Basis != BasisUnknown {
		t.Errorf("contains without entries = %+v, want unknown", got)
	}
	if got := (Inputs{}).FullText(); got.Basis != BasisUnknown {
		t.Errorf("full text without a size = %+v, want unknown", got)
	}
	if got := (Inputs{ZIM: true}).FullText(); got.Basis != BasisUnknown {
		t.Errorf("zim full text without directory numbers = %+v, want unknown", got)
	}
	if got := (Inputs{ZIM: true, BodiesPresent: true}).FullText(); got.Basis != BasisUnknown {
		t.Errorf("zim index without stored bodies = %+v, want unknown", got)
	}
}

// The index-only band is quoted in bytes per article, and the four archives
// this fork converted are what it was measured from: 501 and 845 bytes per
// article for the two Wiktionaries, 2809 and 3369 for the two Wikipedias. All
// four have to fall inside the band their family selects - a band that misses
// the measurements it was taken from is a decimal point in the wrong place,
// which is exactly what the first version of this had.
func TestIndexOnlyBandContainsItsMeasurements(t *testing.T) {
	rows := []struct {
		name            string
		articles, textB int64
		perArticle      int64
		encyclopedia    bool
	}{
		{"en-wiktionary", 9087932, 3_100_000_000, 501, false},
		{"ru-wiktionary", 1528188, 480_000_000, 845, false},
		{"ru-wikipedia", 2199370, 5_700_000_000, 3369, true},
		{"de-wikipedia", 3275250, 7_400_000_000, 2809, true},
	}
	for _, row := range rows {
		got := (Inputs{ZIM: true, ArchiveIndex: true, ZIMArticles: row.articles, ZIMTextBytes: row.textB}).FullText()
		if got.Basis != BasisIndexOnly {
			t.Fatalf("%s: basis = %s, want %s", row.name, got.Basis, BasisIndexOnly)
		}
		real := row.articles * row.perArticle
		if real < got.Low || real > got.High {
			t.Errorf("%s: measured %d bytes per article is outside %d..%d",
				row.name, real, got.Low, got.High)
		}
	}
}
