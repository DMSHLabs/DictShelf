// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/howto"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
)

// Removing a dictionary from the running app (D63, amended).
//
// Who may: the rule every other machine-acting control uses (reveal, power) -
// loopback is the trusted caller. This is a personal app managing a library
// its user owns; managing includes throwing things away, and doing it from the
// page you are already looking at beats copying a path into a file manager. A
// remote browser is the one caller acting on someone else's disk, so it gets a
// different answer: ALLOW_REMOTE_DELETE, off by default because that caller has
// authenticated as nobody, on in one line for a server whose LAN its owner
// trusts with an irreversible operation. Tying removal to Reveal instead -
// offering it only where no file manager can be opened - would withhold it from
// the desktop user at the keyboard and grant it to every browser on the LAN.
//
// The primitive lives at every layer: DELETE /api/library, `wudict rm`, and
// store.RemovePrepared as the single file-deleting surface.
//
// Two objects, never conflated:
//
//   - the PREPARED FOLDER (<db dir>/<name>/) - ours, D20-shaped, unambiguous.
//   - the ORIGINAL FILES - a dictionary is rarely one file, so the set comes
//     from dict.SourceFiles and is shown to the user before anything happens.
//
// And the trap that sets the default: preparation is automatic (AUTO_INDEX).
// Deleting only the prepared folder of a dictionary whose source is still in a
// scanned folder frees space until the next search re-prepares it. Removing
// both is therefore the default, and "index only" reports what will happen.

// removal is what a removal did - reported back so the UI states outcomes
// rather than assuming them, and so a partial failure is visible.
type removal struct {
	Name    string   `json:"name"`
	Folder  string   `json:"folder,omitempty"`  // prepared folder removed
	Sources []string `json:"sources,omitempty"` // original files removed
	Freed   int64    `json:"freed"`             // bytes
	Gone    bool     `json:"gone"`              // no longer listed at all
	Note    string   `json:"note,omitempty"`    // consequence the user should know
}

