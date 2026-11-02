// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A prepared folder holds three files: the dictionary itself, the receipt that
// describes it, and the icon cache. This is the fourth: the measurements a size
// estimate reads, kept beside the database the way icon.cache is, and keyed the
// same way - the database's own uuid plus the source it was built from - so a
// folder that moved, was renamed or was copied to another machine is still
// recognised as the one the numbers describe.
//
// It is a CACHE, not a source of truth: every number in it can be measured
// again (a ZIM's from its directory, a dz/mdx one's from its bodies), the file
// may be missing or unreadable, and the medium may not allow writing at all.
// Nothing in the program may depend on it existing; what it buys is a dialog
// that opens with numbers instead of a progress bar.
const StatsCacheName = "stats.cache"

// Stats are the measurements about a dictionary and its source. Everything here
// describes the SOURCE or the container - never the prepared content - which is
// why it can be recorded after a build without changing what the database is.
type Stats struct {
	Version     int    `json:"version"` // == StatsVersion: how they were measured
	IndexUUID   string `json:"index_uuid"`
	SourceSize  int64  `json:"source_size"`
	SourceMTime string `json:"source_mtime"`
	MeasuredAt  string `json:"measured_at,omitempty"`

	Entries         int64 `json:"entries,omitempty"`
	BodyBytes       int64 `json:"body_bytes,omitempty"`
	StoredBodyBytes int64 `json:"stored_body_bytes,omitempty"`
	HeadwordBytes   int64 `json:"headword_bytes,omitempty"`

	// ZIM only: which prepared form the database is in ("headwords" or
	// "articles", store.ZIMForm) and what its directory held.
	ZIMForm      string `json:"zim_form,omitempty"`
	ZIMArticles  int64  `json:"zim_articles,omitempty"`
	ZIMRedirects int64  `json:"zim_redirects,omitempty"`
	ZIMMedia     int64  `json:"zim_media,omitempty"`
	ZIMOther     int64  `json:"zim_other,omitempty"`
	ZIMClusters  int64  `json:"zim_clusters,omitempty"`
	// The FTS5 index sizes, in bytes. They are measured with dbstat, which is a
	// page-by-page walk of the database - seconds on a large one - so they are
	// measured ONCE, by the pass that fills this cache, and never in the path
	// that opens a dictionary: a listing that walks every page of every
	// database takes a minute before it can show a name (found 2026-11-02).
	ContainsIndexBytes int64 `json:"contains_index_bytes,omitempty"`
	FullTextIndexBytes int64 `json:"full_text_index_bytes,omitempty"`

	ZIMTextBytes  int64 `json:"zim_text_bytes,omitempty"`
	ZIMMixedBytes int64 `json:"zim_mixed_bytes,omitempty"`
	ZIMOtherBytes int64 `json:"zim_other_bytes,omitempty"`
}

// StatsPath is where a folder's measurements live.
func StatsPath(dir string) string { return filepath.Join(dir, StatsCacheName) }

// StatsIdentity fills what any prepared database knows about itself and its
// source, whatever version wrote it: the identity a cache is keyed by and the
// entry count. Measurements stay zero - a database prepared before they were
// recorded answers honestly rather than pretending to a measurement of nothing.
func StatsIdentity(meta map[string]string) Stats {
	return Stats{
		IndexUUID:   meta["dict_uuid"],
		SourceSize:  statInt(meta["source_size"]),
		SourceMTime: meta["source_mtime"],
		Entries:     statInt(meta["entry_count"]),
	}
}

