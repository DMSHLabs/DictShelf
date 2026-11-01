// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package estimate predicts what an index will cost before it is built.
//
// Every coefficient here was measured on prepared dictionaries rather than
// chosen: the series and the per-dictionary tables are in HANDOFF.md ("What
// each index costs on the non-ZIM library" and "What a read-through ZIM
// costs"). The inputs are the measurements a prepared database already
// carries - the ingest records body_bytes, stored_body_bytes and
// headword_bytes (SPEC, meta), and a ZIM's reader records what its directory
// held - so a prediction needs no second pass over a source that may no
// longer be reachable.
//
// A prediction is a band, never a point. The measurements it comes from have a
// spread of one and a half to two times within a family, and a number that is
// wrong by a factor is worse than no number when the decision it informs is
// "spend thirty gigabytes or not".
package estimate

// Version identifies the coefficient table. It is data a cache may key on; the
// sidecar that stores measurements does not need it, because a measurement is
// a fact about a source and a prediction is recomputed from it.
const Version = 1

// Contains indexes headwords, so it is priced per headword and nothing about
// the bodies matters. Measured across 42 dictionaries in two families: 23.2
// bytes per entry over the 38 dz/mdx ones (4 010 535 entries, 88.6 MiB of
// index), and 19.8 to 42.4 per entry over the whole corpus - the cheap end is
// the Oxford English Dictionary, whose headwords are short, the dear end is
// wikipedia_ru, whose titles are long. The band is those measurements rounded
// outward, which is what a band is for: a dictionary can land anywhere in it.
const (
	containsLow  = 18
	containsHigh = 45
)

// FullText over bodies that are already in the file is only an index, and it
// is priced against the file it lands in: measured as the growth of the 38
// prepared dz/mdx dictionaries when full text was built for them, 16.5-117.6 %
// of the file before, median 45 %. The spread comes from deflate: the index is
// built over the bodies as text while the file holds them compressed, so a
// dictionary whose bodies compressed badly pays more for the index than the
// bodies themselves take - which is why this is a band and why the
// body-accurate coefficient waits for stored_body_bytes to exist in a library.
const (
	fullTextShareLow  = 0.16
	fullTextShareHigh = 1.18
)

// A ZIM is different twice over, and the difference is not a detail: its full
// text is a read-through build that copies article bodies into the database,
// not an index over something already there.
//
// With the bodies already stored (a database whose index_kind is "articles"),
// building the index alone cost 18.7-41.3 % of the stored bodies across the
// four archives.
const (
	zimIndexShareLow  = 0.18
	zimIndexShareHigh = 0.42
)

// Without the bodies, the whole read-through cost is the archive's compressed
// article text times a factor that is a property of the family: 2.43-2.57 for
// the two Wikipedias (whose articles are long and compress poorly) against
// 5.79-8.94 for the two Wiktionaries (short entries full of repeated markup,
// which compress well). The families are told apart by the compressed bytes a
// single article takes, which is measurable before anything is built.
const (
	zimEncyclopediaLow, zimEncyclopediaHigh = 2.4, 2.6
	zimDictionaryLow, zimDictionaryHigh     = 5.7, 9.0
	zimFamilyBytesPerArticle                = 1500
)

// Inputs are what a prepared dictionary says about itself and its source. Zero
// means "not measured", never zero bytes: a caller that has no measurement
// gets BasisUnknown rather than a confident nothing.
type Inputs struct {
	// ZIM marks the format whose full text is a read-through copy.
	ZIM bool
	// BodiesPresent says the prepared database already carries article bodies
	// (a ZIM's read-through form, store.ZIMForm's "articles"). For every other
	// format the bodies are always in the file.
	BodiesPresent bool

	Entries         int64
	HeadwordBytes   int64 // headwords as UTF-8 bytes
	BodyBytes       int64 // bodies before deflate
	StoredBodyBytes int64 // bodies as they cost the file
	DBBytes         int64 // the prepared database as it stands now

	// ZIM only, from the archive's directory (zim.DirectoryStats).
	ZIMArticles  int64
	ZIMTextBytes int64 // compressed bytes of clusters holding article text

	HasContains, HasFullText bool
}

// Basis names the measurement a prediction was derived from. It is a key, not
// a sentence: the client says what the number is based on, in its own language.
type Basis string

const (
	BasisBuilt    Basis = "built"    // the index is already in the database
	BasisEntries  Basis = "entries"  // per headword
	BasisDatabase Basis = "database" // a share of the current file
	BasisBodies   Basis = "bodies"   // a share of the bodies stored in the file
	BasisZIMText  Basis = "zim_text" // a multiple of the archive's compressed article text
	BasisUnknown  Basis = "unknown"  // nothing to base it on
)

// Result is the band an index is expected to add, in bytes. Low == High == 0
// with BasisBuilt means there is nothing to add.
type Result struct {
	Low, High int64
	Basis     Basis
}

// Built reports whether the index is already there.
func (r Result) Built() bool { return r.Basis == BasisBuilt }

// Contains predicts the headword index. It is the same work whatever the
// dictionary holds: the trigram and headword FTS tables are built from the
// headwords alone.
func (in Inputs) Contains() Result {
	if in.HasContains {
		return Result{Basis: BasisBuilt}
	}
	if in.Entries <= 0 {
		return Result{Basis: BasisUnknown}
	}
	return Result{Low: in.Entries * containsLow, High: in.Entries * containsHigh, Basis: BasisEntries}
}

// FullText predicts the article index, or - for a ZIM whose bodies are not in
// the database - the read-through build that is what "full text" means there.
//
// The returned addition is measured against the file as it stands: for dz/mdx
// the bodies are already stored, so what is added is the index; for a ZIM
// without bodies almost the whole database is added, because the title-only
// base it replaces is about 2 % of the read-through size (measured: 193 MiB
// against 8.98 GiB for one Wiktionary).
func (in Inputs) FullText() Result {
	if in.HasFullText {
		return Result{Basis: BasisBuilt}
	}
	if !in.ZIM {
		if in.DBBytes <= 0 {
			return Result{Basis: BasisUnknown}
		}
		return Result{
			Low:   int64(float64(in.DBBytes) * fullTextShareLow),
			High:  int64(float64(in.DBBytes) * fullTextShareHigh),
			Basis: BasisDatabase,
		}
	}
	if in.BodiesPresent {
		if in.StoredBodyBytes <= 0 {
			return Result{Basis: BasisUnknown}
		}
		return Result{
			Low:   int64(float64(in.StoredBodyBytes) * zimIndexShareLow),
			High:  int64(float64(in.StoredBodyBytes) * zimIndexShareHigh),
			Basis: BasisBodies,
		}
	}
	if in.ZIMArticles <= 0 || in.ZIMTextBytes <= 0 {
		return Result{Basis: BasisUnknown}
	}
	low, high := zimDictionaryLow, zimDictionaryHigh
	if in.ZIMTextBytes/in.ZIMArticles >= zimFamilyBytesPerArticle {
		low, high = zimEncyclopediaLow, zimEncyclopediaHigh
	}
	return Result{
		Low:   int64(float64(in.ZIMTextBytes) * low),
		High:  int64(float64(in.ZIMTextBytes) * high),
		Basis: BasisZIMText,
	}
}
