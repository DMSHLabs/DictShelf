// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// statsFixture prepares one dictionary and returns its folder and database.
func statsFixture(t *testing.T) (string, string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.dsl")
	if err := os.WriteFile(src, []byte("#NAME test\n\nword\n\tbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	textDB := filepath.Join(dir, TextDBName)
	r := &fakeReader{meta: dict.Meta{Name: "Stats", Format: "dsl", Path: src},
		entries: []dict.Entry{h("alpha", "<p>one two three</p>"), h("beta", "<p>four</p>")}}
	if _, err := IngestPlan(r, textDB, Plan{Contains: true}, nil); err != nil {
		t.Fatal(err)
	}
	meta, err := ReadMeta(textDB)
	if err != nil {
		t.Fatal(err)
	}
	return dir, textDB, meta
}

// What a build recorded is a complete measurement, and it round-trips through
// the sidecar: the file is what a dialog reads when it does not want to open a
// database at all.
func TestStatsCacheRoundTrip(t *testing.T) {
	dir, textDB, meta := statsFixture(t)
	fromMeta, ok := StatsFromMeta(meta)
	if !ok {
		t.Fatal("a build records its measurements in the database: StatsFromMeta said no")
	}
	if fromMeta.Entries != 2 || fromMeta.BodyBytes == 0 || fromMeta.HeadwordBytes == 0 {
		t.Fatalf("measurements from meta = %+v", fromMeta)
	}
	// A build publishes the sidecar now (writeStatsFromMeta), so this test
	// starts from none on purpose: it is about writing one by hand and reading
	// it back.
	if err := os.Remove(StatsPath(dir)); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadStats(textDB); ok {
		t.Error("the sidecar was removed, so there is nothing to read")
	}
	if err := WriteStats(dir, fromMeta); err != nil {
		t.Fatal(err)
	}
	back, ok := ReadStats(textDB)
	if !ok {
		t.Fatal("a sidecar written for this database and source must be accepted")
	}
	if back != fromMeta {
		// MeasuredAt is added by the write.
		back.MeasuredAt, fromMeta.MeasuredAt = "", ""
		if back != fromMeta {
			t.Errorf("read back %+v, wrote %+v", back, fromMeta)
		}
	}
}

// A sidecar describes one database and one source. Anything else in the folder
// - a rebuild, a new source, a local edit - makes it a guess, and a guess is
// what the estimate must not use.
func TestStatsCacheRejectsWhatItDoesNotDescribe(t *testing.T) {
	dir, textDB, meta := statsFixture(t)
	fromMeta, _ := StatsFromMeta(meta)
	if err := WriteStats(dir, fromMeta); err != nil {
		t.Fatal(err)
	}
	// Every case starts from a sidecar that is known to be good: the cases run
	// in order and each of them leaves a rejected file behind.
	read := func(t *testing.T) Stats {
		t.Helper()
		if err := WriteStats(dir, fromMeta); err != nil {
			t.Fatal(err)
		}
		s, ok := ReadStats(textDB)
		if !ok {
			t.Fatal("the sidecar should still be accepted here")
		}
		return s
	}

	t.Run("another database", func(t *testing.T) {
		s := read(t)
		s.IndexUUID = "00000000000000000000000000000000"
		if err := WriteStats(dir, s); err != nil {
			t.Fatal(err)
		}
		if _, ok := ReadStats(textDB); ok {
			t.Error("a sidecar naming another database was accepted")
		}
	})
	t.Run("another source size", func(t *testing.T) {
		s := read(t)
		s.SourceSize++
		if err := WriteStats(dir, s); err != nil {
			t.Fatal(err)
		}
		if _, ok := ReadStats(textDB); ok {
			t.Error("a sidecar from another source was accepted")
		}
	})
	t.Run("another measurement version", func(t *testing.T) {
		s := read(t)
		s.Version++
		if err := WriteStats(dir, s); err != nil {
			t.Fatal(err)
		}
		if _, ok := ReadStats(textDB); ok {
			t.Error("a sidecar from another measurement version was accepted")
		}
	})
	t.Run("a corrupt file", func(t *testing.T) {
		writeStatsOK(t, dir, fromMeta)
		if err := os.WriteFile(StatsPath(dir), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := ReadStats(textDB); ok {
			t.Error("an unreadable sidecar was accepted")
		}
	})
	// Last, because it changes the source the whole fixture shares.
	t.Run("a source changed on disk", func(t *testing.T) {
		writeStatsOK(t, dir, fromMeta)
		src := meta["source_path"]
		if err := os.WriteFile(src, []byte("#NAME test\n\nword\n\tbody edited\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		now := timeNowPlus(t)
		if err := os.Chtimes(src, now, now); err != nil {
			t.Fatal(err)
		}
		if _, ok := ReadStats(textDB); ok {
			t.Error("a sidecar measured before the source changed was accepted")
		}
	})
}

// Prepared folders outlive their sources: deleting one must not turn the
// numbers that describe the database into a mismatch.
func TestStatsCacheSurvivesADeletedSource(t *testing.T) {
	dir, textDB, meta := statsFixture(t)
	fromMeta, _ := StatsFromMeta(meta)
	if err := WriteStats(dir, fromMeta); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(meta["source_path"]); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadStats(textDB); !ok {
		t.Error("a source that is gone is not a source that changed")
	}
}

// A library built before the measurements existed answers false rather than
// zero: an estimate must be able to tell "nothing measured" from "nothing".
func TestStatsFromMetaWithoutTheKeys(t *testing.T) {
	dir, textDB, meta := statsFixture(t)
	delete(meta, "stats_version")
	delete(meta, "body_bytes")
	if _, ok := StatsFromMeta(meta); ok {
		t.Error("meta without a stats version claimed to carry measurements")
	}
	// The sidecar path is a real file location, and reading it without a
	// database of the same identity must not invent one.
	if _, ok := ReadStats(filepath.Join(dir, "missing.text.db")); ok {
		t.Error("a missing database produced measurements")
	}
	_ = textDB
}

func writeStatsOK(t *testing.T, dir string, s Stats) {
	t.Helper()
	if err := WriteStats(dir, s); err != nil {
		t.Fatal(err)
	}
}

func timeNowPlus(t *testing.T) (later time.Time) {
	t.Helper()
	return time.Now().Add(2 * time.Second)
}
