// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// preparedForServer prepares one dictionary into the isolated library and
// returns the registry that sees it.
func preparedForServer(t *testing.T, format string) (*Registry, *entry, string) {
	t.Helper()
	isolatedDBDir(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "sample."+format)
	if err := os.WriteFile(source, []byte("#NAME sample\n\nword\n\tbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	preparedDir, err := store.ClaimDir(source)
	if err != nil {
		t.Fatal(err)
	}
	reader := &stubReader{meta: dict.Meta{Name: "sample", Format: format, Path: source}}
	if _, err := store.IngestPlan(reader, store.TextDBPath(preparedDir), store.Plan{Contains: true}, nil); err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry([]string{dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range reg.all() {
		if e.Path == source {
			return reg, e, store.TextDBPath(preparedDir)
		}
	}
	t.Fatalf("the prepared dictionary is not in the registry: %d entries", reg.Count())
	return nil, nil, ""
}

// The warm pass is what fills the measurements a dialog needs, and it must do
// it once: the second start of the application has nothing to measure.
func TestWarmStatsMeasuresOnceAndCaches(t *testing.T) {
	// The registry measures every prepared dictionary by itself, right after it
	// is built, and writes the same stats.cache this test measures by hand with
	// WriteStats - which writes in place, because a cache its reader validates
	// needs no atomicity (store/dmsh_stats_cache.go). A concurrent read can
	// therefore catch a half-written file: fine for the product, which rejects
	// it and measures again, and fatal for a test that asserts one measurement's
	// own cache. So the registry's pass is turned off here rather than raced
	// with. No test in this package runs in parallel, so the switch is safe.
	keepWarm := warmEnabled
	warmEnabled = false
	t.Cleanup(func() { warmEnabled = keepWarm })

	reg, e, textDB := preparedForServer(t, "dsl")
	if !reg.statsNeeded(e) {
		t.Fatal("a dictionary without a cache needs measuring")
	}
	s, err := reg.measureStats(e)
	if err != nil {
		t.Fatal(err)
	}
	if s.Entries == 0 || s.IndexUUID == "" || s.SourceSize == 0 {
		t.Fatalf("measurements = %+v, want identity and the entry count from meta", s)
	}
	cached, ok := store.ReadStats(textDB)
	if !ok {
		t.Fatal("after measuring, the cache beside the database must be valid")
	}
	if cached.Entries != s.Entries {
		t.Errorf("cached entries = %d, want %d", cached.Entries, s.Entries)
	}
	if reg.statsNeeded(e) {
		t.Error("a measured dictionary still wants measuring: the dialog would never read the cache")
	}
	// A second pass over the same registry must not touch the file: skipping
	// what is already measured is the pass's job, not the dialog's.
	path := store.StatsPath(filepath.Dir(textDB))
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	reg.warmStats(reg.all())
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("the pass measured a dictionary that was already measured")
	}
}

// A dictionary whose source has been deleted is still a prepared dictionary:
// the measurements describe it, and the pass must not fail on one.
func TestMeasureStatsWithoutItsSource(t *testing.T) {
	reg, e, textDB := preparedForServer(t, "dsl")
	src := e.Path
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.measureStats(e); err != nil {
		t.Fatalf("a source that is gone is not a failure: %v", err)
	}
	s, ok := store.ReadStats(textDB)
	if !ok {
		t.Fatal("the cache must be valid without the source")
	}
	// SourceSize is recorded from the meta the build wrote, so the cache stays
	// keyed to the source this database was prepared from.
	if s.SourceSize == 0 {
		t.Error("identity was lost when the source went away")
	}
}

// A dictionary whose bodies are already in its database needs no measurement to
// be estimated: the band is a share of the file it has. That is why the page
// works on a library prepared before any of this existed.
func TestListRowEstimatesWithoutMeasurements(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	e, err := s.reg.get(idOf(t, s, "test.dsl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, cached := store.ReadStats(store.TextDBPath(dir)); cached {
		t.Fatal("the fixture must start without a measurement cache")
	}
	row, _ := s.dictInfoFor(e, "", "")
	s.attachEstimate(&row)
	if row.Caps.FTS {
		t.Fatal("the fixture was prepared without full text")
	}
	if row.Estimate == nil || row.Estimate.Basis != "database" {
		t.Fatalf("estimate = %+v, want a band from the file's size", row.Estimate)
	}
	if row.Estimate.Low <= 0 || row.Estimate.High < row.Estimate.Low {
		t.Fatalf("estimate = %+v", row.Estimate)
	}
}

// A ZIM without the bodies in its database is the case that needs the archive's
// directory: with the numbers measured it gets a read-through band, and without
// them it gets none - which is what the page shows as "Calculating".
func TestZIMRowEstimatesFromTheDirectory(t *testing.T) {
	s := newTestServer(t)
	dir := preparedDSL(t, s)
	zimDir := t.TempDir()
	textDB := filepath.Join(zimDir, store.TextDBName)
	if err := copyFile(store.TextDBPath(dir), textDB); err != nil {
		t.Fatal(err)
	}
	meta, err := store.ReadMeta(textDB)
	if err != nil {
		t.Fatal(err)
	}
	row := dictInfo{ID: "zim", Name: "Wikipedia", Format: "zim", DBPath: textDB, Entries: 1000}
	base := store.StatsIdentity(meta)
	base.Version = store.StatsVersion

	s.attachEstimate(&row)
	if row.Estimate != nil {
		t.Fatalf("a ZIM with nothing measured offered %+v", row.Estimate)
	}

	readThrough := base
	readThrough.ZIMForm = "headwords"
	readThrough.ZIMArticles = 1000
	readThrough.ZIMTextBytes = 4 << 20 // 4 KiB an article: the encyclopedia branch
	if err := store.WriteStats(zimDir, readThrough); err != nil {
		t.Fatal(err)
	}
	row.Estimate = nil
	s.attachEstimate(&row)
	if row.Estimate == nil || row.Estimate.Basis != "zim_text" {
		t.Fatalf("estimate = %+v, want a read-through band", row.Estimate)
	}

	indexOnly := base
	indexOnly.ZIMForm = "articles"
	indexOnly.StoredBodyBytes = 40 << 20
	if err := store.WriteStats(zimDir, indexOnly); err != nil {
		t.Fatal(err)
	}
	row.Estimate = nil
	s.attachEstimate(&row)
	if row.Estimate == nil || row.Estimate.Basis != "bodies" {
		t.Fatalf("estimate = %+v, want a band over the stored bodies", row.Estimate)
	}
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o600)
}

// The page that spends the gigabytes needs the one number a browser cannot
// work out: how much room the library's volume has left.
func TestDictionaryListReportsFreeSpace(t *testing.T) {
	s := newTestServer(t)
	preparedDSL(t, s)
	var end dictMsg
	seen := 0
	if _, err := s.listDictionaries(context.Background(), "", func(m dictMsg) error {
		if m.T == "end" {
			end = m
			seen++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Fatalf("end messages = %d, want 1", seen)
	}
	if end.FreeBytes <= 0 {
		t.Errorf("freeBytes = %d, want the volume's free space", end.FreeBytes)
	}
	if end.LibraryVolume == "" {
		t.Error("the response must name the volume it is talking about")
	}
}