// Remove deletes a dictionary's prepared folder, its original files, or both,
// and rescans so the registry reflects what is actually on disk.
//
// dropPrepared+dropSource is "remove this dictionary". dropPrepared alone
// frees the indexes and leaves a dictionary that will be re-indexed on next
// use. dropSource alone is the Android-shaped case - reclaim the imported
// original and keep the prepared dictionary, which D24 §4 already governs:
// the source is the safety net, so it may only be cut once the media that
// would be lost with it has been packed.
func (r *Registry) Remove(id string, dropPrepared, dropSource bool) (removal, error) {
	var rep removal
	if !dropPrepared && !dropSource {
		return rep, fmt.Errorf("nothing to remove")
	}
	e, err := r.get(id)
	if err != nil {
		return rep, err
	}
	if e.builtin && !(dropPrepared && dropSource) {
		// The app wrote both halves and keeps no other copy: it goes whole.
		return rep, fmt.Errorf("%s is removed whole, with its index", filepath.Base(e.Path))
	}
	// Blocks (and is blocked by) an ingest on this dictionary: deleting the
	// folder a rebuild is writing into would leave the rebuild finishing into
	// nowhere.
	e.ingestMu.Lock()
	ingestLocked := true
	defer func() {
		if ingestLocked {
			e.ingestMu.Unlock()
		}
	}()

	rep.Name = e.probeName()
	native := store.IsTextDB(e.Path)

	prepared := ""
	if native {
		// the entry IS the prepared dictionary: its folder is its parent
		prepared = filepath.Dir(e.Path)
	} else if dir, ok := store.LookupDir(e.Path); ok {
		prepared = dir
	}
	var sources []string
	if !native {
		sources = dict.SourceFiles(e.Path)
	}

	if dropSource && len(sources) == 0 {
		if !dropPrepared {
			return rep, fmt.Errorf("%q has no original files - it is the prepared dictionary itself", rep.Name)
		}
		// "remove this dictionary" on a dictionary that IS the prepared folder:
		// there is nothing else to remove, so this is that request satisfied,
		// not a request that cannot be met.
		dropSource = false
	}
	if dropPrepared && prepared == "" && !dropSource {
		return rep, fmt.Errorf("%q has nothing prepared to remove", rep.Name)
	}
	if dropSource && !dropPrepared {
		// D24 §4, read in the other direction. Media that is not packed lives
		// only in the original, and the prepared text.db would keep serving
		// articles whose images and audio had been deleted.
		packed := prepared != "" && fsx.FileExists(store.MediaDBPath(prepared))
		if !packed && !e.noPackableMedia() {
			return rep, fmt.Errorf(
				"pack media for %q first - its images and audio are still only in the original files", rep.Name)
		}
		if prepared == "" {
			return rep, fmt.Errorf("%q is not prepared, so deleting its files would delete the dictionary", rep.Name)
		}
	}

	// Closed synchronously, not on the usual grace timer: the files are about
	// to be unlinked, Windows refuses to delete an open one, and a reader that
	// gets an error is a better outcome than a half-deleted folder. Requests
	// already in flight fail; the next one reopens, or finds it gone.
	e.rebuilding.Store(true)
	defer e.rebuilding.Store(false)
	e.openMu.Lock()
	openLocked := true
	defer func() {
		if openLocked {
			e.openMu.Unlock()
		}
	}()
	e.closeNow()
	if dropSource && !native {
		if err := dict.RemoveComparison(e.Path); err != nil {
			return rep, err
		}
	}

	if dropPrepared && prepared != "" {
		if !dropSource {
			if err := e.setIndexRemoved(true); err != nil {
				return rep, err
			}
		}
		n, err := store.RemovePrepared(prepared)
		if err != nil {
			if !dropSource {
				_ = e.setIndexRemoved(false)
			}
			return rep, err
		}
		rep.Folder, rep.Freed = prepared, rep.Freed+n
		logx.V("removed prepared dictionary %s (%d MB)", prepared, n>>20)
	}
	if dropSource {
		var failed []string
		for _, p := range sources {
			n := store.TreeSize(p)
			if err := os.RemoveAll(p); err != nil {
				failed = append(failed, fmt.Sprintf("%s (%v)", filepath.Base(p), err))
				continue
			}
			rep.Sources = append(rep.Sources, p)
			rep.Freed += n
		}
		if len(failed) > 0 {
			// Reported, not fatal: whatever was deleted stays deleted, and the
			// user needs to know which files are still there.
			rep.Note = "could not delete " + strings.Join(failed, ", ")
		}
		logx.V("removed %d original file(s) of %s", len(rep.Sources), rep.Name)
		r.pruneEmptied(rep.Sources)
		if !dropPrepared && prepared != "" {
			// Keeping the prepared data without its files IS the decision to
			// keep it standalone: recorded, so Rescan never offers it back as
			// an orphan (D156).
			if err := store.MarkKept(prepared); err != nil {
				logx.V("marking %s kept: %v", prepared, err)
			}
		}
	}

	// Rescan synchronously closes entries that have left the collection. It
	// takes their locks itself; all file operations above are already finished.
	e.openMu.Unlock()
	openLocked = false
	e.ingestMu.Unlock()
	ingestLocked = false
	if err := r.Rescan(); err != nil {
		logx.V("rescan after removing %s: %v", rep.Name, err)
	}
	rep.Gone = !r.has(id)
	return rep, nil
}