// StatsFromMeta reads the measurements a build recorded in the database itself
// (body_bytes and friends, SPEC meta). A library prepared before those keys
// existed answers false, which is what sends the caller to measure instead.
func StatsFromMeta(meta map[string]string) (Stats, bool) {
	if statInt(meta["stats_version"]) != StatsVersion {
		return Stats{}, false
	}
	s := StatsIdentity(meta)
	s.Version = StatsVersion
	s.ZIMForm = ZIMFormOf(meta)
	s.BodyBytes = statInt(meta["body_bytes"])
	s.StoredBodyBytes = statInt(meta["stored_body_bytes"])
	s.HeadwordBytes = statInt(meta["headword_bytes"])
	s.ZIMArticles = statInt(meta["zim_articles"])
	s.ZIMRedirects = statInt(meta["zim_redirects"])
	s.ZIMMedia = statInt(meta["zim_media"])
	s.ZIMOther = statInt(meta["zim_other"])
	s.ZIMClusters = statInt(meta["zim_clusters"])
	s.ZIMTextBytes = statInt(meta["zim_text_bytes"])
	s.ZIMMixedBytes = statInt(meta["zim_mixed_bytes"])
	s.ZIMOtherBytes = statInt(meta["zim_other_bytes"])
	return s, true
}

// ReadStats returns a folder's cached measurements when they describe this
// database and this source, and false in every other case: no file, unreadable
// JSON, a measurement version this build does not understand, a different
// database in the folder, or a source that has changed since it was measured.
//
// A source that is GONE is not a mismatch. Prepared folders outlive their
// sources by design (SPEC: sources can be deleted), and the numbers describe the
// database that is here; the receipt in the database carries the same source
// fingerprint they were taken against.
func ReadStats(textDB string) (Stats, bool) {
	meta, err := ReadMeta(textDB)
	if err != nil {
		return Stats{}, false
	}
	dir := filepath.Dir(textDB)
	raw, err := os.ReadFile(StatsPath(dir))
	if err != nil {
		return Stats{}, false
	}
	var s Stats
	if err := json.Unmarshal(raw, &s); err != nil {
		return Stats{}, false
	}
	if s.Version != StatsVersion || s.IndexUUID == "" || s.IndexUUID != meta["dict_uuid"] {
		return Stats{}, false
	}
	if s.SourceSize != statInt(meta["source_size"]) || s.SourceMTime != meta["source_mtime"] {
		return Stats{}, false
	}
	if src := meta["source_path"]; src != "" {
		if fi, err := os.Stat(src); err == nil {
			if fi.Size() != s.SourceSize || fi.ModTime().UTC().Format(time.RFC3339) != s.SourceMTime {
				return Stats{}, false
			}
		}
	}
	// The FORM is a property of the database, not of the source, and a rebuild
	// into the other form leaves every other check here passing: the uuid is the
	// source's, and the source has not changed. Without this a ZIM rebuilt from
	// the read-through form to the title index went on being measured as if it
	// still stored article bodies - the window would price it against bodies
	// that are not there. Found by rebuilding one and reading the result.
	//
	// Only a database that DECLARES a form is compared: a non-ZIM never has one,
	// and neither does a database from before the two forms existed.
	if form := ZIMFormOf(meta); form != "" && form != s.ZIMForm {
		return Stats{}, false
	}
	return s, true
}

// WriteStats records a folder's measurements, best effort: a library on a
// read-only medium, a full disk or a concurrent write must leave the estimate
// working from what it can measure rather than failing.
//
// The file is written in place rather than through a temporary and a rename.
// Atomicity is not needed for a cache whose reader validates its content - a
// half-written file fails to parse and is treated as absent - while a stray
// temporary file in a prepared folder is something every listing of that folder
// would have to know about.
func WriteStats(dir string, s Stats) error {
	if s.Version == 0 {
		s.Version = StatsVersion
	}
	s.MeasuredAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(StatsPath(dir), b, 0o644)
}

// ZIMFormOf renders the prepared form for formats that have more than one. A
// dictionary that is not a ZIM answers "", which is also what a ZIM prepared
// before the form was recorded answers.
func ZIMFormOf(meta map[string]string) string {
	if meta == nil || !strings.EqualFold(meta["format"], "zim") {
		return ""
	}
	if _, articles := ZIMForm(meta); articles {
		return "articles"
	}
	return "headwords"
}

func statInt(v string) int64 {
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}
