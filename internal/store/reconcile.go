// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/logx"
)

// Reconcile is the one way prepared data changes: whoever prepares, rebuilds
// or packs a dictionary - the panel's switches, the background indexer, the
// abbreviation sweep, the Rebuild job, `wudict ingest`, `wudict reindex`, and
// the formats that prepare themselves on open - states the result it wants as
// a Target and gets the same rules for what is kept, what is rebuilt and when.
// What differs between those callers is policy, and the policy is the Target.

// MediaGoal is what Reconcile does with the media.db beside the text.
type MediaGoal int

const (
	// MediaLeave touches nothing. Packing is the user's decision (D24), so
	// preparation nobody asked for never packs - and a rebuild from a changed
	// source leaves the old media.db unpaired, reported outdated (D151).
	MediaLeave MediaGoal = iota
	// MediaKeep repacks media that was packed and no longer pairs with the
	// text: a rebuild the user asked for keeps everything they had.
	MediaKeep
	// MediaRepack repacks media that was packed, paired or not (reindex -all).
	MediaRepack
	// MediaOn packs media unless a paired media.db is already there.
	MediaOn
	// MediaOff deletes the media.db. Media can be packed again from the
	// source it came from.
	MediaOff
)

// Target is the state Reconcile brings one dictionary's prepared data to.
// The zero Target is "prepared, at whatever level it already has": build a
// missing or unusable database, or one built from an older edition of the
// source, and nothing else.
type Target struct {
	// FullText and Contains set one index each; nil keeps what is built. A
	// dictionary never prepared - or whose database cannot be read - has
	// nothing to keep and starts from headwords only (D24).
	FullText, Contains *bool

	Media MediaGoal

	// Rebuild decides whether text that is usable, built from the source as
	// it is now and on the wanted plan, is rebuilt anyway, given the reasons
	// it is outdated (Prepared.TextStale; empty when it is current). Nil
	// never does: preparation nobody asked for leaves outdated data alone
	// until the user asks (D151).
	Rebuild func(why []Reason) bool

	// Dir is the library folder to work in, for a caller that already holds
	// it (`wudict reindex` walks the folders). Empty finds the source's folder,
	// claiming one if it has none.
	Dir string

	// Out writes the text to this database instead of the library folder:
	// a new database for its caller, always written, from the plan given
	// alone (`wudict ingest -o`).
	Out string
}

// IfOutdated is the Rebuild rule of a user asking for current data.
func IfOutdated(why []Reason) bool { return len(why) > 0 }

// Always is the Rebuild rule of `reindex -all`: current data too.
func Always([]Reason) bool { return true }

// Hooks are what a caller supplies around the work. All are optional.
type Hooks struct {
	// Reader opens the source for an ingest, and is called only when one is
	// needed. The caller owns what it returns. Nil opens dict.OpenReader and
	// closes it.
	Reader func() (dict.Reader, error)

	// Backend is the dictionary media is packed from; done is called when
	// the pack is over. Nil opens the source with dict.Open and closes it.
	Backend func() (d dict.Dictionary, done func(), err error)

	// Release runs before the text.db, or the media.db beside it, is
	// replaced or deleted: the moment anything holding either must let go,
	// because Windows refuses a rename over an open file.
	Release func(textDB string)

	Progress      Progress // entries ingested
	MediaProgress Progress // media files packed
}

// Outcome is what Reconcile did. Libraries return, callers print (logx).
type Outcome struct {
	TextDB string
	Plan   Plan // what the text is built with now

	Rebuilt  bool          // the text was (re)written
	Why      []Reason      // why an existing database was rebuilt; nil for a first build
	Report   Report        // of the ingest, when Rebuilt
	TextTime time.Duration // of the ingest, when Rebuilt

	Packed       int           // media files packed
	Beside       int           // of those, files packed from beside the source, not from its own container
	MediaTime    time.Duration // of the pack, when Packed
	MediaEmpty   bool          // packing was asked for and there was nothing to pack
	MediaCurrent bool          // packing was asked for and a paired media.db was already there
	MediaRemoved bool          // a media.db was deleted: asked for, or unpaired with nothing to pack
}

