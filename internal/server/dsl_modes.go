// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// dslView describes a DSL dictionary's source and index. IndexRemoved is the
// JSON name this field has always had - the state used to mean "the user took
// this dictionary's index away", and it means "there is no index right now" now
// that an index cannot be removed at all. It stays while the dictionary is
// queued for preparation, and it is cleared by the build that finishes it.
type dslView struct {
	Source          string `json:"source"`
	SourceAvailable bool   `json:"sourceAvailable"`
	IndexRemoved    bool   `json:"indexRemoved"`
}

// Resolve once at discovery; cached-only dictionaries use their source receipt.
func dslIdentity(path string) (source, variant string) {
	if strings.EqualFold(filepath.Ext(path), ".db") {
		meta, err := store.ReadMeta(path)
		if err != nil {
			return "", ""
		}
		path = meta["source_path"]
	}
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".dsl"), strings.HasSuffix(lower, ".dsl.dz"):
		return cleanAbs(path), "original"
	case strings.HasSuffix(lower, ".dslgd"):
		input := dict.SourceInput(path)
		if input != path {
			return cleanAbs(input), "gd"
		}
	}
	return "", ""
}

func (r *Registry) dslView(e *entry) *dslView {
	if e.dslSource == "" {
		return nil
	}
	return &dslView{Source: e.dslSource, SourceAvailable: fsx.FileExists(e.dslSource), IndexRemoved: e.indexBlocked()}
}

func (r *Registry) dslAvailable(e *entry) bool { return !e.indexBlocked() }

// indexBlocked reports that this dictionary has no index right now - one is
// queued or being built, or a build failed and the marker has not been cleared
// yet. It is NOT a state the user can put a dictionary into: the headword index
// is mandatory, nothing deletes it, and the queue prepares any dictionary whose
// index is missing (a failed build clears the marker so the source preview can
// serve it meanwhile).
func (e *entry) indexBlocked() bool {
	if e.reg == nil {
		return false
	}
	f, _ := e.reg.prefs.data()
	return f.DSLRemoved[e.indexRemovalKey()]
}

func (e *entry) indexRemovalKey() string {
	if e.dslSource != "" {
		return e.dslSource + "\noriginal"
	}
	return cleanAbs(e.Path) + "\noriginal"
}

func (e *entry) setIndexRemoved(removed bool) error {
	if e.reg == nil {
		return nil
	}
	p := e.reg.prefs
	p.editMu.Lock()
	defer p.editMu.Unlock()
	key := e.indexRemovalKey()
	return p.mutate(func(f *prefsFile) {
		if e.dslSource != "" {
			if f.IndexMigrated == nil {
				f.IndexMigrated = map[string]bool{}
			}
			f.IndexMigrated[e.dslSource] = true
		}
		if f.DSLRemoved == nil {
			f.DSLRemoved = make(map[string]bool)
		}
		if removed {
			f.DSLRemoved[key] = true
		} else {
			delete(f.DSLRemoved, key)
		}
	})
}

// restoreIndex rebuilds an index deliberately removed by the user, or finishes
// the first index for a newly added dictionary.
func (e *entry) restoreIndex(plan store.Plan, progress store.Progress, lane *indexSemaphore) error {
	return e.restoreIndexContext(context.Background(), plan, progress, lane)
}

func (e *entry) restoreIndexContext(ctx context.Context, plan store.Plan, progress store.Progress, lane *indexSemaphore) error {
	if nativeIndexOnly(e) {
		store.IndexDiagnostic("skip restored index source=%q reason=native-index", e.Path)
		return e.setIndexRemoved(false)
	}
	defer lane.Acquire()()
	defer HoldActiveProcs()()
	if err := ctx.Err(); err != nil {
		return err
	}
	e.ingestMu.Lock()
	defer e.ingestMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	defer e.rebuilding.Store(false)
	if !e.indexBlocked() {
		return nil
	}
	dir, err := store.ClaimDir(e.Path)
	if err != nil {
		return err
	}
	textDB := store.TextDBPath(dir)
	prepared, ready := store.PreparedFor(e.Path)
	// SameIndexes, not ==: the form the article index is kept in is not a reason
	// to rebuild by itself - a database that keeps it inside is current, and it
	// moves into a file of its own when the full text is rebuilt - so comparing
	// it here would re-ingest every library built before the split.
	if !ready || prepared != textDB || len(store.Inspect(textDB).TextStale(e.Path)) != 0 || !store.KeptPlan(textDB).SameIndexes(plan) {
		if _, err := e.reconcileLockedContext(ctx, e.probeName(), store.Target{
			FullText: &plan.FullText, Contains: &plan.Contains, Rebuild: store.Always,
		}, progress); err != nil {
			return err
		}
	}
	// Only discard old copies after the replacement is complete. Explicit
	// database sources are user dictionaries, even if they share a receipt.
	dirs, err := store.SourceFolders(e.Path)
	if err != nil {
		return err
	}
	protected := make(map[string]bool)
	for _, other := range e.reg.all() {
		protected[dict.CanonPath(other.Path)] = true
	}
	for _, old := range dirs {
		if old == dir || protected[dict.CanonPath(store.TextDBPath(old))] {
			continue
		}
		if _, err := store.RemovePrepared(old); err != nil {
			return err
		}
	}
	if err := e.setIndexRemoved(false); err != nil {
		return err
	}
	_ = store.WriteInfo(dir)
	return e.reopen()
}
