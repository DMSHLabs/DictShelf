// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// Orphaned prepared dictionaries (D156): the library folders whose source file
// is no longer on disk. The panel asks for them after "Rescan folders" - the
// moment the user says their folders changed - and the user answers each one:
// delete it, or keep it for good. store/orphans.go holds the rules; this is
// the HTTP surface and the registry's part, which is closing what it holds
// open before a folder disappears.
//
//	GET  /api/orphans                                 → {orphans, size}
//	POST /api/orphans  {"delete":[folder…], "keep":[folder…]}  → orphanReport
//
// Folders are named, never pathed, and every one is re-judged at the moment it
// is acted on: the list the user answered is older than their answer.

// maxOrphanAnswers bounds one answer. A library holds one folder per
// dictionary; nobody has this many, and a request that claims to is not one.
const maxOrphanAnswers = 10000

type orphanOutcome struct {
	Folder string `json:"folder"`
	Name   string `json:"name,omitempty"`
	Freed  int64  `json:"freed,omitempty"`
	Error  string `json:"error,omitempty"`
}

type orphanReport struct {
	Deleted []orphanOutcome `json:"deleted"`
	Kept    []orphanOutcome `json:"kept"`
	Failed  []orphanOutcome `json:"failed"`
	Freed   int64           `json:"freed"`
}

func (s *Server) handleOrphans(w http.ResponseWriter, r *http.Request) {
	result, err := s.orphansSnapshot()
	if err != nil {
		httpErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) orphansSnapshot() (map[string]any, error) {
	list, err := store.FindOrphans(s.reg.IsBuiltin)
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	if list == nil {
		list = []store.Orphan{}
	}
	var size int64
	for _, o := range list {
		size += o.Size
	}
	return map[string]any{"orphans": list, "size": size}, nil
}

func (s *Server) handleResolveOrphans(w http.ResponseWriter, r *http.Request) {
	// Keeping writes to the library and deleting is irreversible: both are
	// the removal permission's, so one rule answers who may do either.
	if !s.removalOffered(r) {
		httpErr(w, 403, "deleting from another machine is off: set ALLOW_REMOTE_DELETE = \"1\" "+
			"in wudict.toml, or do it on the machine running wudict")
		return
	}
	var req struct {
		Delete []string `json:"delete"`
		Keep   []string `json:"keep"`
	}
	if !decodeJSON(w, r, &req, 1<<20) {
		return
	}
	report, err := s.resolveOrphanRequest(req.Delete, req.Keep)
	if err != nil {
		httpErr(w, 400, "%v", err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) resolveOrphanRequest(del, keep []string) (orphanReport, error) {
	if len(del)+len(keep) == 0 {
		return orphanReport{}, errors.New("nothing to delete or keep")
	}
	if len(del)+len(keep) > maxOrphanAnswers {
		return orphanReport{}, errors.New("too many folders in one request")
	}
	return s.reg.ResolveOrphans(del, keep), nil
}

// ResolveOrphans deletes and keeps the named orphans and reports each outcome.
// A folder named twice, or in both lists, is refused rather than guessed at.
// A failure is reported per folder and never stops the others.
func (r *Registry) ResolveOrphans(del, keep []string) orphanReport {
	rep := orphanReport{Deleted: []orphanOutcome{}, Kept: []orphanOutcome{}, Failed: []orphanOutcome{}}
	named := map[string]int{}
	for _, f := range del {
		named[f]++
	}
	for _, f := range keep {
		named[f]++
	}
	fail := func(f, msg string) { rep.Failed = append(rep.Failed, orphanOutcome{Folder: f, Error: msg}) }
	for _, f := range del {
		if named[f] > 1 {
			fail(f, "named more than once")
			continue
		}
		o, err := store.OrphanNamed(f, r.IsBuiltin)
		if err != nil {
			fail(f, err.Error())
			continue
		}
		freed, err := r.removeOrphan(o)
		if err != nil {
			fail(f, err.Error())
			continue
		}
		rep.Deleted = append(rep.Deleted, orphanOutcome{Folder: f, Name: o.Name, Freed: freed})
		rep.Freed += freed
		logx.V("removed orphaned %s (%d MB); its source %s was gone", o.Dir, freed>>20, o.Source)
	}
	for _, f := range keep {
		if named[f] > 1 {
			fail(f, "named more than once")
			continue
		}
		o, err := store.OrphanNamed(f, r.IsBuiltin)
		if err == nil {
			err = store.KeepOrphan(f, r.IsBuiltin)
		}
		if err != nil {
			fail(f, err.Error())
			continue
		}
		rep.Kept = append(rep.Kept, orphanOutcome{Folder: f, Name: o.Name})
	}
	if len(rep.Deleted) > 0 {
		if err := r.Rescan(); err != nil {
			logx.V("rescan after removing orphans: %v", err)
		}
	}
	return rep
}

// removeOrphan closes the entry serving an orphan (listed when the library is
// in use) and deletes its folder. The close is synchronous for the reason
// Remove gives: the files are about to be unlinked, and Windows refuses to
// delete an open one.
func (r *Registry) removeOrphan(o store.Orphan) (int64, error) {
	if e := r.entryAt(store.TextDBPath(o.Dir)); e != nil {
		e.ingestMu.Lock()
		defer e.ingestMu.Unlock()
		e.closeNow()
	}
	return store.RemoveOrphan(o.Folder, r.IsBuiltin)
}

// entryAt returns the listed entry whose file is path, if any.
func (r *Registry) entryAt(path string) *entry {
	path = filepath.Clean(path)
	for _, e := range r.all() {
		if filepath.Clean(e.Path) == path {
			return e
		}
		if prepared, ok := e.preparedDB(); ok && filepath.Clean(prepared) == path {
			return e
		}
	}
	return nil
}