// pruneEmptied removes the folder a removed dictionary's files were the whole
// of. An import creates one folder per dictionary, so removing the dictionary
// that lived there leaves an empty one behind - and an empty folder with a
// dictionary's name is not inert: the next import of the same bundle would
// read it as an installation and offer to "update" something the user has
// just removed (D137).
//
// Deliberately one level and no recursion, and never a scanned folder or the
// download shelf: those are places the user put things, and this only unmakes
// what an install made. A folder that still holds anything at all - a note, a
// licence, the other half of something - is left exactly as it is.
func (r *Registry) pruneEmptied(removed []string) {
	roots := make(map[string]bool)
	for _, d := range r.Dirs() {
		roots[filepath.Clean(d)] = true
	}
	seen := make(map[string]bool, len(removed))
	for _, p := range removed {
		dir := filepath.Clean(filepath.Dir(p))
		if seen[dir] || roots[dir] || filepath.Base(dir) == dict.DownloadDirName {
			continue
		}
		seen[dir] = true
		ents, err := os.ReadDir(dir)
		if err != nil || len(ents) > 0 {
			continue
		}
		if err := os.Remove(dir); err != nil {
			// Nothing to report: the dictionary is gone either way, and an
			// empty folder is not a failure the user can act on (D102).
			logx.V("could not remove emptied folder %s: %v", dir, err)
		}
	}
}

// has reports whether an id survived the last rescan.
func (r *Registry) has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byID[id]
	return ok
}

// closeNow drops this entry's open backend immediately, and any it retired
// that are still in their grace. evict() defers the close so in-flight readers
// finish; removal cannot, because the file is about to disappear underneath
// them either way.
func (e *entry) closeNow() {
	e.dMu.Lock()
	d := e.d
	e.d, e.err = nil, nil
	e.weight.Store(0)
	e.dMu.Unlock()
	if d != nil {
		_ = d.Close()
	}
	e.retired.closeAll()
}

// handleRemoveLibrary deletes one dictionary. DELETE, because it is one, and
// because a destructive action must not be reachable by following a link or by
// a page that only knows how to GET.
//
//	DELETE /api/library?dict=<id>[&prepared=0|1][&source=0|1]
//
// Both default to 1: "remove this dictionary". See Remove for why that is the
// default rather than the cautious-looking prepared-only.
func (s *Server) handleRemoveLibrary(w http.ResponseWriter, r *http.Request) {
	if !s.removalOffered(r) {
		httpErr(w, 403, "deleting from another machine is off: set ALLOW_REMOTE_DELETE = \"1\" "+
			"in wudict.toml, or do it on the machine running wudict")
		return
	}
	q := r.URL.Query()
	id := strings.TrimSpace(q.Get("dict"))
	if id == "" {
		httpErr(w, 400, "missing dict parameter")
		return
	}
	// both default on: Remove… deletes the whole dictionary unless told to keep a half
	prepared, sent := queryFlag(q, "prepared")
	prepared = prepared || !sent
	source, sent := queryFlag(q, "source")
	source = source || !sent
	if source && !prepared && !s.reg.UseCached() {
		// Its folder would survive but nothing would list it: the library is
		// only enrolled when the user opted in (D19).
		httpErr(w, 409, "turn on prepared dictionaries first, or this one would vanish from the list with its files")
		return
	}
	builtin := s.reg.builtinID(id)
	rep, err := s.reg.Remove(id, prepared, source)
	if err != nil {
		httpErr(w, 400, "%v", err)
		return
	}
	if builtin && s.User.Builtin() != "" {
		// Recorded, so the next start does not write it back; Setup's
		// "Bring back the wudict howto" undoes it (POST /api/howto?restore=1).
		if err := howto.MarkRemoved(s.User.Builtin()); err != nil {
			rep.Note = "it will be back at the next start: " + err.Error()
		}
	}
	writeJSON(w, rep)
}

// removalOffered says whether this caller may delete. Loopback always may: the
// user at the keyboard owns the files, and the platform they are on is not the
// question. Everyone else is a browser on another machine, and needs to have
// been invited - ALLOW_REMOTE_DELETE, which defaults to OFF (see config) and is
// what a server bound to 0.0.0.0 turns off.
func (s *Server) removalOffered(r *http.Request) bool {
	return isLoopback(r) || s.AllowRemoteDelete
}