// Changed reports that anything on disk changed.
func (o Outcome) Changed() bool { return o.Rebuilt || o.Packed > 0 || o.MediaRemoved }

// textLocks serialises the text phase of Reconcile per dictionary. The
// server's switches and lanes, and a self-preparing format's open (DSL, BGL,
// wudict markdown prepare inside dict.Open), can reach one dictionary at the
// same moment; two ingests racing into one text.db would leave whichever
// finished last, possibly on the wrong plan. The second caller waits, then
// finds the text current and builds nothing.
var textLocks sync.Map // source path or -o target -> *sync.Mutex

func lockText(key string) func() {
	if abs, err := filepath.Abs(key); err == nil {
		key = filepath.Clean(abs)
	}
	m, _ := textLocks.LoadOrStore(key, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Reconcile brings the prepared data of the dictionary at src to t. The text
// phase is serialised per dictionary (textLocks). The media phase is not: it
// may open the source, and opening a self-preparing format re-enters
// Reconcile, so it must run outside that lock - and only the server's switches,
// the Rebuild job and the CLI pack media, each already serialised by its
// caller.
func Reconcile(src string, t Target, h Hooks) (out Outcome, err error) {
	key := src
	if t.Out != "" {
		key = t.Out
	}
	unlock := lockText(key)
	p, dir, err := reconcileText(src, t, h, &out)
	unlock()
	if err != nil {
		return out, err
	}
	textDB := out.TextDB
	release := func() {
		if h.Release != nil {
			h.Release(textDB)
		}
	}

	mediaDB := MediaSibling(textDB)
	switch t.Media {
	case MediaOff:
		if mediaDB != "" && fsx.FileExists(mediaDB) {
			release()
			if err := os.Remove(mediaDB); err != nil {
				return out, fmt.Errorf("removing packed media: %w", err)
			}
			out.MediaRemoved = true
		}
	case MediaKeep, MediaRepack, MediaOn:
		if t.Media != MediaOn && !p.Media {
			break // never packed: nothing to keep
		}
		if t.Media != MediaRepack {
			paired := p.MediaPaired()
			if out.Rebuilt {
				paired = MediaPaired(textDB) // a rebuild from a changed source unpairs it
			}
			if paired {
				out.MediaCurrent = true
				break
			}
		}
		if err := pack(src, textDB, mediaDB, h, &out); err != nil {
			return out, err
		}
	}
	// An ingest and a pack each rewrite the receipt; a deletion is the one
	// change that leaves it describing a media.db that is gone.
	if dir != "" && out.MediaRemoved {
		_ = WriteInfo(dir)
	}
	return out, nil
}

// reconcileText resolves the database to work on and (re)builds the text when
// the target asks for it. It returns what was there before any rebuild: the
// media decision depends on whether a media.db existed then.
func reconcileText(src string, t Target, h Hooks, out *Outcome) (p Prepared, dir string, err error) {
	textDB, dir := t.Out, t.Dir
	if textDB == "" {
		if dir == "" {
			var ok bool
			if dir, ok = LookupDir(src); !ok {
				if dir, err = ClaimDir(src); err != nil {
					return p, dir, err
				}
			}
		}
		textDB = TextDBPath(dir)
	}
	out.TextDB = textDB

	p = Inspect(textDB)
	plan := p.Plan
	if t.Out != "" {
		plan = Plan{}
	}
	if t.FullText != nil {
		plan.FullText = *t.FullText
	}
	if t.Contains != nil {
		plan.Contains = *t.Contains
	}
	out.Plan = plan

	// The reasons are read only when they can matter: a usable, current
	// database on the wanted plan is rebuilt only if Rebuild says so, and this
	// runs on every open of a self-preparing format.
	need := t.Out != "" || !p.Current(src) || plan != p.Plan
	var why []Reason
	if !need && t.Rebuild != nil {
		why = p.TextStale(src)
		need = t.Rebuild(why)
	}
	if !need {
		return p, dir, nil
	}
	if p.Meta != nil {
		if why == nil {
			why = p.TextStale(src)
		}
		out.Why = why
	}
	if h.Release != nil {
		h.Release(textDB)
	}
	return p, dir, ingest(src, textDB, plan, h, out)
}

// ingest writes the text.db from the source.
func ingest(src, textDB string, plan Plan, h Hooks, out *Outcome) (resultErr error) {
	IndexDiagnostic("open reader source=%q target=%q contains=%v fulltext=%v", src, textDB, plan.Contains, plan.FullText)
	defer func() {
		IndexDiagnostic("text result source=%q target=%q error=%q", src, textDB, fmt.Sprint(resultErr))
	}()
	var r dict.Reader
	var err error
	if h.Reader != nil {
		r, err = h.Reader()
	} else if r, err = dict.OpenReader(src); err == nil {
		defer r.Close()
	}
	if err != nil {
		return err
	}
	start := time.Now()
	rep, err := IngestPlan(r, textDB, plan, h.Progress)
	if err != nil {
		return err
	}
	out.Rebuilt, out.Report, out.TextTime = true, rep, time.Since(start)
	return nil
}

// pack writes the media.db from the source's resources and the loose files
// its articles reference. Nothing to pack leaves no media.db: one that does not
// pair serves nothing and would be reported outdated forever.
func pack(src, textDB, mediaDB string, h Hooks, out *Outcome) (resultErr error) {
	IndexDiagnostic("media start source=%q target=%q", src, mediaDB)
	defer func() {
		IndexDiagnostic("media result source=%q target=%q error=%q", src, mediaDB, fmt.Sprint(resultErr))
	}()
	if mediaDB == "" {
		return fmt.Errorf("media is packed beside a text.db, and %s is not one", textDB)
	}
	if fsx.FileExists(mediaDB) && h.Release != nil {
		h.Release(textDB) // the pack renames over it
	}
	var d dict.Dictionary
	done := func() {}
	var err error
	if h.Backend != nil {
		d, done, err = h.Backend()
	} else if d, err = dict.Open(src); err == nil {
		done = func() { d.Close() }
	}
	if err != nil {
		return err
	}
	defer done()
	names, beside := MediaNames(d, textDB)
	if len(names) == 0 {
		out.MediaEmpty = true
		if fsx.FileExists(mediaDB) {
			if err := os.Remove(mediaDB); err != nil {
				return fmt.Errorf("removing unpaired media: %w", err)
			}
			out.MediaRemoved = true
		}
		return nil
	}
	uuid, err := ReadMetaValue(textDB, "dict_uuid")
	if err != nil {
		return err
	}
	start := time.Now()
	last := time.Time{}
	callerProgress := h.MediaProgress
	h.MediaProgress = func(done, total int) {
		if time.Since(last) >= 10*time.Second {
			last = time.Now()
			IndexDiagnostic("media progress source=%q progress=%d/%d elapsed=%s", src, done, total, time.Since(start).Round(time.Millisecond))
		}
		if callerProgress != nil {
			callerProgress(done, total)
		}
	}
	if err := IngestMedia(d, names, mediaDB, uuid, h.MediaProgress); err != nil {
		return err
	}
	out.Packed, out.Beside, out.MediaTime = len(names), beside, time.Since(start)
	return nil
}

// OpenSelfPrepared opens the prepared database of a format with no index of
// its own - DSL, BGL, wudict markdown - preparing it first when it is
// missing, unusable or built from an older edition of the source, with the
// plan the user chose for it or headwords only (D24). reader is called only
// when an ingest is needed; the caller owns and closes what it returns.
func OpenSelfPrepared(path, format string, reader func() (dict.Reader, error)) (*Store, error) {
	name := ""
	out, err := Reconcile(path, Target{}, Hooks{
		Reader: func() (dict.Reader, error) {
			r, err := reader()
			if err == nil {
				name = r.Meta().Name
				logx.Status("%spreparing search index (%s, first open)…", logx.Dict(name), format)
			}
			return r, err
		},
		Progress: func(done, _ int) { logx.Progress("  %d entries", done) },
	})
	if err != nil {
		if name != "" {
			logx.ClearLine()
			return nil, fmt.Errorf("preparing %q: %w", name, err)
		}
		return nil, err
	}
	if out.Rebuilt {
		ReportPrepared(name, out.Report, out.TextTime)
	}
	return Open(out.TextDB)
}
