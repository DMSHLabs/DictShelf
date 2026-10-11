// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"sync"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/logx"
)

// failures are the open failures kept across restarts (open-failures.json,
// UserDir.Failures). entry.err keeps a failure for the life of the process;
// this keeps the ones that would only fail again, so a dictionary that cannot
// be opened costs a stat at startup instead of the whole failed open: 7 s for
// a 1.2 GB wudict markdown text unpacked up to the bound that stopped it.
//
// Two kinds are kept:
//   - dict.ErrUnreadable, which the file's bytes decided: it holds while the
//     file (sourceSig) and the build that judged it (failBuild) are the same;
//   - a *dict.NoRoomError on a disk: it holds, as well, while that disk has
//     no more free space than the open was known to need.
//
// Anything else - a permission, an I/O error, memory - is tried again on the
// next start. The user's Rescan forgets every record (Registry.ForgetFailures).
// Only an open with no prepared database consults or writes a record: a
// library folder that appears for the same file answers without it.
type failures struct {
	path string // "" keeps the records in memory only
	mu   sync.Mutex
	recs map[string]failRec // by source path
}

type failRec struct {
	Sig      string `json:"sig"`   // sourceSig when it failed
	Build    string `json:"build"` // failBuild when it failed
	Error    string `json:"error"`
	TooLarge bool   `json:"tooLarge,omitempty"`
	// a NoRoomError: what the space was for, where it was missing, and how
	// much was known to be needed
	What string `json:"what,omitempty"`
	Dir  string `json:"dir,omitempty"`
	Need int64  `json:"need,omitempty"`
}

type failsFile struct {
	Version int                `json:"version"`
	Files   map[string]failRec `json:"files"`
}

const (
	failsVersion = 1
	failsMax     = 1 << 20 // a record is ~300 B: room for ~3000 dictionaries
)

// diskFree is fsx.FreeSpace; a variable for the tests.
var diskFree = fsx.FreeSpace

// failBuild is what judged a failure: this executable and its readers'
// versions. "" when the executable cannot be identified, and then nothing is
// recorded - a record from an unknown build could outlive the rule it states.
func failBuild() string {
	id := buildID()
	if id == "" {
		return ""
	}
	return id + "|" + dict.ReaderVersionsSig()
}

// loadFailures reads the records at path. A missing, unreadable or foreign
// file is no records; a record of a file that is gone is dropped.
func loadFailures(path string) *failures {
	f := &failures{path: path, recs: map[string]failRec{}}
	if path == "" {
		return f
	}
	b, err := fsx.ReadBounded(path, failsMax)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logx.V("open failures: %v", err)
		}
		return f
	}
	var ff failsFile
	if json.Unmarshal(b, &ff) != nil || ff.Version != failsVersion {
		return f
	}
	for p, r := range ff.Files {
		if p != "" && r.Sig != "" && r.Build != "" && r.Error != "" && fsx.FileExists(p) {
			f.recs[p] = r
		}
	}
	if len(f.recs) != len(ff.Files) {
		f.saveLocked() // not yet shared: no lock to take
	}
	return f
}

// lookup is the recorded failure of the file at path, now at sig, or nil
// when there is none that still holds - and then the record is dropped.
func (f *failures) lookup(path, sig string) error {
	if f == nil || sig == "" {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.recs[path]
	if !ok {
		return nil
	}
	if r.Sig != sig || r.Build != failBuild() {
		f.dropLocked(path)
		return nil
	}
	if r.Dir == "" {
		return &recalled{msg: r.Error, tooLarge: r.TooLarge, unreadable: true}
	}
	free, ok := diskFree(r.Dir)
	if !ok || free > r.Need {
		f.dropLocked(path)
		return nil
	}
	return &dict.NoRoomError{What: r.What, Dir: r.Dir, Need: r.Need, Free: free}
}

// record keeps err, the failure of the file at path at sig, when it is a kind
// that would only fail again.
func (f *failures) record(path, sig string, err error) {
	build := failBuild()
	if f == nil || sig == "" || err == nil || build == "" {
		return
	}
	r := failRec{Sig: sig, Build: build, Error: err.Error(), TooLarge: errors.Is(err, dict.ErrTooLarge)}
	var re *dict.NoRoomError
	switch {
	case errors.As(err, &re):
		if re.Dir == "" {
			return // memory: there is nothing to look at before trying again
		}
		r.What, r.Dir, r.Need = re.What, re.Dir, re.Need
	case errors.Is(err, dict.ErrUnreadable):
	default:
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs[path] = r
	f.saveLocked()
}

// forget drops every record.
func (f *failures) forget() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.recs) == 0 {
		return
	}
	clear(f.recs)
	f.saveLocked()
}

func (f *failures) dropLocked(path string) {
	delete(f.recs, path)
	f.saveLocked()
}

// saveLocked writes the records; a failure to write is logged and costs only
// the next start's retry.
func (f *failures) saveLocked() {
	if f.path == "" {
		return
	}
	if len(f.recs) == 0 {
		if err := fsx.RemoveIfExists(f.path); err != nil {
			logx.V("open failures: %v", err)
		}
		return
	}
	b, err := json.MarshalIndent(failsFile{Version: failsVersion, Files: f.recs}, "", "  ")
	if err == nil {
		err = fsx.WriteAtomic(f.path, append(b, '\n'), 0o644)
	}
	if err != nil {
		logx.V("open failures: %v", err)
	}
}

// recalled is a recorded failure as the open that read it returns it: its
// text, and the kinds it was of.
type recalled struct {
	msg                  string
	tooLarge, unreadable bool
}

func (e *recalled) Error() string { return e.msg }

func (e *recalled) Is(target error) bool {
	return (target == dict.ErrTooLarge && e.tooLarge) || (target == dict.ErrUnreadable && e.unreadable)
}
